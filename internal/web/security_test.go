package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/security"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func TestSecurityPage(t *testing.T) {
	srv, s := setup(t)
	// trang trống vẫn render, có hướng dẫn
	if code, body := get(t, srv.URL+"/security"); code != 200 || !strings.Contains(body, "fa sec backfill") {
		t.Fatalf("trang trống: HTTP %d", code)
	}
	a := &store.Article{Hash: "sec1", Feed: "The Hacker News", Source: "thehackernews.com", Region: "intl", Title: "Lazarus npm",
		Link: "https://thn.example/1", Published: time.Now(), Fetched: time.Now()}
	if _, err := s.st.InsertArticle(a); err != nil {
		t.Fatal(err)
	}
	ev := security.Event{Kind: "campaign", IncidentKey: "lazarus-npm", Title: "Lazarus đầu độc gói npm </script>", Severity: 4,
		AttackTypes: []string{"malicious_package"}, Actor: security.Actor{Name: "Lazarus", Origin: "KP", Motivation: "financial", Attribution: "confirmed"},
		Targets: []string{"VN"}, Sectors: []string{"developers"}, Techniques: []security.Technique{{ID: "T1195.002", Name: "Compromise Software Supply Chain", Tactic: "initial-access"}}}
	det, _ := json.Marshal(ev)
	if err := s.st.SaveSecEvent(store.SecEvent{ArticleID: a.ID, Key: ev.IncidentKey, Kind: ev.Kind, Date: s.cfg.Today(), Detail: string(det)}, "m"); err != nil {
		t.Fatal(err)
	}
	br := security.Brief{Days: 30, Headline: "Chuỗi cung ứng là mặt trận chính", ThreatLevel: "high",
		Controls: []security.Control{{Control: "Ghim phiên bản dependency", Layer: "dependencies", Priority: "P0", Effort: "S", Impact: 5, Counters: []string{"T1195.002"}}}}
	bj, _ := json.Marshal(br)
	_ = s.st.SaveReport(store.Report{Date: s.cfg.Today(), Kind: "secbrief", JSON: string(bj)})

	code, body := get(t, srv.URL+"/security/30")
	if code != 200 {
		t.Fatalf("HTTP %d", code)
	}
	for _, w := range []string{`id="sec-map"`, `id="sec-sankey"`, `id="sec-attack"`, `id="sec-list"`, "/static/sec.js",
		"Chuỗi cung ứng là mặt trận chính", "Ghim phiên bản dependency", "Cao", "🛡️ An ninh mạng"} {
		if !strings.Contains(body, w) {
			t.Errorf("thiếu %q", w)
		}
	}
	if strings.Contains(body, "npm </script>") {
		t.Error("dữ liệu nhúng phải chặn </script>")
	}
	for _, p := range []string{"/static/sec.js", "/static/world.json"} {
		if code, _ := get(t, srv.URL+p); code != 200 {
			t.Errorf("%s: HTTP %d", p, code)
		}
	}
	// bản công khai: mặc định KHÔNG có trang security (lộ stack của người dùng); bật bằng publish.include_security
	pub := NewPublic(s.cfg, s.st, PublicOptions{SiteTitle: "x"})
	rec := httptest.NewRecorder()
	pub.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/security", nil))
	if rec.Code != 404 {
		t.Fatalf("public /security mặc định phải 404, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	pub.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/dashboard", nil))
	if strings.Contains(rec.Body.String(), `href="/security`) {
		t.Fatal("nav công khai không được có link /security")
	}
	s.cfg.Publish.IncludeSecurity = true
	rec = httptest.NewRecorder()
	pub.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/security", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `href="/security/30/"`) {
		t.Fatalf("public /security khi bật include_security: %d", rec.Code)
	}
}
