package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPublic(t *testing.T) {
	_, local := setup(t)
	pub := NewPublic(local.cfg, local.st, PublicOptions{SiteTitle: "Blog", BaseURL: "https://blog.example.org/", Exclude: []string{"vn-rate"}})
	dir := filepath.Join(t.TempDir(), "site")
	st, err := pub.Export(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Dailies != 1 || st.Deeps != 0 || st.Stories != 0 {
		t.Fatalf("stats=%+v (vn-rate phải bị loại khỏi deep & storyline)", st)
	}
	date := local.cfg.Today()
	for _, f := range []string{"index.html", "r/" + date + "/index.html", "dashboard/index.html", "dashboard/30/index.html",
		"stories/index.html", "deep/index.html", "404.html", "feed.xml", "sitemap.xml", "robots.txt", "_headers", "static/viz.js", "static/echarts.min.js"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("thiếu %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "s", "vn-rate")); err == nil {
		t.Error("storyline bị exclude vẫn được xuất")
	}
	daily, _ := os.ReadFile(filepath.Join(dir, "r", date, "index.html"))
	for _, bad := range []string{"Với bạn", "fa deep", "Chi phí Claude", `href="/articles`, "/healthz"} {
		if strings.Contains(string(daily), bad) {
			t.Errorf("trang công khai lộ %q", bad)
		}
	}
	if !strings.Contains(string(daily), `href="/dashboard/"`) || !strings.Contains(string(daily), `rel="canonical" href="https://blog.example.org/r/`+date+`/"`) {
		t.Error("link/canonical không theo dạng thư mục")
	}
	sm, _ := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	if !strings.Contains(string(sm), "https://blog.example.org/r/"+date+"/") {
		t.Error("sitemap thiếu bản tin")
	}

	// chạy lại: dọn sạch file cũ nhưng không đụng thư mục lạ
	if _, err := pub.Export(dir); err != nil {
		t.Fatalf("export lần 2: %v", err)
	}
	foreign := filepath.Join(t.TempDir(), "mine")
	_ = os.MkdirAll(foreign, 0o755)
	_ = os.WriteFile(filepath.Join(foreign, "keep.txt"), []byte("x"), 0o644)
	if _, err := pub.Export(foreign); err == nil {
		t.Fatal("không được ghi đè thư mục không do fa tạo")
	}
	if _, err := os.Stat(filepath.Join(foreign, "keep.txt")); err != nil {
		t.Fatal("file của người dùng bị xoá")
	}
}
