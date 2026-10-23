package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/research"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func setup(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	home := t.TempDir()
	cfg, err := config.Parse(config.DefaultYAML, home)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	a := &store.Article{Hash: "h", Feed: "f", Source: "x.vn", Region: "vn", Title: "Lãi suất", Link: "https://x.vn/1", Published: time.Now(), Fetched: time.Now()}
	if _, err := st.InsertArticle(a); err != nil {
		t.Fatal(err)
	}
	_ = st.SaveClassification(store.Classification{ArticleID: a.ID, Topics: []store.TopicScore{{Topic: "economy", Relevance: 0.9}}, Primary: "economy",
		Relevance: 0.9, Importance: 4, Scale: "macro", Geo: "vn", Entities: []string{"NHNN"}, SummaryVI: "x"})
	date := cfg.Today()
	sl := analyze.StorylineOut{Slug: "vn-rate", Title: "Lãi suất VN", Status: "new", Level: "macro", Importance: 4, Summary: "s", Confidence: "medium",
		CausalChain: []analyze.ChainStep{{Level: "micro", Step: "a"}, {Level: "macro", Step: "b", Channel: "tín dụng"}}, ArticleIDs: []int64{a.ID}}
	det, _ := json.Marshal(sl)
	_ = st.UpsertStoryline(store.Storyline{Slug: sl.Slug, Topic: "economy", Title: sl.Title, Status: "new", Level: "macro", Summary: "s"}, date, string(det), sl.ArticleIDs)
	d := analyze.Daily{Date: date, Topics: []analyze.TopicAnalysis{{Topic: "economy", Headline: "h", ArticleCount: 1, Storylines: []analyze.StorylineOut{sl}}},
		Synthesis: &analyze.Synthesis{Title: "T", CrossLinks: []analyze.CrossLink{{FromTopic: "economy", FromStory: "vn-rate", ToTopic: "politics", ToStory: "khái niệm", Mechanism: "m", Implication: "i"}},
			VNImpact: []analyze.VNImpact{{Area: "Ngân hàng", Impact: "x", Direction: "negative", Horizon: "short", Confidence: "low"}}}}
	b, _ := json.Marshal(d)
	_ = st.SaveReport(store.Report{Date: date, Kind: "daily", JSON: string(b)})
	_ = st.SaveReport(store.Report{Date: date, Kind: "deep:vn-rate", Markdown: "# Báo cáo\n\nnội dung"})
	viz := research.DeepViz{Headline: "H", KeyMetrics: []research.VizMetric{{Label: "USD", Value: "25.830", Unit: "đ"}},
		CausalNodes: []research.VizNode{{ID: "n1", Label: "a", Level: "micro"}, {ID: "n2", Label: "b", Level: "macro"}},
		CausalEdges: []research.VizEdge{{From: "n1", To: "n2", Channel: "c", Strength: 2}},
		Scenarios:   []research.VizScenario{{Name: "Cơ sở", Probability: 60}},
		Indicators:  []research.VizIndic{{Name: "CPI", Status: "alert"}}}
	vb, _ := json.Marshal(viz)
	_ = st.SaveReport(store.Report{Date: date, Kind: "deepviz:vn-rate", JSON: string(vb)})

	s := New(cfg, st)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestPages(t *testing.T) {
	srv, s := setup(t)
	date := s.cfg.Today()
	cases := []struct {
		path string
		want []string
	}{
		{"/dashboard?days=7", []string{`data-viz="volume"`, `data-viz="storyheat"`, `data-viz="entities"`, `data-viz="facet"`, "Bài liên quan"}},
		{"/r/" + date, []string{`data-viz="storymap"`, `data-viz="network"`, `class="matrix"`, `class="chain"`, "Đọc bản tin"}},
		{"/deep/" + date + "/vn-rate", []string{`data-viz="causal"`, `data-viz="scenarios"`, "25.830", "cảnh báo", "Báo cáo đầy đủ"}},
		{"/deep", []string{"vn-rate", "📊 có"}},
		{"/s/vn-rate", []string{`class="chain"`, "tín dụng"}},
		{"/healthz", []string{HealthToken}},
		{"/static/viz.js", []string{"R.causal"}},
		{"/static/echarts.min.js", []string{"Apache"}},
	}
	for _, c := range cases {
		code, body := get(t, srv.URL+c.path)
		if code != 200 {
			t.Errorf("%s: HTTP %d", c.path, code)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: thiếu %q", c.path, w)
			}
		}
	}
	// JSON nhúng phải parse được (không vỡ bởi ký tự đặc biệt)
	_, body := get(t, srv.URL+"/r/"+date)
	if strings.Count(body, `type="application/json"`) < 3 {
		t.Error("thiếu khối dữ liệu JSON cho chart")
	}
}

func TestShutdownOnlyWhenEnabled(t *testing.T) {
	srv, s := setup(t)
	resp, _ := http.Post(srv.URL+"/api/shutdown", "text/plain", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("không có Shutdown hook phải 403, got %d", resp.StatusCode)
	}
	called := make(chan bool, 1)
	s.Shutdown = func() { called <- true }
	resp, _ = http.Post(srv.URL+"/api/shutdown", "text/plain", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown không được gọi")
	}
}
