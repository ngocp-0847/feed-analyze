// Package store lưu bài viết, phân loại, storyline và bản tin vào SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS feeds (
  name TEXT PRIMARY KEY, url TEXT, last_fetched INTEGER, last_status TEXT,
  last_items INTEGER DEFAULT 0, last_new INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS articles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  hash TEXT UNIQUE NOT NULL,
  feed TEXT, source TEXT, region TEXT, lang TEXT,
  title TEXT NOT NULL, link TEXT, summary TEXT,
  published_at INTEGER NOT NULL, fetched_at INTEGER NOT NULL,
  classified INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_articles_pub ON articles(published_at);
CREATE INDEX IF NOT EXISTS idx_articles_cls ON articles(classified);
CREATE TABLE IF NOT EXISTS classifications (
  article_id INTEGER PRIMARY KEY REFERENCES articles(id),
  primary_topic TEXT, relevance REAL, importance INTEGER, scale TEXT, geo TEXT,
  entities TEXT, summary_vi TEXT, model TEXT, created_at INTEGER
);
CREATE TABLE IF NOT EXISTS article_topics (
  article_id INTEGER, topic TEXT, relevance REAL,
  PRIMARY KEY(article_id, topic)
);
CREATE INDEX IF NOT EXISTS idx_at_topic ON article_topics(topic, relevance);
CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
  title, summary, summary_vi, entities, tokenize='unicode61 remove_diacritics 2'
);
CREATE TABLE IF NOT EXISTS storylines (
  slug TEXT PRIMARY KEY, topic TEXT, title TEXT, status TEXT, level TEXT,
  summary TEXT, why TEXT, first_date TEXT, last_date TEXT,
  days_active INTEGER DEFAULT 1, updated_at INTEGER
);
CREATE TABLE IF NOT EXISTS storyline_events (
  slug TEXT, date TEXT, status TEXT, summary TEXT, detail TEXT,
  PRIMARY KEY(slug, date)
);
CREATE TABLE IF NOT EXISTS storyline_articles (
  slug TEXT, article_id INTEGER, date TEXT,
  PRIMARY KEY(slug, article_id)
);
CREATE TABLE IF NOT EXISTS reports (
  date TEXT, kind TEXT, json TEXT, markdown TEXT, model TEXT, created_at INTEGER,
  PRIMARY KEY(date, kind)
);
CREATE TABLE IF NOT EXISTS llm_calls (
  id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER, purpose TEXT, model TEXT,
  cost_usd REAL, duration_ms INTEGER, ok INTEGER, err TEXT
);
`

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: 1 writer — tránh SQLITE_BUSY khi phân loại song song
	if _, err := db.Exec(schema + secSchema); err != nil {
		return nil, fmt.Errorf("khởi tạo schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ---------- articles ----------

type Article struct {
	ID        int64     `json:"id"`
	Hash      string    `json:"-"`
	Feed      string    `json:"feed"`
	Source    string    `json:"source"`
	Region    string    `json:"region"`
	Lang      string    `json:"lang"`
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Summary   string    `json:"summary,omitempty"`
	Published time.Time `json:"published"`
	Fetched   time.Time `json:"-"`
}

type TopicScore struct {
	Topic     string  `json:"topic"`
	Relevance float64 `json:"relevance"`
}

type Classification struct {
	ArticleID  int64        `json:"-"`
	Topics     []TopicScore `json:"topics,omitempty"`
	Primary    string       `json:"primary_topic,omitempty"`
	Relevance  float64      `json:"relevance"`
	Importance int          `json:"importance"`
	Scale      string       `json:"scale,omitempty"`
	Geo        string       `json:"geo,omitempty"`
	Entities   []string     `json:"entities,omitempty"`
	SummaryVI  string       `json:"summary_vi,omitempty"`
	Model      string       `json:"-"`
}

// ArticleView = bài viết + phân loại (nếu có).
type ArticleView struct {
	Article
	*Classification
}

func (s *Store) InsertArticle(a *Article) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO articles(hash,feed,source,region,lang,title,link,summary,published_at,fetched_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, a.Hash, a.Feed, a.Source, a.Region, a.Lang, a.Title, a.Link, a.Summary,
		a.Published.Unix(), a.Fetched.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	a.ID, _ = res.LastInsertId()
	_, err = s.db.Exec(`INSERT INTO articles_fts(rowid,title,summary,summary_vi,entities) VALUES(?,?,?,?,?)`,
		a.ID, a.Title, a.Summary, "", "")
	return true, err
}

func (s *Store) TitleExistsSince(title string, since time.Time) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM articles WHERE title=? AND published_at>=?`, title, since.Unix()).Scan(&n)
	return n > 0
}

const articleCols = `a.id,a.feed,a.source,a.region,a.lang,a.title,a.link,a.summary,a.published_at,a.fetched_at`

func scanArticle(sc interface{ Scan(...any) error }, extra ...any) (Article, error) {
	var a Article
	var pub, fet int64
	dst := append([]any{&a.ID, &a.Feed, &a.Source, &a.Region, &a.Lang, &a.Title, &a.Link, &a.Summary, &pub, &fet}, extra...)
	if err := sc.Scan(dst...); err != nil {
		return a, err
	}
	a.Published, a.Fetched = time.Unix(pub, 0), time.Unix(fet, 0)
	return a, nil
}

func (s *Store) Unclassified(limit int) ([]Article, error) {
	rows, err := s.db.Query(`SELECT `+articleCols+` FROM articles a WHERE classified=0 ORDER BY published_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CountUnclassified() int {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM articles WHERE classified=0`).Scan(&n)
	return n
}

func (s *Store) SaveClassification(c Classification) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ents, _ := json.Marshal(c.Entities)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO classifications(article_id,primary_topic,relevance,importance,scale,geo,entities,summary_vi,model,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, c.ArticleID, c.Primary, c.Relevance, c.Importance, c.Scale, c.Geo, string(ents), c.SummaryVI, c.Model, time.Now().Unix()); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM article_topics WHERE article_id=?`, c.ArticleID); err != nil {
		return err
	}
	for _, t := range c.Topics {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO article_topics(article_id,topic,relevance) VALUES(?,?,?)`, c.ArticleID, t.Topic, t.Relevance); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE articles SET classified=1 WHERE id=?`, c.ArticleID); err != nil {
		return err
	}
	if c.SummaryVI != "" || len(c.Entities) > 0 {
		var title, summary string
		_ = tx.QueryRow(`SELECT title, summary FROM articles WHERE id=?`, c.ArticleID).Scan(&title, &summary)
		if _, err := tx.Exec(`DELETE FROM articles_fts WHERE rowid=?`, c.ArticleID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO articles_fts(rowid,title,summary,summary_vi,entities) VALUES(?,?,?,?,?)`,
			c.ArticleID, title, summary, c.SummaryVI, strings.Join(c.Entities, " ; ")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const viewCols = articleCols + `,c.primary_topic,c.relevance,c.importance,c.scale,c.geo,c.entities,c.summary_vi`

func scanView(sc interface{ Scan(...any) error }) (ArticleView, error) {
	var prim, scale, geo, ents, sum sql.NullString
	var rel sql.NullFloat64
	var imp sql.NullInt64
	a, err := scanArticle(sc, &prim, &rel, &imp, &scale, &geo, &ents, &sum)
	if err != nil {
		return ArticleView{}, err
	}
	v := ArticleView{Article: a}
	if prim.Valid || sum.Valid {
		c := &Classification{ArticleID: a.ID, Primary: prim.String, Relevance: rel.Float64, Importance: int(imp.Int64),
			Scale: scale.String, Geo: geo.String, SummaryVI: sum.String}
		_ = json.Unmarshal([]byte(ents.String), &c.Entities)
		v.Classification = c
	}
	return v, nil
}

func (s *Store) queryViews(q string, args ...any) ([]ArticleView, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArticleView
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.attachTopics(out)
}

func (s *Store) attachTopics(vs []ArticleView) error {
	for i := range vs {
		if vs[i].Classification == nil {
			continue
		}
		rows, err := s.db.Query(`SELECT topic, relevance FROM article_topics WHERE article_id=? ORDER BY relevance DESC`, vs[i].ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var t TopicScore
			if err := rows.Scan(&t.Topic, &t.Relevance); err != nil {
				rows.Close()
				return err
			}
			vs[i].Topics = append(vs[i].Topics, t)
		}
		rows.Close()
	}
	return nil
}

// TopicArticles: bài thuộc topic trong cửa sổ thời gian, xếp theo importance*relevance.
func (s *Store) TopicArticles(topic string, since, until time.Time, minRel float64, limit int) ([]ArticleView, error) {
	return s.queryViews(`SELECT `+viewCols+` FROM article_topics t
		JOIN articles a ON a.id=t.article_id JOIN classifications c ON c.article_id=a.id
		WHERE t.topic=? AND t.relevance>=? AND a.published_at>=? AND a.published_at<?
		ORDER BY (c.importance * t.relevance) DESC, a.published_at DESC LIMIT ?`, topic, minRel, since.Unix(), until.Unix(), limit)
}

func (s *Store) ArticlesByIDs(ids []int64) ([]ArticleView, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return s.queryViews(`SELECT `+viewCols+` FROM articles a LEFT JOIN classifications c ON c.article_id=a.id
		WHERE a.id IN (`+ph+`) ORDER BY a.published_at DESC`, args...)
}

type SearchOpts struct {
	Query  string // từ khoá người dùng (AND)
	Raw    string // biểu thức FTS5 dựng sẵn (ưu tiên hơn Query)
	Topic  string
	Since  time.Time
	Limit  int
	OnlyOn bool // chỉ bài có liên quan tới ít nhất 1 topic
}

// ftsQuery biến chuỗi người dùng thành truy vấn FTS5 an toàn (AND các từ; hỗ trợ OR).
func ftsQuery(q string) string {
	var parts []string
	for _, w := range strings.Fields(q) {
		if w == "OR" {
			parts = append(parts, "OR")
			continue
		}
		w = strings.ReplaceAll(w, `"`, "")
		if w != "" {
			parts = append(parts, `"`+w+`"`)
		}
	}
	return strings.Join(parts, " ")
}

func (s *Store) Search(o SearchOpts) ([]ArticleView, error) {
	if o.Limit <= 0 {
		o.Limit = 30
	}
	where := []string{"a.published_at >= ?"}
	args := []any{o.Since.Unix()}
	from := `articles a LEFT JOIN classifications c ON c.article_id=a.id`
	order := "a.published_at DESC"
	q := o.Raw
	if q == "" {
		q = ftsQuery(o.Query)
	}
	if q != "" {
		from = `articles_fts f JOIN articles a ON a.id=f.rowid LEFT JOIN classifications c ON c.article_id=a.id`
		where = append(where, "articles_fts MATCH ?")
		args = append(args, q)
		order = "bm25(articles_fts), a.published_at DESC"
	}
	if o.Topic != "" {
		where = append(where, "EXISTS (SELECT 1 FROM article_topics t WHERE t.article_id=a.id AND t.topic=?)")
		args = append(args, o.Topic)
	} else if o.OnlyOn {
		where = append(where, "EXISTS (SELECT 1 FROM article_topics t WHERE t.article_id=a.id)")
	}
	args = append(args, o.Limit)
	return s.queryViews(`SELECT `+viewCols+` FROM `+from+` WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order+` LIMIT ?`, args...)
}

// ---------- feeds ----------

type FeedStatus struct {
	Name, URL, Status string
	LastFetched       time.Time
	Items, New        int
}

func (s *Store) SaveFeedStatus(f FeedStatus) error {
	_, err := s.db.Exec(`INSERT INTO feeds(name,url,last_fetched,last_status,last_items,last_new) VALUES(?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET url=excluded.url,last_fetched=excluded.last_fetched,last_status=excluded.last_status,
		last_items=excluded.last_items,last_new=excluded.last_new`, f.Name, f.URL, f.LastFetched.Unix(), f.Status, f.Items, f.New)
	return err
}

func (s *Store) FeedStatuses() (map[string]FeedStatus, error) {
	rows, err := s.db.Query(`SELECT name,url,last_fetched,last_status,last_items,last_new FROM feeds`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FeedStatus{}
	for rows.Next() {
		var f FeedStatus
		var ts int64
		if err := rows.Scan(&f.Name, &f.URL, &ts, &f.Status, &f.Items, &f.New); err != nil {
			return nil, err
		}
		f.LastFetched = time.Unix(ts, 0)
		out[f.Name] = f
	}
	return out, rows.Err()
}

// ---------- storylines ----------

type Storyline struct {
	Slug       string `json:"slug"`
	Topic      string `json:"topic"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Level      string `json:"level"`
	Summary    string `json:"summary"`
	Why        string `json:"why_it_matters,omitempty"`
	FirstDate  string `json:"first_date"`
	LastDate   string `json:"last_date"`
	DaysActive int    `json:"days_active"`
}

type StorylineEvent struct {
	Date    string `json:"date"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"` // JSON đầy đủ của storyline hôm đó
}

func (s *Store) UpsertStoryline(sl Storyline, date string, detail string, articleIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lastDate string
	var days int
	err = tx.QueryRow(`SELECT last_date, days_active FROM storylines WHERE slug=?`, sl.Slug).Scan(&lastDate, &days)
	switch {
	case err == sql.ErrNoRows:
		_, err = tx.Exec(`INSERT INTO storylines(slug,topic,title,status,level,summary,why,first_date,last_date,days_active,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,1,?)`, sl.Slug, sl.Topic, sl.Title, sl.Status, sl.Level, sl.Summary, sl.Why, date, date, time.Now().Unix())
	case err == nil:
		if lastDate != date {
			days++
		}
		if date < lastDate {
			date = lastDate
		}
		_, err = tx.Exec(`UPDATE storylines SET title=?,status=?,level=?,summary=?,why=?,last_date=?,days_active=?,updated_at=? WHERE slug=?`,
			sl.Title, sl.Status, sl.Level, sl.Summary, sl.Why, date, days, time.Now().Unix(), sl.Slug)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO storyline_events(slug,date,status,summary,detail) VALUES(?,?,?,?,?)`,
		sl.Slug, date, sl.Status, sl.Summary, detail); err != nil {
		return err
	}
	for _, id := range articleIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO storyline_articles(slug,article_id,date) VALUES(?,?,?)`, sl.Slug, id, date); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetTopicDate xoá kết quả storyline của 1 topic trong 1 ngày (trước khi phân tích lại ngày đó).
func (s *Store) ResetTopicDate(topic, date string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`DELETE FROM storyline_articles WHERE date=? AND slug IN (SELECT slug FROM storylines WHERE topic=?)`,
		`DELETE FROM storyline_events WHERE date=? AND slug IN (SELECT slug FROM storylines WHERE topic=?)`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, date, topic); err != nil {
			return err
		}
	}
	// storyline chỉ sinh ra trong ngày này → xoá hẳn; storyline cũ → khôi phục last_date/days_active
	if _, err := tx.Exec(`DELETE FROM storylines WHERE topic=? AND NOT EXISTS (SELECT 1 FROM storyline_events e WHERE e.slug=storylines.slug)`, topic); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE storylines SET
		last_date=(SELECT max(date) FROM storyline_events e WHERE e.slug=storylines.slug),
		days_active=(SELECT count(*) FROM storyline_events e WHERE e.slug=storylines.slug)
		WHERE topic=? AND last_date>=?`, topic, date); err != nil {
		return err
	}
	return tx.Commit()
}

const slCols = `slug,topic,title,status,level,summary,why,first_date,last_date,days_active`

func (s *Store) scanStorylines(q string, args ...any) ([]Storyline, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Storyline
	for rows.Next() {
		var sl Storyline
		if err := rows.Scan(&sl.Slug, &sl.Topic, &sl.Title, &sl.Status, &sl.Level, &sl.Summary, &sl.Why, &sl.FirstDate, &sl.LastDate, &sl.DaysActive); err != nil {
			return nil, err
		}
		out = append(out, sl)
	}
	return out, rows.Err()
}

// Storylines: storyline có hoạt động từ sinceDate (YYYY-MM-DD); topic rỗng = mọi topic.
func (s *Store) Storylines(topic, sinceDate string, limit int) ([]Storyline, error) {
	if limit <= 0 {
		limit = 200
	}
	if topic == "" {
		return s.scanStorylines(`SELECT `+slCols+` FROM storylines WHERE last_date>=? ORDER BY last_date DESC, days_active DESC LIMIT ?`, sinceDate, limit)
	}
	return s.scanStorylines(`SELECT `+slCols+` FROM storylines WHERE topic=? AND last_date>=? ORDER BY last_date DESC, days_active DESC LIMIT ?`, topic, sinceDate, limit)
}

func (s *Store) StorylinesOn(date string) ([]Storyline, error) {
	return s.scanStorylines(`SELECT `+slCols+` FROM storylines WHERE slug IN (SELECT slug FROM storyline_events WHERE date=?) ORDER BY topic`, date)
}

func (s *Store) Storyline(slug string) (*Storyline, error) {
	out, err := s.scanStorylines(`SELECT `+slCols+` FROM storylines WHERE slug=?`, slug)
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return &out[0], nil
}

func (s *Store) StorylineEvents(slug string) ([]StorylineEvent, error) {
	rows, err := s.db.Query(`SELECT date,status,summary,detail FROM storyline_events WHERE slug=? ORDER BY date`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorylineEvent
	for rows.Next() {
		var e StorylineEvent
		if err := rows.Scan(&e.Date, &e.Status, &e.Summary, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) StorylineArticleIDs(slug string) ([]int64, error) {
	rows, err := s.db.Query(`SELECT article_id FROM storyline_articles WHERE slug=? ORDER BY date`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---------- reports ----------

type Report struct {
	Date, Kind, JSON, Markdown, Model string
	CreatedAt                         time.Time
}

func (s *Store) SaveReport(r Report) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO reports(date,kind,json,markdown,model,created_at) VALUES(?,?,?,?,?,?)`,
		r.Date, r.Kind, r.JSON, r.Markdown, r.Model, time.Now().Unix())
	return err
}

func (s *Store) GetReport(date, kind string) (*Report, error) {
	r := &Report{}
	var ts int64
	err := s.db.QueryRow(`SELECT date,kind,json,markdown,model,created_at FROM reports WHERE date=? AND kind=?`, date, kind).
		Scan(&r.Date, &r.Kind, &r.JSON, &r.Markdown, &r.Model, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	r.CreatedAt = time.Unix(ts, 0)
	return r, err
}

// ReportsByKindPrefix liệt kê report theo prefix kind (vd "deep:"), mới nhất trước.
func (s *Store) ReportsByKindPrefix(prefix string, limit int) ([]Report, error) {
	rows, err := s.db.Query(`SELECT date,kind,model,created_at FROM reports WHERE kind LIKE ? ORDER BY created_at DESC LIMIT ?`, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		var r Report
		var ts int64
		if err := rows.Scan(&r.Date, &r.Kind, &r.Model, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(ts, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ReportDates(kind string, limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT date FROM reports WHERE kind=? ORDER BY date DESC LIMIT ?`, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------- llm call log & stats ----------

func (s *Store) LogLLMCall(purpose, model string, cost float64, dur time.Duration, callErr error) {
	errStr := ""
	if callErr != nil {
		errStr = callErr.Error()
		if len(errStr) > 500 {
			errStr = errStr[:500]
		}
	}
	ok := 1
	if callErr != nil {
		ok = 0
	}
	_, _ = s.db.Exec(`INSERT INTO llm_calls(ts,purpose,model,cost_usd,duration_ms,ok,err) VALUES(?,?,?,?,?,?,?)`,
		time.Now().Unix(), purpose, model, cost, dur.Milliseconds(), ok, errStr)
}

type Stats struct {
	Articles, Classified, Relevant, Storylines, Reports int
	TopicCounts                                         map[string]int
	Cost24h, Cost30d                                    float64
	Calls24h, Fail24h                                   int
}

func (s *Store) Stats(since time.Time) (Stats, error) {
	st := Stats{TopicCounts: map[string]int{}}
	q := func(dst *int, sql string, args ...any) { _ = s.db.QueryRow(sql, args...).Scan(dst) }
	q(&st.Articles, `SELECT count(*) FROM articles`)
	q(&st.Classified, `SELECT count(*) FROM articles WHERE classified=1`)
	q(&st.Relevant, `SELECT count(DISTINCT article_id) FROM article_topics`)
	q(&st.Storylines, `SELECT count(*) FROM storylines`)
	q(&st.Reports, `SELECT count(DISTINCT date) FROM reports WHERE kind='daily'`)
	q(&st.Calls24h, `SELECT count(*) FROM llm_calls WHERE ts>=?`, time.Now().Add(-24*time.Hour).Unix())
	q(&st.Fail24h, `SELECT count(*) FROM llm_calls WHERE ok=0 AND ts>=?`, time.Now().Add(-24*time.Hour).Unix())
	_ = s.db.QueryRow(`SELECT COALESCE(sum(cost_usd),0) FROM llm_calls WHERE ts>=?`, time.Now().Add(-24*time.Hour).Unix()).Scan(&st.Cost24h)
	_ = s.db.QueryRow(`SELECT COALESCE(sum(cost_usd),0) FROM llm_calls WHERE ts>=?`, time.Now().Add(-30*24*time.Hour).Unix()).Scan(&st.Cost30d)
	rows, err := s.db.Query(`SELECT t.topic, count(*) FROM article_topics t JOIN articles a ON a.id=t.article_id
		WHERE a.published_at>=? AND t.relevance>=0.5 GROUP BY t.topic`, since.Unix())
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return st, err
		}
		st.TopicCounts[t] = n
	}
	return st, rows.Err()
}
