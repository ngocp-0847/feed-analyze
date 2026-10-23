// Package web: giao diện đọc bản tin & dashboard — chạy cục bộ (fa serve / fa open) hoặc xuất site tĩnh công khai (fa publish).
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
	"path"
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

// PublicOptions bật chế độ xuất bản công khai: link dạng thư mục (/r/2026-10-10/), không có trang tìm kiếm,
// chi phí Claude, mục cá nhân "Với bạn" hay gợi ý lệnh CLI; chỉ nội dung từ Since trở đi.
type PublicOptions struct {
	SiteTitle   string
	Description string
	BaseURL     string   // vd https://blog.bomclaw.org (không có "/" cuối)
	Since       string   // YYYY-MM-DD; rỗng = mọi ngày
	Exclude     []string // slug báo cáo deep / storyline không xuất bản
}

type Server struct {
	cfg      *config.Config
	st       *store.Store
	md       goldmark.Markdown
	tpl      *template.Template
	Pub      *PublicOptions // nil = chế độ cục bộ
	Shutdown func()         // gọi khi nhận /api/shutdown; nil = tắt tính năng
}

func New(cfg *config.Config, st *store.Store) *Server {
	s := &Server{cfg: cfg, st: st,
		md: goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe()))}
	s.tpl = template.Must(template.New("page").Parse(pageTpl))
	return s
}

// NewPublic tạo server ở chế độ xuất bản công khai.
func NewPublic(cfg *config.Config, st *store.Store, p PublicOptions) *Server {
	s := New(cfg, st)
	p.BaseURL = strings.TrimRight(p.BaseURL, "/")
	s.Pub = &p
	return s
}

func (s *Server) public() bool { return s.Pub != nil }

// link: ở chế độ public, đường dẫn trang kết thúc bằng "/" (site tĩnh lưu dạng thư mục/index.html).
func (s *Server) link(p string) string {
	if !s.public() || strings.ContainsAny(p, "?#") || strings.HasSuffix(p, "/") || path.Ext(p) != "" {
		return p
	}
	return p + "/"
}

func (s *Server) storyURL(slug string) string { return s.link("/s/" + url.PathEscape(slug)) }

// hidden: slug bị loại khỏi bản công khai.
func (s *Server) hidden(slug string) bool {
	if !s.public() {
		return false
	}
	for _, x := range s.Pub.Exclude {
		if strings.EqualFold(strings.TrimSpace(x), slug) {
			return true
		}
	}
	return false
}

// inWindow: ngày thuộc phạm vi xuất bản.
func (s *Server) inWindow(date string) bool {
	return !s.public() || s.Pub.Since == "" || date >= s.Pub.Since
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServerFS(sub), 3600)))
	if !s.public() {
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s %s", HealthToken, s.cfg.Home)
		})
		mux.HandleFunc("POST /api/shutdown", s.shutdown)
		mux.HandleFunc("GET /articles", s.articles)
	}
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /dashboard", s.dashboard)
	mux.HandleFunc("GET /dashboard/{days}", s.dashboard)
	mux.HandleFunc("GET /r/{date}", s.daily)
	mux.HandleFunc("GET /stories", s.stories)
	mux.HandleFunc("GET /s/{slug}", s.story)
	mux.HandleFunc("GET /security", s.security)
	mux.HandleFunc("GET /security/{days}", s.security)
	mux.HandleFunc("GET /deep", s.deepList)
	mux.HandleFunc("GET /deep/{date}/{slug}", s.deepView)
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

type navLink struct{ Href, Label string }

type page struct {
	Title, Nav, SiteTitle, Description, Canonical, Latest string
	Public, Security                                      bool
	Body                                                  template.HTML
	Dates                                                 []navLink
	Meta                                                  template.JS
	Year                                                  int
}

type topicMeta struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, nav, title, body string) {
	dates, _ := s.st.ReportDates("daily", 30)
	var links []navLink
	for _, d := range dates {
		if s.inWindow(d) {
			links = append(links, navLink{s.link("/r/" + d), d})
		}
	}
	var tm []topicMeta
	for _, t := range s.cfg.Topics {
		tm = append(tm, topicMeta{t.ID, t.Name, t.Emoji})
	}
	mb, _ := json.Marshal(map[string]any{"topics": tm, "public": s.public()})
	p := page{Title: title, Nav: nav, Body: template.HTML(body), Dates: links, Meta: template.JS(mb),
		SiteTitle: "feed-analyze", Year: time.Now().In(s.cfg.Loc).Year(), Latest: "/", Security: s.secVisible()}
	if len(links) > 0 && s.public() {
		p.Latest = links[0].Href
	}
	if s.public() {
		p.Public, p.SiteTitle, p.Description = true, s.Pub.SiteTitle, s.Pub.Description
		if s.Pub.BaseURL != "" && r != nil {
			p.Canonical = s.Pub.BaseURL + s.link(r.URL.Path)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, p)
}

func (s *Server) markdown(md string) string {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(md), &buf); err != nil {
		return "<pre>" + template.HTMLEscapeString(md) + "</pre>"
	}
	return buf.String()
}

func (s *Server) latestDates(n int) []string {
	dates, _ := s.st.ReportDates("daily", 400)
	var out []string
	for _, d := range dates {
		if s.inWindow(d) {
			out = append(out, d)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if s.public() {
		s.home(w, r)
		return
	}
	dates := s.latestDates(1)
	if len(dates) == 0 {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/r/"+dates[0], http.StatusFound)
}

// home: trang chủ site công khai — bản tin mới nhất nổi bật + lưu trữ + phân tích sâu.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	dates := s.latestDates(400)
	var b strings.Builder
	fmt.Fprintf(&b, `<h1>%s</h1>`, esc(s.Pub.SiteTitle))
	if s.Pub.Description != "" {
		fmt.Fprintf(&b, `<p class="sub">%s</p>`, esc(s.Pub.Description))
	}
	if len(dates) == 0 {
		b.WriteString(`<div class="empty">Chưa có bản tin nào được xuất bản.</div>`)
		s.render(w, r, "home", s.Pub.SiteTitle, b.String())
		return
	}
	b.WriteString(`<div class="cards">`)
	for i, d := range dates {
		dl, _ := analyze.LoadDaily(s.st, d)
		if dl == nil {
			continue
		}
		title, n := "Bản tin", 0
		if dl.Synthesis != nil && dl.Synthesis.Title != "" {
			title = dl.Synthesis.Title
		}
		for _, t := range dl.Topics {
			n += len(t.Storylines)
		}
		cls := "card"
		if i == 0 {
			cls = "card lead"
		}
		fmt.Fprintf(&b, `<a class="%s" href="%s"><span class="d">%s</span><b>%s</b>`, cls, s.link("/r/"+d), esc(report.DateTitle(d, s.cfg.Loc)), esc(title))
		if i == 0 && dl.Synthesis != nil {
			b.WriteString(`<ul>`)
			for j, x := range dl.Synthesis.TLDR {
				if j >= 4 {
					break
				}
				fmt.Fprintf(&b, `<li>%s</li>`, esc(x))
			}
			b.WriteString(`</ul>`)
		}
		fmt.Fprintf(&b, `<span class="m">%d storyline · %d chủ đề</span></a>`, n, len(dl.Topics))
		if i >= 30 {
			break
		}
	}
	b.WriteString(`</div>`)
	reps := s.deepReports(8)
	if len(reps) > 0 {
		b.WriteString(`<h2>Phân tích sâu</h2><div class="cards">`)
		for _, rp := range reps {
			title := strings.TrimPrefix(rp.Kind, "deep:")
			if v, _ := research.LoadViz(s.st, rp.Date, rp.Kind); v != nil && v.Headline != "" {
				title = v.Headline
			}
			fmt.Fprintf(&b, `<a class="card" href="%s"><span class="d">%s</span><b>%s</b></a>`, s.link(DeepURL(rp.Date, strings.TrimPrefix(rp.Kind, "deep:"))), esc(rp.Date), esc(trunc(title, 160)))
		}
		b.WriteString(`</div>`)
	}
	s.render(w, r, "home", s.Pub.SiteTitle, b.String())
}

func (s *Server) daily(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	d, err := analyze.LoadDaily(s.st, date)
	if err != nil || d == nil || !s.inWindow(date) {
		http.NotFound(w, r)
		return
	}
	in, _ := report.Collect(s.st, d)
	md := report.Markdown(s.cfg, in, report.Options{StoryLink: s.storyURL, ChainHTML: chainHTML, Public: s.public()})
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="tabs"><a href="#dash" class="on">📊 Dashboard ngày</a><a href="#read">📰 Đọc bản tin</a><a href="%s">Xu hướng nhiều ngày →</a></div>`, s.link("/dashboard"))
	b.WriteString(`<div id="dash">`)
	b.WriteString(s.dailyDash(d, in))
	b.WriteString(`</div><div id="read" class="prose">`)
	b.WriteString(s.markdown(md))
	b.WriteString(`</div>`)
	title := "Bản tin " + date
	if d.Synthesis != nil && d.Synthesis.Title != "" {
		title = d.Synthesis.Title + " — " + date
	}
	s.render(w, r, "daily", title, b.String())
}

func (s *Server) stories(w http.ResponseWriter, r *http.Request) {
	days := atoi(r.URL.Query().Get("days"), 14)
	topic := r.URL.Query().Get("topic")
	since := time.Now().In(s.cfg.Loc).AddDate(0, 0, -days+1).Format("2006-01-02")
	var b strings.Builder
	if s.public() {
		since = s.Pub.Since
		b.WriteString("# Storyline\n\nCác câu chuyện được theo dõi qua nhiều ngày — mới cập nhật trước.\n\n")
	} else {
		fmt.Fprintf(&b, "# Storyline %d ngày qua\n\n", days)
		b.WriteString(s.filterBar("/stories", topic, days, ""))
	}
	sls, _ := s.st.Storylines(topic, since, 500)
	for _, t := range s.cfg.Topics {
		var rows []store.Storyline
		for _, sl := range sls {
			if sl.Topic == t.ID && !s.hidden(sl.Slug) {
				rows = append(rows, sl)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s %s\n\n| Storyline | Tầng | Trạng thái | Số ngày | Lần cuối |\n|---|---|---|---|---|\n", t.Emoji, t.Name)
		for _, sl := range rows {
			fmt.Fprintf(&b, "| [%s](%s)<br><small>%s</small> | %s | %s | %d | %s |\n", cell(sl.Title), s.storyURL(sl.Slug),
				cell(trunc(sl.Summary, 180)), analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.LastDate)
		}
	}
	s.render(w, r, "stories", "Storyline", `<div class="prose">`+s.markdown(b.String())+`</div>`)
}

func (s *Server) story(w http.ResponseWriter, r *http.Request) {
	sl, _ := s.st.Storyline(r.PathValue("slug"))
	if sl == nil || !s.inWindow(sl.LastDate) || s.hidden(sl.Slug) {
		http.NotFound(w, r)
		return
	}
	var b strings.Builder
	t, _ := s.cfg.Topic(sl.Topic)
	fmt.Fprintf(&b, "# %s\n\n`%s` · %s %s · %s · %s · %d ngày (%s → %s)\n\n%s\n\n**Vì sao quan trọng:** %s\n\n",
		sl.Title, sl.Slug, t.Emoji, t.Name, analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.FirstDate, sl.LastDate, sl.Summary, sl.Why)
	if !s.public() {
		fmt.Fprintf(&b, "> Đào sâu có chart: `fa deep %s --open`\n\n", sl.Slug)
	}
	b.WriteString("## Diễn biến\n\n")
	evs, _ := s.st.StorylineEvents(sl.Slug)
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		if !s.inWindow(e.Date) {
			continue
		}
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
	fmt.Fprintf(&b, "## Nguồn tin (%d)\n\n", len(vs))
	s.articleTable(&b, vs)
	s.render(w, r, "stories", sl.Title, `<div class="prose">`+s.markdown(b.String())+`</div>`)
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
	s.render(w, r, "articles", "Bài viết", s.markdown(b.String()))
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

// deepReports: báo cáo deep trong phạm vi xuất bản, mới nhất trước.
func (s *Server) deepReports(n int) []store.Report {
	reps, _ := s.st.ReportsByKindPrefix("deep:", 500)
	var out []store.Report
	for _, rp := range reps {
		if s.inWindow(rp.Date) && !s.hidden(strings.TrimPrefix(rp.Kind, "deep:")) {
			out = append(out, rp)
			if len(out) >= n {
				break
			}
		}
	}
	return out
}

func (s *Server) deepList(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("# Phân tích sâu\n\n")
	if !s.public() {
		b.WriteString("Tạo mới: `fa deep <slug | id | \"câu hỏi\"> --open`\n\n")
	}
	b.WriteString("| Báo cáo | Ngày | Dashboard |\n|---|---|---|\n")
	for _, rp := range s.deepReports(500) {
		slug := strings.TrimPrefix(rp.Kind, "deep:")
		title, has := slug, "—"
		if v, _ := research.LoadViz(s.st, rp.Date, rp.Kind); v != nil {
			has = "📊 có"
			if v.Headline != "" {
				title = trunc(v.Headline, 140)
			}
		}
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s |\n", cell(title), s.link(DeepURL(rp.Date, slug)), rp.Date, has)
	}
	s.render(w, r, "deep", "Phân tích sâu", `<div class="prose">`+s.markdown(b.String())+`</div>`)
}

func (s *Server) deepView(w http.ResponseWriter, r *http.Request) {
	date, slug := r.PathValue("date"), r.PathValue("slug")
	kind := "deep:" + slug
	rp, _ := s.st.GetReport(date, kind)
	if rp == nil || !s.inWindow(date) || s.hidden(slug) {
		http.NotFound(w, r)
		return
	}
	v, _ := research.LoadViz(s.st, date, kind)
	var b strings.Builder
	if v != nil || !s.public() {
		b.WriteString(`<div class="tabs"><a href="#dash" class="on">📊 Dashboard</a><a href="#read">📄 Báo cáo đầy đủ</a></div><div id="dash">`)
		b.WriteString(deepDash(v, slug))
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div id="read" class="prose">`)
	b.WriteString(s.markdown(rp.Markdown))
	b.WriteString(`</div>`)
	title := slug
	if v != nil && v.Headline != "" {
		title = trunc(v.Headline, 90)
	}
	s.render(w, r, "deep", title, b.String())
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
<title>{{.Title}} · {{.SiteTitle}}</title>
{{if .Description}}<meta name="description" content="{{.Description}}">{{end}}
{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}"><meta property="og:url" content="{{.Canonical}}">{{end}}
<meta property="og:title" content="{{.Title}}"><meta property="og:site_name" content="{{.SiteTitle}}"><meta property="og:type" content="article">
{{if .Public}}<link rel="alternate" type="application/atom+xml" title="{{.SiteTitle}}" href="/feed.xml">{{end}}
<link rel="stylesheet" href="/static/app.css">
<script>try{var q=new URLSearchParams(location.search).get("theme");if(q==="dark"||q==="light")localStorage.setItem("fa-theme",q);var t=localStorage.getItem("fa-theme");if(t)document.documentElement.setAttribute("data-theme",t)}catch(e){}</script>
</head><body>
<header class="top"><div class="bar"><a class="brand" href="/">📰 {{.SiteTitle}}</a>
{{if .Public}}<a href="/" class="nav {{if eq .Nav "home"}}on{{end}}">Trang chủ</a>{{end}}
<a href="{{.Latest}}" class="nav {{if eq .Nav "daily"}}on{{end}}">Bản tin</a>
<a href="/dashboard{{if .Public}}/{{end}}" class="nav {{if eq .Nav "dashboard"}}on{{end}}">Dashboard</a>
<a href="/stories{{if .Public}}/{{end}}" class="nav {{if eq .Nav "stories"}}on{{end}}">Storyline</a>
{{if not .Public}}<a href="/articles" class="nav {{if eq .Nav "articles"}}on{{end}}">Bài viết</a>{{end}}
<a href="/deep{{if .Public}}/{{end}}" class="nav {{if eq .Nav "deep"}}on{{end}}">Phân tích sâu</a>
{{if .Security}}<a href="/security{{if .Public}}/{{end}}" class="nav {{if eq .Nav "security"}}on{{end}}">🛡️ An ninh mạng</a>{{end}}
<span class="sp"></span>{{if .Public}}<a class="nav" href="/feed.xml" title="Theo dõi qua RSS/Atom">RSS</a>{{end}}<button id="theme-toggle" type="button" title="Đổi sáng/tối">◐</button></div></header>
<div class="wrap"><main>{{.Body}}</main>
<aside>{{if .Dates}}<div><small>BẢN TIN</small></div>{{range .Dates}}<a href="{{.Href}}">{{.Label}}</a>{{end}}{{end}}</aside></div>
{{if .Public}}<footer class="foot"><div>Bản tin & phân tích được tạo tự động bởi <a href="https://github.com/ngocp-0847/feed-analyze">feed-analyze</a> + Claude từ tiêu đề/tóm tắt RSS của các báo trong nước & quốc tế (dẫn link nguồn gốc). Nội dung do AI tổng hợp có thể sai sót — hãy kiểm chứng ở nguồn trước khi ra quyết định; không phải lời khuyên đầu tư.</div><div>© {{.Year}} {{.SiteTitle}}</div></footer>{{end}}
<script type="application/json" id="fa-meta">{{.Meta}}</script>
<script src="/static/echarts.min.js"></script>
<script src="/static/viz.js"></script>
<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>
document.querySelectorAll('code.language-mermaid').forEach(function(c){var d=document.createElement('div');d.className='mermaid';d.textContent=c.textContent;c.parentNode.replaceWith(d)});
if(window.mermaid){var dk=getComputedStyle(document.documentElement).getPropertyValue('color-scheme').trim()==='dark';mermaid.initialize({startOnLoad:false,theme:dk?'dark':'default'});mermaid.run()}
</script>
</body></html>`
