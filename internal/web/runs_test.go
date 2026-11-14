package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func TestRunsBoard(t *testing.T) {
	srv, s := setup(t)

	// chưa có run nào → trạng thái rỗng, không lỗi
	code, body := get(t, srv.URL+"/runs")
	if code != 200 || !strings.Contains(body, "Chưa có lần chạy nào") {
		t.Fatalf("board rỗng: %d", code)
	}

	id, err := s.st.StartRun("daily", "--skip-fetch <x>")
	if err != nil {
		t.Fatal(err)
	}
	s.st.LogLLMCall("classify", "haiku", 0.12, 3*time.Second, nil)
	s.st.LogLLMCall("analyze:economy", "sonnet", 0, time.Second, errors.New("<script>alert(1)</script>"))
	s.st.AddRunStep("fetch", time.Now(), "ok", "7 bài mới", nil)
	s.st.AddRunStep("analyze", time.Now(), store.RunError, "", errors.New("hết hạn"))
	s.st.FinishRun(id, store.RunOK, nil) // có bước lỗi → "Một phần"
	other, _ := s.st.StartRun("fetch", "")
	s.st.FinishRun(other, store.RunError, errors.New("mạng lỗi"))

	code, body = get(t, srv.URL+"/runs")
	if code != 200 {
		t.Fatalf("/runs: %d", code)
	}
	for _, w := range []string{"fa daily", "fa fetch", "Một phần", "Lỗi", "$0.120", `href="/runs/` + "1" + `"`, "Lịch sử chạy"} {
		if !strings.Contains(body, w) {
			t.Errorf("/runs thiếu %q", w)
		}
	}
	if strings.Contains(body, "<x>") {
		t.Error("args chưa được escape")
	}

	// lọc theo lệnh
	_, body = get(t, srv.URL+"/runs?kind=fetch")
	if !strings.Contains(body, "fa fetch") || strings.Contains(body, "--skip-fetch") {
		t.Error("lọc kind=fetch sai")
	}

	// chi tiết
	code, body = get(t, srv.URL+"/runs/1")
	if code != 200 {
		t.Fatalf("/runs/1: %d", code)
	}
	for _, w := range []string{"Run #1", "7 bài mới", "hết hạn", "analyze:economy", "haiku", "Các bước", "Lần gọi Claude (2)"} {
		if !strings.Contains(body, w) {
			t.Errorf("/runs/1 thiếu %q", w)
		}
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("lỗi Claude chưa được escape (XSS)")
	}

	for _, p := range []string{"/runs/999", "/runs/abc"} {
		if code, _ := get(t, srv.URL+p); code != 404 {
			t.Errorf("%s: want 404, got %d", p, code)
		}
	}
}

// Bản công khai không được lộ lịch sử run (chứa chi phí và lỗi nội bộ).
func TestRunsNotPublic(t *testing.T) {
	_, s := setup(t)
	pub := NewPublic(s.cfg, s.st, PublicOptions{SiteTitle: "x"})
	for _, p := range []string{"/runs", "/runs/1"} {
		rec := httptest.NewRecorder()
		pub.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 404 {
			t.Errorf("public %s: want 404, got %d", p, rec.Code)
		}
	}
}
