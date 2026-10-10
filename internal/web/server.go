// Package web: giao diện đọc bản tin & dashboard cục bộ (fa serve / fa open).
package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/report"
	"github.com/ngocp-0847/feed-analyze/internal/research"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

//go:embed static
var staticFS embed.FS

// HealthToken nhận diện server của fa (để `fa open` biết server đang chạy là của mình).
const HealthToken = "feed-analyze"

type Server struct {
	cfg      *config.Config
	st       *store.Store
	md       goldmark.Markdown
	tpl      *template.Template
	Shutdown func() // gọi khi nhận /api/shutdown; nil = tắt tính năng
}

func New(cfg *config.Config, st *store.Store) *Server {
	s := &Server{cfg: cfg, st: st,
		md: goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe()))}
	s.tpl = template.Must(template.New("page").Parse(pageTpl))
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServerFS(sub), 3600)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s", HealthToken, s.cfg.Home)
	})
	mux.HandleFunc("POST /api/shutdown", s.shutdown)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /dashboard", s.dashboard)
	mux.HandleFunc("GET /r/{date}", s.daily)
	mux.HandleFunc("GET /stories", s.stories)
	mux.HandleFunc("GET /s/{slug}", s.story)
	mux.HandleFunc("GET /articles", s.articles)
	mux.HandleFunc("GET /deep", s.deepList)
	mux.HandleFunc("GET /deep/{date}/{kind}", s.deepView)
	return mux
}

func cacheFor(h http.Handler, sec int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(sec))
		h.ServeHTTP(w, r)
	})
}

func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() || s.Shutdown == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fmt.Fprintln(w, "bye")
	go func() { time.Sleep(100 * time.Millisecond); s.Shutdown() }()
}

type page struct {
	Title string
	Nav   string
	Body  template.HTML
	Dates []string
	Meta  template.JS
}

type topicMeta struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

func (s *Server) render(w http.ResponseWriter, nav, title string, body string) {
	dates, _ := s.st.ReportDates("daily", 30)
	var tm []topicMeta
	for _, t := range s.cfg.Topics {
		tm = append(tm, topicMeta{t.ID, t.Name, t.Emoji})
	}
	mb, _ := json.Marshal(map[string]any{"topics": tm})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, page{Title: title, Nav: nav, Body: template.HTML(body), Dates: dates, Meta: template.JS(mb)})
}

func (s *Server) markdown(md string) string {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(md), &buf); err != nil {
		return "<pre>" + template.HTMLEscapeString(md) + "</pre>"
	}
	return buf.String()
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	dates, _ := s.st.ReportDates("daily", 1)
	if len(dates) == 0 {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/r/"+dates[0], http.StatusFound)
}

func storyURL(slug string) string { return "/s/" + url.PathEscape(slug) }

func (s *Server) daily(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	d, err := analyze.LoadDaily(s.st, date)
	if err != nil || d == nil {
		http.NotFound(w, r)
		return
	}
	in, _ := report.Collect(s.st, d)
	md := report.Markdown(s.cfg, in, report.Options{StoryLink: storyURL, ChainHTML: chainHTML})
	var b strings.Builder
	b.WriteString(`<div class="tabs"><a href="#dash" class="on">📊 Dashboard ngày</a><a href="#read">📰 Đọc bản tin</a><a href="/dashboard">Xu hướng nhiều ngày →</a></div>`)
	b.WriteString(`<div id="dash">`)
	b.WriteString(s.dailyDash(d, in))
	b.WriteString(`</div><div id="read" class="prose">`)
	b.WriteString(s.markdown(md))
	b.WriteString(`</div>`)
	s.render(w, "daily", "Bản tin "+date, b.String())
}

func (s *Server) stories(w http.ResponseWriter, r *http.Request) {
	days := atoi(r.URL.Query().Get("days"), 14)
	topic := r.URL.Query().Get("topic")
	since := time.Now().In(s.cfg.Loc).AddDate(0, 0, -days+1).Format("2006-01-02")
	sls, _ := s.st.Storylines(topic, since, 300)
	var b strings.Builder
	fmt.Fprintf(&b, "# Storyline %d ngày qua\n\n", days)
	b.WriteString(s.filterBar("/stories", topic, days, ""))
	for _, t := range s.cfg.Topics {
		var rows []store.Storyline
		for _, sl := range sls {
			if sl.Topic == t.ID {
				rows = append(rows, sl)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s %s\n\n| Storyline | Tầng | Trạng thái | Số ngày | Lần cuối |\n|---|---|---|---|---|\n", t.Emoji, t.Name)
		for _, sl := range rows {
			fmt.Fprintf(&b, "| [%s](/s/%s)<br><small>%s</small> | %s | %s | %d | %s |\n", cell(sl.Title), url.PathEscape(sl.Slug),
				cell(trunc(sl.Summary, 180)), analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.LastDate)
		}
	}
	s.render(w, "stories", "Storyline", `<div class="prose">`+s.markdown(b.String())+`</div>`)
}

func (s *Server) story(w http.ResponseWriter, r *http.Request) {
	sl, _ := s.st.Storyline(r.PathValue("slug"))
	if sl == nil {
		http.NotFound(w, r)
		return
	}
	var b strings.Builder
	t, _ := s.cfg.Topic(sl.Topic)
	fmt.Fprintf(&b, "# %s\n\n`%s` · %s %s · %s · %s · %d ngày (%s → %s)\n\n%s\n\n**Vì sao quan trọng:** %s\n\n",
		sl.Title, sl.Slug, t.Emoji, t.Name, analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.FirstDate, sl.LastDate, sl.Summary, sl.Why)
	fmt.Fprintf(&b, "> Đào sâu có chart: `fa deep %s --open`\n\n## Diễn biến\n\n", sl.Slug)
	evs, _ := s.st.StorylineEvents(sl.Slug)
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		var d analyze.StorylineOut
		_ = json.Unmarshal([]byte(e.Detail), &d)
		fmt.Fprintf(&b, "### %s — %s\n\n%s\n\n", e.Date, e.Status, e.Summary)
		if d.WhatChanged != "" {
			fmt.Fprintf(&b, "- **Mới:** %s\n", d.WhatChanged)
		}
		if d.VNAngle != "" {
			fmt.Fprintf(&b, "- **Việt Nam:** %s\n", d.VNAngle)
		}
		if len(d.WatchNext) > 0 {
			fmt.Fprintf(&b, "- **Theo dõi:** %s\n", strings.Join(d.WatchNext, "; "))
		}
		if len(d.CausalChain) > 0 {
			b.WriteString("\n" + chainHTML(d.CausalChain) + "\n")
		}
		b.WriteString("\n")
	}
	ids, _ := s.st.StorylineArticleIDs(sl.Slug)
	vs, _ := s.st.ArticlesByIDs(ids)
	fmt.Fprintf(&b, "## Bài viết (%d)\n\n", len(vs))
	s.articleTable(&b, vs)
	s.render(w, "stories", sl.Title, `<div class="prose">`+s.markdown(b.String())+`</div>`)
}

func (s *Server) articles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days := atoi(q.Get("days"), 2)
	topic := q.Get("topic")
	query := q.Get("q")
	vs, err := s.st.Search(store.SearchOpts{Query: query, Topic: topic, Since: time.Now().AddDate(0, 0, -days), Limit: 200, OnlyOn: topic == "" && query == ""})
	var b strings.Builder
	b.WriteString("# Bài viết\n\n")
	b.WriteString(s.filterBar("/articles", topic, days, query))
	if err != nil {
		fmt.Fprintf(&b, "\n> Lỗi tìm kiếm: %s\n", err)
	}
	fmt.Fprintf(&b, "\n%d bài\n\n", len(vs))
	s.articleTable(&b, vs)
	s.render(w, "articles", "Bài viết", s.markdown(b.String()))
}

func (s *Server) articleTable(b *strings.Builder, vs []store.ArticleView) {
	b.WriteString("| # | Bài | Nguồn | Chủ đề | Mức |\n|---|---|---|---|---|\n")
	for _, a := range vs {
		sum, topics, imp := "", "", ""
		if a.Classification != nil {
			sum = a.SummaryVI
			var ts []string
			for _, t := range a.Topics {
				if tp, ok := s.cfg.Topic(t.Topic); ok {
					ts = append(ts, fmt.Sprintf("%s %.1f", tp.Emoji, t.Relevance))
				}
			}
			topics = strings.Join(ts, " ")
			imp = fmt.Sprintf("%d · %s", a.Importance, analyze.LevelVI(a.Scale))
		}
		fmt.Fprintf(b, "| %d | [%s](%s)<br><small>%s</small> | %s<br><small>%s</small> | %s | %s |\n", a.ID, cell(a.Title), a.Link, cell(sum),
			a.Source, a.Published.In(s.cfg.Loc).Format("02/01 15:04"), topics, imp)
	}
}

func (s *Server) deepList(w http.ResponseWriter, r *http.Request) {
	reps, _ := s.st.ReportsByKindPrefix("deep:", 100)
	var b strings.Builder
	b.WriteString("# Phân tích sâu\n\nTạo mới: `fa deep <slug | id | \"câu hỏi\"> --open`\n\n| Báo cáo | Ngày | Dashboard |\n|---|---|---|\n")
	for _, rp := range reps {
		has := "—"
		if v, _ := research.LoadViz(s.st, rp.Date, rp.Kind); v != nil {
			has = "📊 có"
		}
		fmt.Fprintf(&b, "| [%s](/deep/%s/%s) | %s | %s |\n", strings.TrimPrefix(rp.Kind, "deep:"), rp.Date, url.PathEscape(rp.Kind), rp.Date, has)
	}
	s.render(w, "deep", "Phân tích sâu", `<div class="prose">`+s.markdown(b.String())+`</div>`)
}

func (s *Server) deepView(w http.ResponseWriter, r *http.Request) {
	date, kind := r.PathValue("date"), r.PathValue("kind")
	rp, _ := s.st.GetReport(date, kind)
	if rp == nil {
		http.NotFound(w, r)
		return
	}
	v, _ := research.LoadViz(s.st, date, kind)
	var b strings.Builder
	b.WriteString(`<div class="tabs"><a href="#dash" class="on">📊 Dashboard</a><a href="#read">📄 Báo cáo đầy đủ</a></div><div id="dash">`)
	b.WriteString(deepDash(v, strings.TrimPrefix(kind, "deep:")))
	b.WriteString(`</div><div id="read" class="prose">`)
	b.WriteString(s.markdown(rp.Markdown))
	b.WriteString(`</div>`)
	s.render(w, "deep", strings.TrimPrefix(kind, "deep:"), b.String())
}

func (s *Server) filterBar(path, topic string, days int, q string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<form class="filters" method="get" action="%s">`, path)
	if path == "/articles" {
		fmt.Fprintf(&b, `<input name="q" value="%s" placeholder="Tìm (không dấu cũng được)…">`, template.HTMLEscapeString(q))
	}
	b.WriteString(`<select name="topic"><option value="">Mọi chủ đề</option>`)
	for _, t := range s.cfg.Topics {
		sel := ""
		if t.ID == topic {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s %s</option>`, t.ID, sel, t.Emoji, template.HTMLEscapeString(t.Name))
	}
	fmt.Fprintf(&b, `</select><input name="days" type="number" min="1" max="365" value="%d"> ngày <button>Lọc</button></form>`+"\n", days)
	return b.String()
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "|", "/"), "\n", " ")
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

const pageTpl = `<!doctype html>
<html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · feed-analyze</title>
<link rel="stylesheet" href="/static/app.css">
<script>try{var q=new URLSearchParams(location.search).get("theme");if(q==="dark"||q==="light")localStorage.setItem("fa-theme",q);var t=localStorage.getItem("fa-theme");if(t)document.documentElement.setAttribute("data-theme",t)}catch(e){}</script>
</head><body>
<header class="top"><div class="bar"><a class="brand" href="/">📰 feed-analyze</a>
<a href="/" class="nav {{if eq .Nav "daily"}}on{{end}}">Bản tin</a>
<a href="/dashboard" class="nav {{if eq .Nav "dashboard"}}on{{end}}">Dashboard</a>
<a href="/stories" class="nav {{if eq .Nav "stories"}}on{{end}}">Storyline</a>
<a href="/articles" class="nav {{if eq .Nav "articles"}}on{{end}}">Bài viết</a>
<a href="/deep" class="nav {{if eq .Nav "deep"}}on{{end}}">Phân tích sâu</a>
<span class="sp"></span><button id="theme-toggle" type="button" title="Đổi sáng/tối">◐</button></div></header>
<div class="wrap"><main>{{.Body}}</main>
<aside>{{if .Dates}}<div><small>BẢN TIN</small></div>{{range .Dates}}<a href="/r/{{.}}">{{.}}</a>{{end}}{{end}}</aside></div>
<script type="application/json" id="fa-meta">{{.Meta}}</script>
<script src="/static/echarts.min.js"></script>
<script src="/static/viz.js"></script>
<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>
document.querySelectorAll('code.language-mermaid').forEach(function(c){var d=document.createElement('div');d.className='mermaid';d.textContent=c.textContent;c.parentNode.replaceWith(d)});
if(window.mermaid){var dk=getComputedStyle(document.documentElement).getPropertyValue('color-scheme').trim()==='dark';mermaid.initialize({startOnLoad:false,theme:dk?'dark':'default'});mermaid.run()}
</script>
</body></html>`
