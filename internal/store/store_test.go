package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func addArticle(t *testing.T, s *Store, hash, title string, pub time.Time) int64 {
	t.Helper()
	a := &Article{Hash: hash, Feed: "f", Source: "x.vn", Region: "vn", Lang: "vi", Title: title, Link: "https://x.vn/" + hash, Published: pub, Fetched: time.Now()}
	ok, err := s.InsertArticle(a)
	if err != nil || !ok {
		t.Fatalf("insert: ok=%v err=%v", ok, err)
	}
	return a.ID
}

func TestInsertDedupAndClassify(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	id := addArticle(t, s, "h1", "Ngân hàng Nhà nước giữ nguyên lãi suất điều hành", now)
	if ok, _ := s.InsertArticle(&Article{Hash: "h1", Title: "dup", Published: now, Fetched: now}); ok {
		t.Fatal("bài trùng hash không được chèn lại")
	}
	if n := s.CountUnclassified(); n != 1 {
		t.Fatalf("unclassified=%d", n)
	}
	err := s.SaveClassification(Classification{ArticleID: id, Topics: []TopicScore{{"economy", 0.9}}, Primary: "economy", Relevance: 0.9,
		Importance: 4, Scale: "macro", Geo: "vn", Entities: []string{"NHNN"}, SummaryVI: "NHNN giữ lãi suất tái cấp vốn 4,5%."})
	if err != nil {
		t.Fatal(err)
	}
	if n := s.CountUnclassified(); n != 0 {
		t.Fatalf("unclassified=%d sau khi phân loại", n)
	}
	vs, err := s.TopicArticles("economy", now.Add(-time.Hour), now.Add(time.Hour), 0.5, 10)
	if err != nil || len(vs) != 1 || vs[0].Importance != 4 || len(vs[0].Topics) != 1 {
		t.Fatalf("TopicArticles: %+v err=%v", vs, err)
	}
}

func TestSearchDiacriticInsensitive(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	addArticle(t, s, "a", "Lãi suất huy động tăng mạnh", now)
	addArticle(t, s, "b", "Giá vàng lập đỉnh", now)
	for _, q := range []string{"lai suat", "lãi suất", "LÃI"} {
		vs, err := s.Search(SearchOpts{Query: q, Since: now.Add(-time.Hour)})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if len(vs) != 1 || vs[0].Title != "Lãi suất huy động tăng mạnh" {
			t.Fatalf("%q: got %d kết quả", q, len(vs))
		}
	}
	// ký tự đặc biệt FTS không được làm vỡ truy vấn
	if _, err := s.Search(SearchOpts{Query: `"AI" (agent) OR -x*`, Since: now.Add(-time.Hour)}); err != nil {
		t.Fatalf("query đặc biệt lỗi: %v", err)
	}
}

func TestStorylineLifecycle(t *testing.T) {
	s := newStore(t)
	id := addArticle(t, s, "a", "t", time.Now())
	sl := Storyline{Slug: "fed-rate", Topic: "economy", Title: "Fed", Status: "new", Level: "macro", Summary: "s1"}
	if err := s.UpsertStoryline(sl, "2026-10-08", "{}", []int64{id}); err != nil {
		t.Fatal(err)
	}
	sl.Status, sl.Summary = "developing", "s2"
	if err := s.UpsertStoryline(sl, "2026-10-09", "{}", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Storyline("fed-rate")
	if got.DaysActive != 2 || got.FirstDate != "2026-10-08" || got.LastDate != "2026-10-09" || got.Summary != "s2" {
		t.Fatalf("sau 2 ngày: %+v", got)
	}
	// chạy lại phân tích ngày 09: reset rồi upsert lại không được đếm trùng ngày
	if err := s.ResetTopicDate("economy", "2026-10-09"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Storyline("fed-rate")
	if got.DaysActive != 1 || got.LastDate != "2026-10-08" {
		t.Fatalf("sau reset: %+v", got)
	}
	if err := s.UpsertStoryline(sl, "2026-10-09", "{}", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Storyline("fed-rate")
	if got.DaysActive != 2 {
		t.Fatalf("sau chạy lại: days=%d", got.DaysActive)
	}
	// storyline chỉ sinh ra trong ngày bị reset phải bị xoá
	_ = s.UpsertStoryline(Storyline{Slug: "only-today", Topic: "economy", Title: "x", Status: "new"}, "2026-10-10", "{}", nil)
	_ = s.ResetTopicDate("economy", "2026-10-10")
	if x, _ := s.Storyline("only-today"); x != nil {
		t.Fatal("storyline mồ côi chưa bị xoá")
	}
	evs, _ := s.StorylineEvents("fed-rate")
	if len(evs) != 2 {
		t.Fatalf("events=%d", len(evs))
	}
}

func TestReports(t *testing.T) {
	s := newStore(t)
	_ = s.SaveReport(Report{Date: "2026-10-09", Kind: "daily", JSON: "{}"})
	_ = s.SaveReport(Report{Date: "2026-10-10", Kind: "daily", JSON: "{}"})
	_ = s.SaveReport(Report{Date: "2026-10-10", Kind: "deep:x", Markdown: "# x"})
	ds, _ := s.ReportDates("daily", 10)
	if len(ds) != 2 || ds[0] != "2026-10-10" {
		t.Fatalf("dates=%v", ds)
	}
	r, _ := s.GetReport("2026-10-10", "deep:x")
	if r == nil || r.Markdown != "# x" {
		t.Fatal("GetReport deep")
	}
	if r, _ := s.GetReport("2000-01-01", "daily"); r != nil {
		t.Fatal("phải trả nil khi không có")
	}
}
