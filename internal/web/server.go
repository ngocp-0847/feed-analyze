// Package web: giao diện đọc bản tin cục bộ (fa serve).
package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
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
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

type Server struct {
	cfg *config.Config
	st  *store.Store
	md  goldmark.Markdown
	tpl *template.Template
}

func New(cfg *config.Config, st *store.Store) *Server {
	s := &Server{cfg: cfg, st: st,
		md: goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe()))}
	s.tpl = template.Must(template.New("page").Parse(pageTpl))
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /r/{date}", s.daily)
	mux.HandleFunc("GET /stories", s.stories)
	mux.HandleFunc("GET /s/{slug}", s.story)
	mux.HandleFunc("GET /articles", s.articles)
	mux.HandleFunc("GET /deep", s.deepList)
	mux.HandleFunc("GET /deep/{date}/{kind}", s.deepView)
	return mux
}

type page struct {
	Title string
	Nav   string
	Body  template.HTML
	Dates []string
}

func (s *Server) render(w http.ResponseWriter, nav, title string, body []byte) {
	dates, _ := s.st.ReportDates("daily", 30)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, page{Title: title, Nav: nav, Body: template.HTML(body), Dates: dates})
}

func (s *Server) markdown(md string) []byte {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(md), &buf); err != nil {
		return []byte("<pre>" + template.HTMLEscapeString(md) + "</pre>")
	}
	return buf.Bytes()
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	dates, _ := s.st.ReportDates("daily", 1)
	if len(dates) == 0 {
		s.render(w, "daily", "feed-analyze", s.markdown("# Chưa có bản tin\n\nChạy `fa daily` trong terminal rồi tải lại trang."))
		return
	}
	http.Redirect(w, r, "/r/"+dates[0], http.StatusFound)
}

func (s *Server) daily(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	d, err := analyze.LoadDaily(s.st, date)
	if err != nil || d == nil {
		http.NotFound(w, r)
		return
	}
	in, _ := report.Collect(s.st, d)
	md := report.Markdown(s.cfg, in, report.Options{StoryLink: func(slug string) string { return "/s/" + url.PathEscape(slug) }})
	s.render(w, "daily", "Bản tin "+date, s.markdown(md))
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
	s.render(w, "stories", "Storyline", s.markdown(b.String()))
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
	fmt.Fprintf(&b, "> Đào sâu: `fa deep %s`\n\n## Diễn biến\n\n", sl.Slug)
	evs, _ := s.st.StorylineEvents(sl.Slug)
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		var d analyze.StorylineOut
		_ = json.Unmarshal([]byte(e.Detail), &d)
		fmt.Fprintf(&b, "### %s — %s\n\n%s\n\n", e.Date, e.Status, e.Summary)
		if d.WhatChanged != "" {
			fmt.Fprintf(&b, "- **Mới:** %s\n", d.WhatChanged)
		}
		if len(d.CausalChain) > 0 {
			fmt.Fprintf(&b, "- **Chuỗi tác động:** %s\n", analyze.ChainString(d.CausalChain))
		}
		if d.VNAngle != "" {
			fmt.Fprintf(&b, "- **Việt Nam:** %s\n", d.VNAngle)
		}
		if len(d.WatchNext) > 0 {
			fmt.Fprintf(&b, "- **Theo dõi:** %s\n", strings.Join(d.WatchNext, "; "))
		}
		b.WriteString("\n")
	}
	ids, _ := s.st.StorylineArticleIDs(sl.Slug)
	vs, _ := s.st.ArticlesByIDs(ids)
	fmt.Fprintf(&b, "## Bài viết (%d)\n\n", len(vs))
	s.articleTable(&b, vs)
	s.render(w, "stories", sl.Title, s.markdown(b.String()))
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
	b.WriteString("# Phân tích sâu\n\nTạo mới: `fa deep <slug | id | \"câu hỏi\">`\n\n")
	for _, rp := range reps {
		fmt.Fprintf(&b, "- [%s](/deep/%s/%s) — %s\n", strings.TrimPrefix(rp.Kind, "deep:"), rp.Date, url.PathEscape(rp.Kind), rp.Date)
	}
	s.render(w, "deep", "Phân tích sâu", s.markdown(b.String()))
}

func (s *Server) deepView(w http.ResponseWriter, r *http.Request) {
	rp, _ := s.st.GetReport(r.PathValue("date"), r.PathValue("kind"))
	if rp == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "deep", strings.TrimPrefix(rp.Kind, "deep:"), s.markdown(rp.Markdown))
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
<style>
:root{--bg:#fbfaf7;--fg:#1d1d1f;--mut:#6b6b70;--line:#e6e3dc;--acc:#b4441c;--card:#fff;--code:#f2efe8}
@media (prefers-color-scheme:dark){:root{--bg:#141416;--fg:#e8e6e1;--mut:#9a9aa0;--line:#2c2c30;--acc:#f08a5d;--card:#1c1c1f;--code:#26262a}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.65 -apple-system,"Segoe UI",Roboto,"Noto Sans",sans-serif}
header{position:sticky;top:0;background:var(--bg);border-bottom:1px solid var(--line);z-index:2}
.bar{max-width:1080px;margin:0 auto;padding:10px 16px;display:flex;gap:18px;align-items:center;flex-wrap:wrap}
.bar b{color:var(--acc)}.bar a{color:var(--mut);text-decoration:none}.bar a.on{color:var(--fg);font-weight:600}
.wrap{max-width:1080px;margin:0 auto;padding:16px;display:grid;grid-template-columns:1fr 150px;gap:28px}
@media (max-width:820px){.wrap{grid-template-columns:1fr}aside{display:none}}
main{min-width:0}aside{font-size:14px;position:sticky;top:60px;align-self:start}aside a{display:block;color:var(--mut);text-decoration:none;padding:2px 0}
h1{font-size:1.75rem;line-height:1.25}h2{margin-top:2.2rem;padding-bottom:.3rem;border-bottom:1px solid var(--line)}h3{margin-top:1.6rem}
a{color:var(--acc)}code{background:var(--code);padding:1px 5px;border-radius:4px;font-size:.88em}
pre{background:var(--code);padding:12px;border-radius:8px;overflow-x:auto}pre code{padding:0}
blockquote{margin:0;padding:6px 14px;border-left:3px solid var(--acc);color:var(--mut);background:var(--card)}
table{border-collapse:collapse;width:100%;display:block;overflow-x:auto;font-size:14.5px}th,td{border:1px solid var(--line);padding:6px 9px;vertical-align:top;text-align:left}
th{background:var(--card)}small{color:var(--mut)}
.filters{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin:8px 0 12px}.filters input,.filters select,.filters button{font:inherit;padding:5px 8px;border:1px solid var(--line);border-radius:6px;background:var(--card);color:var(--fg)}
.filters input[name=q]{flex:1;min-width:200px}.filters input[type=number]{width:70px}
</style></head><body>
<header><div class="bar"><b>📰 feed-analyze</b>
<a href="/" class="{{if eq .Nav "daily"}}on{{end}}">Bản tin</a>
<a href="/stories" class="{{if eq .Nav "stories"}}on{{end}}">Storyline</a>
<a href="/articles" class="{{if eq .Nav "articles"}}on{{end}}">Bài viết</a>
<a href="/deep" class="{{if eq .Nav "deep"}}on{{end}}">Phân tích sâu</a></div></header>
<div class="wrap"><main>{{.Body}}</main>
<aside>{{if .Dates}}<div><small>BẢN TIN</small></div>{{range .Dates}}<a href="/r/{{.}}">{{.}}</a>{{end}}{{end}}</aside></div>
<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>
document.querySelectorAll('code.language-mermaid').forEach(function(c){var d=document.createElement('div');d.className='mermaid';d.textContent=c.textContent;c.parentNode.replaceWith(d)});
if(window.mermaid){mermaid.initialize({startOnLoad:false,theme:matchMedia('(prefers-color-scheme: dark)').matches?'dark':'default'});mermaid.run()}
</script>
</body></html>`
