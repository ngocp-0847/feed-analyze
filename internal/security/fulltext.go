package security

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/ngocp-0847/feed-analyze/internal/store"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 feed-analyze/1.0"

// skipTags: vùng không phải nội dung bài.
var skipTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Nav: true, atom.Header: true, atom.Footer: true,
	atom.Aside: true, atom.Form: true, atom.Button: true, atom.Svg: true, atom.Iframe: true, atom.Select: true, atom.Figure: true,
}

// blockTags: phần tử khối có chữ đáng lấy.
var blockTags = map[atom.Atom]bool{
	atom.P: true, atom.Li: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true,
	atom.Pre: true, atom.Blockquote: true, atom.Td: true, atom.Dd: true,
}

// ExtractText lấy nội dung chính của trang bài: ưu tiên <article>, sau đó <main>, cuối cùng <body>.
func ExtractText(r io.Reader, maxChars int) string {
	doc, err := html.Parse(r)
	if err != nil {
		return ""
	}
	root := find(doc, atom.Article)
	if root == nil || len(collect(root, maxChars)) < 400 {
		if m := find(doc, atom.Main); m != nil {
			root = m
		} else if b := find(doc, atom.Body); b != nil {
			root = b
		} else {
			root = doc
		}
	}
	return collect(root, maxChars)
}

func find(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, a); f != nil {
			return f
		}
	}
	return nil
}

func collect(root *html.Node, maxChars int) string {
	var b strings.Builder
	seen := map[string]bool{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if b.Len() >= maxChars*2 {
			return
		}
		if n.Type == html.ElementNode {
			if skipTags[n.DataAtom] || hasNoiseClass(n) {
				return
			}
			if blockTags[n.DataAtom] {
				t := strings.Join(strings.Fields(textOf(n)), " ")
				min := 40
				if n.DataAtom == atom.H2 || n.DataAtom == atom.H3 || n.DataAtom == atom.Pre {
					min = 8
				}
				if len(t) >= min && !seen[t] {
					seen[t] = true
					b.WriteString(t)
					b.WriteString("\n")
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	out := strings.TrimSpace(b.String())
	if r := []rune(out); len(r) > maxChars {
		out = string(r[:maxChars]) + "…"
	}
	return out
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteString(" ")
		}
		if n.Type == html.ElementNode && skipTags[n.DataAtom] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// hasNoiseClass: khối quảng cáo / bài liên quan / chia sẻ / bình luận.
func hasNoiseClass(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key != "class" && a.Key != "id" {
			continue
		}
		v := strings.ToLower(a.Val)
		for _, w := range []string{"related", "share", "comment", "newsletter", "subscribe", "advert", "promo", "sidebar", "cookie", "popup", "breadcrumb"} {
			if strings.Contains(v, w) {
				return true
			}
		}
	}
	return false
}

// FetchText tải 1 trang bài và trích nội dung.
func FetchText(ctx context.Context, client *http.Client, url string, maxChars int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en,vi;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "html") {
		return "", fmt.Errorf("không phải HTML (%s)", ct)
	}
	t := ExtractText(io.LimitReader(resp.Body, 4<<20), maxChars)
	if len([]rune(t)) < 300 {
		return "", fmt.Errorf("trang ít nội dung (%d ký tự)", len([]rune(t)))
	}
	return t, nil
}

// EnsureTexts tải nội dung bài gốc cho các bài chưa có (song song); trả về id → nội dung (rỗng nếu lỗi).
func EnsureTexts(ctx context.Context, st *store.Store, arts []store.ArticleView, maxChars int) map[int64]string {
	client := &http.Client{Timeout: 20 * time.Second}
	out := make(map[int64]string, len(arts))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, a := range arts {
		if t, status, ok := st.ArticleText(a.ID); ok {
			if status == "ok" {
				out[a.ID] = t
			}
			continue
		}
		if a.Link == "" {
			continue
		}
		wg.Add(1)
		go func(a store.ArticleView) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t, err := FetchText(ctx, client, a.Link, maxChars)
			status := "ok"
			if err != nil {
				status, t = "fail: "+err.Error(), ""
			}
			mu.Lock()
			defer mu.Unlock()
			_ = st.SaveArticleText(a.ID, status, t)
			if t != "" {
				out[a.ID] = t
			}
		}(a)
	}
	wg.Wait()
	return out
}
