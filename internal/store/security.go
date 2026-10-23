package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Bảng cho bản đồ mối đe doạ (fa sec): nội dung bài gốc, sự kiện an ninh trích xuất, CISA KEV, leak site ransomware.
const secSchema = `
CREATE TABLE IF NOT EXISTS article_text (
  article_id INTEGER PRIMARY KEY, status TEXT, text TEXT, fetched_at INTEGER
);
CREATE TABLE IF NOT EXISTS sec_events (
  article_id INTEGER PRIMARY KEY, incident_key TEXT, kind TEXT, date TEXT, detail TEXT, model TEXT, created_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_sec_key ON sec_events(incident_key);
CREATE INDEX IF NOT EXISTS idx_sec_date ON sec_events(date);
CREATE TABLE IF NOT EXISTS kev (
  cve TEXT PRIMARY KEY, vendor TEXT, product TEXT, name TEXT, date_added TEXT, description TEXT,
  action TEXT, due_date TEXT, ransomware TEXT, cwes TEXT
);
CREATE TABLE IF NOT EXISTS epss (cve TEXT PRIMARY KEY, score REAL, pct REAL, at INTEGER);
CREATE INDEX IF NOT EXISTS idx_kev_added ON kev(date_added);
CREATE TABLE IF NOT EXISTS ransom_posts (
  id TEXT PRIMARY KEY, grp TEXT, discovered INTEGER, industry TEXT
);
CREATE INDEX IF NOT EXISTS idx_ransom_disc ON ransom_posts(discovered);
`

// ---------- nội dung bài gốc ----------

// ArticleText trả về nội dung đã tải (status: ok | fail); ok=false nếu chưa tải.
func (s *Store) ArticleText(id int64) (text, status string, ok bool) {
	err := s.db.QueryRow(`SELECT text, status FROM article_text WHERE article_id=?`, id).Scan(&text, &status)
	return text, status, err == nil
}

func (s *Store) SaveArticleText(id int64, status, text string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO article_text(article_id,status,text,fetched_at) VALUES(?,?,?,?)`,
		id, status, text, time.Now().Unix())
	return err
}

// ---------- sự kiện an ninh ----------

type SecEvent struct {
	ArticleID int64
	Key       string // incident_key: gom nhiều bài cùng một sự cố/chiến dịch
	Kind      string
	Date      string // ngày đăng bài (YYYY-MM-DD theo timezone cấu hình)
	Detail    string // JSON đầy đủ trích xuất
	Title     string
	Link      string
	Source    string
	Region    string
	Published time.Time
}

// SecPending: bài thuộc topic an ninh (relevance >= minRel) từ since, chưa trích xuất.
func (s *Store) SecPending(topic string, minRel float64, since time.Time, limit int) ([]ArticleView, error) {
	return s.queryViews(`SELECT `+viewCols+` FROM article_topics t
		JOIN articles a ON a.id=t.article_id JOIN classifications c ON c.article_id=a.id
		WHERE t.topic=? AND t.relevance>=? AND a.published_at>=?
		AND NOT EXISTS (SELECT 1 FROM sec_events e WHERE e.article_id=a.id)
		ORDER BY a.published_at ASC LIMIT ?`, topic, minRel, since.Unix(), limit)
}

func (s *Store) CountSecPending(topic string, minRel float64, since time.Time) int {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM article_topics t JOIN articles a ON a.id=t.article_id
		WHERE t.topic=? AND t.relevance>=? AND a.published_at>=?
		AND NOT EXISTS (SELECT 1 FROM sec_events e WHERE e.article_id=a.id)`, topic, minRel, since.Unix()).Scan(&n)
	return n
}

func (s *Store) SaveSecEvent(e SecEvent, model string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO sec_events(article_id,incident_key,kind,date,detail,model,created_at) VALUES(?,?,?,?,?,?,?)`,
		e.ArticleID, e.Key, e.Kind, e.Date, e.Detail, model, time.Now().Unix())
	return err
}

// DeleteSecEvents xoá kết quả trích xuất từ sinceDate (để trích xuất lại).
func (s *Store) DeleteSecEvents(sinceDate string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sec_events WHERE date>=?`, sinceDate)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SecEvents: sự kiện an ninh từ sinceDate, kèm thông tin bài (cũ trước).
func (s *Store) SecEvents(sinceDate string) ([]SecEvent, error) {
	rows, err := s.db.Query(`SELECT e.article_id, e.incident_key, e.kind, e.date, e.detail, a.title, a.link, a.source, a.region, a.published_at
		FROM sec_events e JOIN articles a ON a.id=e.article_id WHERE e.date>=? ORDER BY a.published_at`, sinceDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecEvent
	for rows.Next() {
		var e SecEvent
		var pub int64
		if err := rows.Scan(&e.ArticleID, &e.Key, &e.Kind, &e.Date, &e.Detail, &e.Title, &e.Link, &e.Source, &e.Region, &pub); err != nil {
			return nil, err
		}
		e.Published = time.Unix(pub, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

type IncidentRef struct {
	Key, Title, LastDate string
	Articles             int
}

// RecentIncidents: incident_key đã thấy từ sinceDate (mới nhất trước) — để Claude dùng lại khi bài mới nói về cùng sự cố.
func (s *Store) RecentIncidents(sinceDate string, limit int) ([]IncidentRef, error) {
	rows, err := s.db.Query(`SELECT incident_key, max(date), count(*), (SELECT detail FROM sec_events x WHERE x.incident_key=e.incident_key ORDER BY date DESC LIMIT 1)
		FROM sec_events e WHERE date>=? AND incident_key<>'' AND kind IN ('incident','campaign','vulnerability')
		GROUP BY incident_key ORDER BY max(date) DESC, count(*) DESC LIMIT ?`, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IncidentRef
	for rows.Next() {
		var r IncidentRef
		var detail sql.NullString
		if err := rows.Scan(&r.Key, &r.LastDate, &r.Articles, &detail); err != nil {
			return nil, err
		}
		var d struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal([]byte(detail.String), &d)
		r.Title = d.Title
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- CISA KEV ----------

type KEV struct {
	CVE         string   `json:"cve"`
	Vendor      string   `json:"vendor"`
	Product     string   `json:"product"`
	Name        string   `json:"name"`
	DateAdded   string   `json:"date_added"`
	Description string   `json:"description,omitempty"`
	Action      string   `json:"action,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
	Ransomware  string   `json:"ransomware"` // Known | Unknown
	CWEs        []string `json:"cwes,omitempty"`
	EPSS        float64  `json:"epss"`     // xác suất bị khai thác trong 30 ngày (0..1)
	EPSSPct     float64  `json:"epss_pct"` // percentile (0..1)
}

// UpsertKEV ghi đè danh mục KEV (giữ điểm EPSS đã có). Trả về số CVE mới.
func (s *Store) UpsertKEV(items []KEV) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, k := range items {
		cwes, _ := json.Marshal(k.CWEs)
		res, err := tx.Exec(`INSERT OR IGNORE INTO kev(cve,vendor,product,name,date_added,description,action,due_date,ransomware,cwes) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			k.CVE, k.Vendor, k.Product, k.Name, k.DateAdded, k.Description, k.Action, k.DueDate, k.Ransomware, string(cwes))
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
			continue
		}
		if _, err := tx.Exec(`UPDATE kev SET vendor=?,product=?,name=?,date_added=?,description=?,action=?,due_date=?,ransomware=?,cwes=? WHERE cve=?`,
			k.Vendor, k.Product, k.Name, k.DateAdded, k.Description, k.Action, k.DueDate, k.Ransomware, string(cwes), k.CVE); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

// KEVSince: CVE được CISA thêm vào KEV từ sinceDate (mới nhất trước).
func (s *Store) KEVSince(sinceDate string) ([]KEV, error) {
	rows, err := s.db.Query(`SELECT k.cve,k.vendor,k.product,k.name,k.date_added,k.description,k.action,k.due_date,k.ransomware,k.cwes,
		COALESCE(e.score,0),COALESCE(e.pct,0) FROM kev k LEFT JOIN epss e ON e.cve=k.cve
		WHERE k.date_added>=? ORDER BY k.date_added DESC, k.cve DESC`, sinceDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KEV
	for rows.Next() {
		var k KEV
		var cwes string
		if err := rows.Scan(&k.CVE, &k.Vendor, &k.Product, &k.Name, &k.DateAdded, &k.Description, &k.Action, &k.DueDate, &k.Ransomware, &cwes, &k.EPSS, &k.EPSSPct); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cwes), &k.CWEs)
		out = append(out, k)
	}
	return out, rows.Err()
}

// KEVByCVE tra nhanh các CVE có trong KEV.
func (s *Store) KEVByCVE(cves []string) (map[string]KEV, error) {
	out := map[string]KEV{}
	if len(cves) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(cves)), ",")
	args := make([]any, len(cves))
	for i, c := range cves {
		args[i] = c
	}
	rows, err := s.db.Query(`SELECT k.cve,k.vendor,k.product,k.date_added,k.ransomware,COALESCE(e.score,0),COALESCE(e.pct,0)
		FROM kev k LEFT JOIN epss e ON e.cve=k.cve WHERE k.cve IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k KEV
		if err := rows.Scan(&k.CVE, &k.Vendor, &k.Product, &k.DateAdded, &k.Ransomware, &k.EPSS, &k.EPSSPct); err != nil {
			return nil, err
		}
		out[k.CVE] = k
	}
	return out, rows.Err()
}

func (s *Store) KEVCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM kev`).Scan(&n)
	return n
}

// EPSSStale: trong danh sách cves, các CVE chưa có điểm EPSS hoặc điểm cũ hơn staleBefore.
func (s *Store) EPSSStale(cves []string, staleBefore time.Time) []string {
	var out []string
	for _, c := range cves {
		var at int64
		if err := s.db.QueryRow(`SELECT at FROM epss WHERE cve=?`, c).Scan(&at); err != nil || at < staleBefore.Unix() {
			out = append(out, c)
		}
	}
	return out
}

// SetEPSS lưu điểm EPSS: cve → {score, percentile}.
func (s *Store) SetEPSS(scores map[string][2]float64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for cve, v := range scores {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO epss(cve,score,pct,at) VALUES(?,?,?,?)`, cve, v[0], v[1], now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EPSS tra điểm đã lưu cho danh sách CVE.
func (s *Store) EPSS(cves []string) map[string][2]float64 {
	out := map[string][2]float64{}
	for _, c := range cves {
		var sc, pct float64
		if err := s.db.QueryRow(`SELECT score, pct FROM epss WHERE cve=?`, c).Scan(&sc, &pct); err == nil {
			out[c] = [2]float64{sc, pct}
		}
	}
	return out
}

// ---------- leak site ransomware ----------

// RansomPost: 1 bài đăng nạn nhân trên leak site. Chỉ giữ nhóm, thời điểm, ngành — không lưu tên nạn nhân.
type RansomPost struct {
	ID         string    `json:"-"`
	Group      string    `json:"group"`
	Discovered time.Time `json:"-"`
	Industry   string    `json:"industry"`
}

func (s *Store) InsertRansomPosts(ps []RansomPost) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, p := range ps {
		var one int
		if tx.QueryRow(`SELECT 1 FROM ransom_posts WHERE id=?`, p.ID).Scan(&one) != nil {
			n++
		}
		// cập nhật ngành nếu bài đã có (luật ánh xạ có thể được cải thiện)
		if _, err := tx.Exec(`INSERT INTO ransom_posts(id,grp,discovered,industry) VALUES(?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET industry=excluded.industry`, p.ID, p.Group, p.Discovered.Unix(), p.Industry); err != nil {
			return 0, err
		}
	}
	return n, tx.Commit()
}

func (s *Store) RansomPosts(since time.Time) ([]RansomPost, error) {
	rows, err := s.db.Query(`SELECT id,grp,discovered,industry FROM ransom_posts WHERE discovered>=? ORDER BY discovered`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RansomPost
	for rows.Next() {
		var p RansomPost
		var ts int64
		if err := rows.Scan(&p.ID, &p.Group, &ts, &p.Industry); err != nil {
			return nil, err
		}
		p.Discovered = time.Unix(ts, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}
