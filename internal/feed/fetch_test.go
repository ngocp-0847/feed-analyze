package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func TestCleanAndHash(t *testing.T) {
	if got := clean("<p>Lãi&nbsp;suất <b>tăng</b></p>\n\n 2%", 100); got != "Lãi suất tăng 2%" && got != "Lãi suất tăng 2%" {
		t.Fatalf("clean=%q", got)
	}
	if got := clean("abcdef", 3); got != "abc…" {
		t.Fatalf("truncate=%q", got)
	}
	a := hashKey("https://www.x.vn/a?utm_source=fb#top", "T", "f")
	b := hashKey("http://x.vn/a", "khác", "g")
	if a != b {
		t.Fatal("link chuẩn hoá phải cho cùng hash")
	}
	if hashKey("", "T", "f") == hashKey("", "T", "g") {
		t.Fatal("không link: hash phải phụ thuộc feed")
	}
}

func TestFetchAll(t *testing.T) {
	now := time.Now().UTC()
	rss := fmt.Sprintf(`<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
<item><title>Tin mới</title><link>https://x.vn/1</link><description>&lt;p&gt;mô tả&lt;/p&gt;</description><pubDate>%s</pubDate></item>
<item><title>Tin cũ</title><link>https://x.vn/2</link><pubDate>%s</pubDate></item>
<item><title>Tin mới</title><link>https://x.vn/1?utm_source=a</link><pubDate>%s</pubDate></item>
</channel></rss>`, now.Format(time.RFC1123Z), now.Add(-100*time.Hour).Format(time.RFC1123Z), now.Format(time.RFC1123Z))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			http.Error(w, "x", 500)
			return
		}
		fmt.Fprint(w, rss)
	}))
	defer srv.Close()

	home := t.TempDir()
	cfg, err := config.Parse([]byte(fmt.Sprintf(`topics: [{id: economy}]
feeds:
  - {name: ok, url: "%s/rss", region: vn}
  - {name: bad, url: "%s/bad"}`, srv.URL, srv.URL)), home)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res := FetchAll(context.Background(), cfg, st, cfg.ActiveFeeds(), nil)
	if res[0].Err != nil || res[0].New != 1 || res[0].Items != 3 {
		t.Fatalf("ok feed: %+v", res[0])
	}
	if res[1].Err == nil {
		t.Fatal("bad feed phải lỗi")
	}
	// chạy lại: không có bài mới
	res = FetchAll(context.Background(), cfg, st, cfg.ActiveFeeds()[:1], nil)
	if res[0].New != 0 {
		t.Fatalf("lần 2 New=%d", res[0].New)
	}
	arts, _ := st.Unclassified(10)
	if len(arts) != 1 || arts[0].Summary != "mô tả" || arts[0].Lang != "vi" || arts[0].Source != "x.vn" {
		t.Fatalf("article: %+v", arts)
	}
}
