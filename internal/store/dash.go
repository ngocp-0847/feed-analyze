package store

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Các truy vấn tổng hợp cho dashboard. Gom theo ngày ở Go để dùng đúng timezone cấu hình.

type DayTopicCount struct {
	Day   string `json:"day"`
	Topic string `json:"topic"`
	N     int    `json:"n"`
}

// TopicVolume: số bài liên quan (relevance >= minRel) theo ngày × topic.
func (s *Store) TopicVolume(since time.Time, minRel float64, loc *time.Location) ([]DayTopicCount, error) {
	rows, err := s.db.Query(`SELECT a.published_at, t.topic FROM article_topics t JOIN articles a ON a.id=t.article_id
		WHERE a.published_at>=? AND t.relevance>=?`, since.Unix(), minRel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[[2]string]int{}
	for rows.Next() {
		var ts int64
		var topic string
		if err := rows.Scan(&ts, &topic); err != nil {
			return nil, err
		}
		agg[[2]string{time.Unix(ts, 0).In(loc).Format("2006-01-02"), topic}]++
	}
	out := make([]DayTopicCount, 0, len(agg))
	for k, n := range agg {
		out = append(out, DayTopicCount{Day: k[0], Topic: k[1], N: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, rows.Err()
}

type TopicFacet struct {
	Topic string `json:"topic"`
	Key   string `json:"key"`
	N     int    `json:"n"`
}

// TopicFacets: phân bố theo cột phân loại (scale | geo | importance) cho từng topic.
func (s *Store) TopicFacets(column string, since time.Time, minRel float64) ([]TopicFacet, error) {
	switch column {
	case "scale", "geo", "importance":
	default:
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT t.topic, CAST(c.`+column+` AS TEXT), count(*) FROM article_topics t
		JOIN articles a ON a.id=t.article_id JOIN classifications c ON c.article_id=a.id
		WHERE a.published_at>=? AND t.relevance>=? GROUP BY 1,2`, since.Unix(), minRel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopicFacet
	for rows.Next() {
		var f TopicFacet
		if err := rows.Scan(&f.Topic, &f.Key, &f.N); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type EntityCount struct {
	Name   string         `json:"name"`
	N      int            `json:"n"`
	Topics map[string]int `json:"topics"`
}

// TopEntities: thực thể được nhắc nhiều nhất trong bài liên quan.
func (s *Store) TopEntities(since time.Time, minRel float64, limit int) ([]EntityCount, error) {
	rows, err := s.db.Query(`SELECT c.entities, c.primary_topic FROM classifications c JOIN articles a ON a.id=c.article_id
		WHERE a.published_at>=? AND c.relevance>=? AND c.entities IS NOT NULL`, since.Unix(), minRel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[string]*EntityCount{}
	display := map[string]string{}
	for rows.Next() {
		var raw, topic string
		if err := rows.Scan(&raw, &topic); err != nil {
			return nil, err
		}
		var ents []string
		if json.Unmarshal([]byte(raw), &ents) != nil {
			continue
		}
		seen := map[string]bool{}
		for _, e := range ents {
			e = strings.TrimSpace(e)
			key := strings.ToLower(e)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			if agg[key] == nil {
				agg[key] = &EntityCount{Topics: map[string]int{}}
				display[key] = e
			}
			agg[key].N++
			agg[key].Topics[topic]++
		}
	}
	out := make([]EntityCount, 0, len(agg))
	for k, v := range agg {
		v.Name = display[k]
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

type StorylineDay struct {
	Slug       string `json:"slug"`
	Date       string `json:"date"`
	Status     string `json:"status"`
	Importance int    `json:"importance"`
	Articles   int    `json:"articles"`
}

// StorylineDays: hoạt động theo ngày của storyline từ sinceDate (để vẽ ma trận storyline × ngày).
func (s *Store) StorylineDays(sinceDate string) ([]StorylineDay, error) {
	rows, err := s.db.Query(`SELECT e.slug, e.date, e.status, e.detail,
		(SELECT count(*) FROM storyline_articles sa WHERE sa.slug=e.slug AND sa.date=e.date)
		FROM storyline_events e WHERE e.date>=? ORDER BY e.date`, sinceDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorylineDay
	for rows.Next() {
		var d StorylineDay
		var detail string
		if err := rows.Scan(&d.Slug, &d.Date, &d.Status, &detail, &d.Articles); err != nil {
			return nil, err
		}
		var x struct {
			Importance int `json:"importance"`
		}
		_ = json.Unmarshal([]byte(detail), &x)
		d.Importance = x.Importance
		out = append(out, d)
	}
	return out, rows.Err()
}

type DayCost struct {
	Day   string  `json:"day"`
	Cost  float64 `json:"cost"`
	Calls int     `json:"calls"`
	Fails int     `json:"fails"`
}

// CostByDay: chi phí Claude theo ngày.
func (s *Store) CostByDay(since time.Time, loc *time.Location) ([]DayCost, error) {
	rows, err := s.db.Query(`SELECT ts, cost_usd, ok FROM llm_calls WHERE ts>=?`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[string]*DayCost{}
	for rows.Next() {
		var ts int64
		var cost float64
		var ok int
		if err := rows.Scan(&ts, &cost, &ok); err != nil {
			return nil, err
		}
		day := time.Unix(ts, 0).In(loc).Format("2006-01-02")
		if agg[day] == nil {
			agg[day] = &DayCost{Day: day}
		}
		agg[day].Cost += cost
		agg[day].Calls++
		if ok == 0 {
			agg[day].Fails++
		}
	}
	out := make([]DayCost, 0, len(agg))
	for _, v := range agg {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, rows.Err()
}
