package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/report"
	"github.com/ngocp-0847/feed-analyze/internal/research"
)

var seq atomic.Int64

func esc(s string) string { return template.HTMLEscapeString(s) }

// dataScript nhúng JSON cho renderer JS, trả về id.
func dataScript(b *strings.Builder, v any) string {
	id := fmt.Sprintf("d%d", seq.Add(1))
	j, _ := json.Marshal(v)
	// chặn "</script>" trong dữ liệu
	js := strings.ReplaceAll(string(j), "</", `<\/`)
	fmt.Fprintf(b, `<script type="application/json" id="%s">%s</script>`, id, js)
	return id
}

type panelOpt struct {
	Width     string // "" | half | third
	Title     string
	Sub       string
	Viz       string
	Data      any
	Height    int
	Legend    bool // legend HTML do renderer sinh
	HTML      string
	NoTable   bool
	EmptyText string
}

func panel(b *strings.Builder, o panelOpt) {
	fmt.Fprintf(b, `<section class="panel %s"><h3>%s</h3>`, o.Width, esc(o.Title))
	if o.Sub != "" {
		fmt.Fprintf(b, `<p class="sub">%s</p>`, esc(o.Sub))
	}
	if o.HTML != "" {
		b.WriteString(o.HTML)
	}
	if o.Viz != "" {
		if o.Legend {
			b.WriteString(`<div class="legend" data-auto></div>`)
		}
		id := dataScript(b, o.Data)
		h := o.Height
		if h == 0 {
			h = 300
		}
		fmt.Fprintf(b, `<div class="chart" data-viz="%s" data-src="%s" style="height:%dpx" role="img" aria-label="%s"></div>`, o.Viz, id, h, esc(o.Title))
		if !o.NoTable {
			b.WriteString(`<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body"></div></details>`)
		}
	}
	b.WriteString(`</section>`)
}

func kpi(b *strings.Builder, label, value, unit, delta, note string) {
	fmt.Fprintf(b, `<div class="kpi"><div class="lab">%s</div><div class="val">%s`, esc(label), esc(value))
	if unit != "" {
		fmt.Fprintf(b, `<span class="unit">%s</span>`, esc(unit))
	}
	b.WriteString(`</div>`)
	if delta != "" {
		fmt.Fprintf(b, `<div class="delta">%s</div>`, esc(delta))
	}
	if note != "" {
		fmt.Fprintf(b, `<div class="note">%s</div>`, esc(note))
	}
	b.WriteString(`</div>`)
}

// chainHTML: sơ đồ chuỗi tác động (ô theo tầng, mũi tên ghi kênh truyền dẫn).
func chainHTML(c []analyze.ChainStep) string {
	var b strings.Builder
	b.WriteString(`<div class="chain">`)
	for i, s := range c {
		if i > 0 {
			ch := s.Channel
			if ch == "" {
				ch = " "
			}
			fmt.Fprintf(&b, `<div class="arrow"><span>→</span>%s</div>`, esc(ch))
		}
		fmt.Fprintf(&b, `<div class="node %s"><b>%s</b>%s</div>`, esc(s.Level), esc(analyze.LevelVI(s.Level)), esc(s.Step))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// status hiển thị kèm icon + nhãn (không bao giờ chỉ dựa vào màu).
func statusHTML(kind, label string) string {
	icon := map[string]string{"good": "▲", "warning": "◆", "serious": "▼", "critical": "■", "neutral": "●"}[kind]
	return fmt.Sprintf(`<span class="st %s"><i>%s</i> %s</span>`, kind, icon, esc(label))
}

// ---------- dashboard ngày ----------

type storyPoint struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Topic      string `json:"topic"`
	Level      string `json:"level"`
	Importance int    `json:"importance"`
	Status     string `json:"status"`
	Articles   int    `json:"articles"`
	Summary    string `json:"summary"`
}

type netNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Topic string `json:"topic"`
	Slug  string `json:"slug,omitempty"`
}

type netLink struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Mechanism   string `json:"mechanism"`
	Implication string `json:"implication"`
}

func (s *Server) dailyDash(d *analyze.Daily, in report.Input) string {
	var b strings.Builder
	var points []storyPoint
	titles := map[string]storyPoint{}
	total, esc8, nw := 0, 0, 0
	for _, t := range d.Topics {
		total += t.ArticleCount
		for _, sl := range t.Storylines {
			p := storyPoint{sl.Slug, sl.Title, t.Topic, sl.Level, sl.Importance, sl.Status, len(sl.ArticleIDs), sl.Summary}
			points = append(points, p)
			titles[sl.Slug] = p
			if sl.Status == "escalating" {
				esc8++
			}
			if sl.Status == "new" {
				nw++
			}
		}
	}
	b.WriteString(`<div class="dash">`)
	if syn := d.Synthesis; syn != nil && syn.Title != "" {
		fmt.Fprintf(&b, `<div class="hero"><b>%s</b> — %s</div>`, esc(report.DateTitle(d.Date, s.cfg.Loc)), esc(syn.Title))
	}
	b.WriteString(`<div class="kpis">`)
	kpi(&b, "Bài liên quan được phân tích", fmt.Sprint(total), "bài", "", "")
	kpi(&b, "Storyline", fmt.Sprint(len(points)), "", fmt.Sprintf("%d mới · %d leo thang", nw, esc8), "")
	if d.Synthesis != nil {
		kpi(&b, "Liên kết chéo chủ đề", fmt.Sprint(len(d.Synthesis.CrossLinks)), "", "", "AI ↔ kinh tế ↔ chính trị")
		kpi(&b, "Tác động tới VN", fmt.Sprint(len(d.Synthesis.VNImpact)), "lĩnh vực", "", "")
	}
	for _, t := range d.Topics {
		tp, _ := s.cfg.Topic(t.Topic)
		kpi(&b, tp.Emoji+" "+tp.Name, fmt.Sprint(t.ArticleCount), "bài", fmt.Sprintf("%d storyline", len(t.Storylines)), "")
	}
	b.WriteString(`</div>`)

	panel(&b, panelOpt{Title: "Bản đồ storyline", Sub: "Tầng tác động (ngang) × mức quan trọng (dọc) · kích thước = số bài · bấm để mở storyline",
		Viz: "storymap", Data: points, Height: 380})

	if syn := d.Synthesis; syn != nil && len(syn.CrossLinks) > 0 {
		nodes := map[string]netNode{}
		var order []string
		add := func(key, topic string) string {
			if p, ok := titles[key]; ok {
				if _, seen := nodes[key]; !seen {
					nodes[key] = netNode{ID: key, Label: p.Title, Topic: p.Topic, Slug: key}
					order = append(order, key)
				}
				return key
			}
			id := "txt:" + key
			if _, seen := nodes[id]; !seen {
				nodes[id] = netNode{ID: id, Label: key, Topic: topic}
				order = append(order, id)
			}
			return id
		}
		var links []netLink
		for _, l := range syn.CrossLinks {
			f := add(l.FromStory, l.FromTopic)
			t := add(l.ToStory, l.ToTopic)
			links = append(links, netLink{f, t, l.Mechanism, l.Implication})
		}
		var ns []netNode
		for _, k := range order {
			ns = append(ns, nodes[k])
		}
		panel(&b, panelOpt{Title: "Mạng liên kết chéo", Sub: "Storyline của chủ đề này tác động lên chủ đề khác · rê chuột vào cạnh để xem cơ chế",
			Viz: "network", Data: map[string]any{"nodes": ns, "links": links}, Height: 440, Legend: true})
	}
	if syn := d.Synthesis; syn != nil && len(syn.VNImpact) > 0 {
		panel(&b, panelOpt{Title: "Ma trận tác động tới Việt Nam", Sub: "Kỳ hạn × chiều tác động · rê chuột để đọc chi tiết",
			HTML: vnMatrix(syn.VNImpact)})
	}
	b.WriteString(`</div>`)
	return b.String()
}

func vnMatrix(items []analyze.VNImpact) string {
	dirs := []struct{ key, kind, label string }{
		{"positive", "good", "Tích cực"}, {"mixed", "warning", "Đan xen"}, {"uncertain", "neutral", "Chưa rõ"}, {"negative", "critical", "Tiêu cực"},
	}
	hz := []struct{ key, label string }{{"short", "Ngắn hạn"}, {"medium", "Trung hạn"}, {"long", "Dài hạn"}}
	var b strings.Builder
	b.WriteString(`<div class="matrix"><div></div>`)
	for _, d := range dirs {
		fmt.Fprintf(&b, `<div class="h">%s</div>`, statusHTML(d.kind, d.label))
	}
	for _, h := range hz {
		fmt.Fprintf(&b, `<div class="h">%s</div>`, h.label)
		for _, d := range dirs {
			b.WriteString(`<div class="cell">`)
			for _, it := range items {
				if it.Horizon == h.key && it.Direction == d.key {
					fmt.Fprintf(&b, `<span class="chip" title="%s (tin cậy: %s)">%s</span>`, esc(it.Impact), esc(it.Confidence), esc(it.Area))
				}
			}
			b.WriteString(`</div>`)
		}
	}
	b.WriteString(`</div><details class="tbl"><summary>Bảng dữ liệu</summary><table><tr><th>Lĩnh vực</th><th>Tác động</th><th>Chiều</th><th>Kỳ hạn</th><th>Tin cậy</th></tr>`)
	for _, it := range items {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`, esc(it.Area), esc(it.Impact), esc(it.Direction), esc(it.Horizon), esc(it.Confidence))
	}
	b.WriteString(`</table></details>`)
	return b.String()
}

// ---------- dashboard xu hướng nhiều ngày ----------

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	days := atoi(r.URL.Query().Get("days"), 14)
	now := time.Now().In(s.cfg.Loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.cfg.Loc).AddDate(0, 0, -days+1)
	var dayList []string
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		dayList = append(dayList, d.Format("2006-01-02"))
	}
	minRel := s.cfg.Analyze.MinRelevance
	vol, _ := s.st.TopicVolume(start, minRel, s.cfg.Loc)
	scale, _ := s.st.TopicFacets("scale", start, minRel)
	geo, _ := s.st.TopicFacets("geo", start, minRel)
	ents, _ := s.st.TopEntities(start, minRel, 20)
	sdays, _ := s.st.StorylineDays(start.Format("2006-01-02"))
	cost, _ := s.st.CostByDay(start, s.cfg.Loc)
	sls, _ := s.st.Storylines("", start.Format("2006-01-02"), 400)
	reports, _ := s.st.ReportDates("daily", 400)

	var b strings.Builder
	b.WriteString(`<h1>Dashboard xu hướng</h1><div class="filters">`)
	for _, n := range []int{7, 14, 30, 90} {
		on := ""
		if n == days {
			on = " on"
		}
		fmt.Fprintf(&b, `<a class="btn%s" href="/dashboard?days=%d">%d ngày</a>`, on, n, n)
	}
	b.WriteString(`</div>`)

	if len(vol) == 0 && len(sls) == 0 {
		b.WriteString(`<div class="empty">Chưa có dữ liệu trong khoảng này. Chạy <code>fa daily</code> để quét và phân tích tin.</div>`)
		s.render(w, "dashboard", "Dashboard", b.String())
		return
	}

	total := 0
	for _, v := range vol {
		total += v.N
	}
	escCount, multi := 0, 0
	for _, sl := range sls {
		if sl.Status == "escalating" {
			escCount++
		}
		if sl.DaysActive >= 2 {
			multi++
		}
	}
	costSum := 0.0
	for _, c := range cost {
		costSum += c.Cost
	}
	nrep := 0
	for _, d := range reports {
		if d >= dayList[0] {
			nrep++
		}
	}
	b.WriteString(`<div class="dash"><div class="kpis">`)
	kpi(&b, "Bài liên quan", fmt.Sprint(total), "bài", fmt.Sprintf("≈ %d/ngày", total/max(1, len(dayList))), "")
	kpi(&b, "Storyline hoạt động", fmt.Sprint(len(sls)), "", fmt.Sprintf("%d kéo dài ≥ 2 ngày", multi), "")
	kpi(&b, "Đang leo thang", fmt.Sprint(escCount), "storyline", "", "")
	kpi(&b, "Bản tin đã tạo", fmt.Sprint(nrep), "", "", "")
	kpi(&b, "Chi phí Claude", fmt.Sprintf("$%.2f", costSum), "", fmt.Sprintf("%d ngày", len(dayList)), "")
	b.WriteString(`</div>`)

	panel(&b, panelOpt{Title: "Khối lượng tin liên quan theo ngày", Sub: "Số bài có độ liên quan ≥ ngưỡng, theo chủ đề", Viz: "volume",
		Data: map[string]any{"days": dayList, "rows": vol}, Height: 300})
	panel(&b, panelOpt{Width: "half", Title: "Tầng tác động", Sub: "Tỷ lệ bài vi mô / trung mô / vĩ mô trong từng chủ đề", Viz: "facet",
		Data: map[string]any{"keys": []string{"micro", "meso", "macro"}, "labels": []string{"Vi mô", "Trung mô", "Vĩ mô"}, "palette": "level", "rows": scale}, Height: 220})
	panel(&b, panelOpt{Width: "half", Title: "Phạm vi địa lý", Sub: "Tỷ lệ tin về Việt Nam / quốc tế có liên hệ VN / quốc tế", Viz: "facet",
		Data: map[string]any{"keys": []string{"vn", "both", "intl"}, "labels": []string{"Việt Nam", "Quốc tế ↔ VN", "Quốc tế"}, "palette": "geo", "rows": geo}, Height: 220})

	// storyline × ngày: chọn tối đa 24 storyline nổi bật
	type agg struct {
		days, maxImp int
		last         string
	}
	ag := map[string]*agg{}
	for _, c := range sdays {
		a := ag[c.Slug]
		if a == nil {
			a = &agg{}
			ag[c.Slug] = a
		}
		a.days++
		a.maxImp = max(a.maxImp, c.Importance)
		if c.Date > a.last {
			a.last = c.Date
		}
	}
	type sitem struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
		Topic string `json:"topic"`
	}
	var items []sitem
	for _, sl := range sls {
		if ag[sl.Slug] != nil {
			items = append(items, sitem{sl.Slug, sl.Title, sl.Topic})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, c := ag[items[i].Slug], ag[items[j].Slug]
		if a.days != c.days {
			return a.days > c.days
		}
		if a.maxImp != c.maxImp {
			return a.maxImp > c.maxImp
		}
		return a.last > c.last
	})
	if len(items) > 24 {
		items = items[:24]
	}
	keep := map[string]bool{}
	for _, it := range items {
		keep[it.Slug] = true
	}
	var cells []any
	for _, c := range sdays {
		if keep[c.Slug] {
			cells = append(cells, c)
		}
	}
	panel(&b, panelOpt{Title: "Dòng thời gian storyline", Sub: "Mỗi hàng là một câu chuyện; ô = ngày có diễn biến, đậm = quan trọng hơn · bấm để mở",
		Viz: "storyheat", Data: map[string]any{"days": dayList, "stories": items, "cells": cells}})
	panel(&b, panelOpt{Width: "half", Title: "Thực thể được nhắc nhiều nhất", Sub: "Số bài liên quan nhắc tới, theo chủ đề · bấm để tìm bài",
		Viz: "entities", Data: ents, Legend: false})
	panel(&b, panelOpt{Width: "half", Title: "Chi phí Claude theo ngày", Sub: "USD theo total_cost_usd mà Claude CLI báo", Viz: "cost", Data: cost, Height: 260})
	b.WriteString(`</div>`)
	s.render(w, "dashboard", "Dashboard", b.String())
}

// ---------- dashboard phân tích sâu ----------

func deepDash(v *research.DeepViz, slug string) string {
	var b strings.Builder
	if v == nil {
		fmt.Fprintf(&b, `<div class="empty">Báo cáo này chưa có dashboard. Tạo bằng: <code>fa viz %s --open</code> (Claude rút số liệu, kịch bản, chuỗi nhân quả… từ báo cáo để vẽ chart).</div>`, esc(slug))
		return b.String()
	}
	b.WriteString(`<div class="dash">`)
	if v.Headline != "" {
		fmt.Fprintf(&b, `<div class="hero">%s</div>`, esc(v.Headline))
	}
	if len(v.KeyMetrics) > 0 {
		b.WriteString(`<div class="kpis">`)
		for _, m := range v.KeyMetrics {
			kpi(&b, m.Label, m.Value, m.Unit, m.Change, m.Note)
		}
		b.WriteString(`</div>`)
	}
	if len(v.CausalNodes) > 0 {
		panel(&b, panelOpt{Title: "Chuỗi tác động vi mô → trung mô → vĩ mô", Sub: "Mũi tên ghi kênh truyền dẫn; nét đậm = tác động mạnh · kéo/zoom được",
			Viz: "causal", Data: map[string]any{"nodes": v.CausalNodes, "edges": v.CausalEdges}})
	}
	if len(v.Scenarios) > 0 {
		var h strings.Builder
		h.WriteString(`<details class="tbl" open><summary>Điều kiện & tín hiệu sớm</summary><table><tr><th>Kịch bản</th><th class="num">Xác suất</th><th>Điều kiện</th><th>Tín hiệu sớm</th></tr>`)
		for _, sc := range v.Scenarios {
			fmt.Fprintf(&h, `<tr><td><b>%s</b></td><td class="num">%d%%</td><td>%s</td><td>%s</td></tr>`, esc(sc.Name), sc.Probability, esc(sc.Conditions), esc(sc.Signals))
		}
		h.WriteString(`</table></details>`)
		var b2 strings.Builder
		panel(&b2, panelOpt{Width: "half", Title: "Kịch bản", Sub: "Xác suất chủ quan theo báo cáo", Viz: "scenarios", Data: v.Scenarios, NoTable: true, Height: 200})
		// chèn bảng điều kiện vào cuối panel
		out := strings.TrimSuffix(b2.String(), `</section>`) + h.String() + `</section>`
		b.WriteString(out)
	}
	if len(v.VNImpacts) > 0 {
		panel(&b, panelOpt{Width: "half", Title: "Tác động tới Việt Nam", Sub: "Điểm −2 (rất tiêu cực) … +2 (rất tích cực)", Viz: "vnimpact", Data: v.VNImpacts, Legend: true, Height: 220})
	}
	if len(v.Stakeholders) > 0 {
		panel(&b, panelOpt{Width: "half", Title: "Bản đồ các bên liên quan", Sub: "Lập trường / được–mất (ngang) × mức ảnh hưởng tới kết cục (dọc)",
			Viz: "stakeholders", Data: v.Stakeholders, Legend: true, Height: 340})
	}
	if len(v.Timeline) > 0 {
		var h strings.Builder
		h.WriteString(`<ul class="tl">`)
		kinds := map[string]string{"fact": "dữ kiện", "claim": "tuyên bố", "forecast": "dự báo"}
		for _, e := range v.Timeline {
			fmt.Fprintf(&h, `<li class="%s"><span class="d">%s</span>%s<span class="badge">%s</span></li>`, esc(e.Kind), esc(e.Date), esc(e.Event), esc(kinds[e.Kind]))
		}
		h.WriteString(`</ul>`)
		panel(&b, panelOpt{Width: "half", Title: "Dòng thời gian", Sub: "● dữ kiện · ● xám = tuyên bố · ○ = dự báo", HTML: h.String()})
	}
	if len(v.Indicators) > 0 {
		var h strings.Builder
		h.WriteString(`<table><tr><th>Trạng thái</th><th>Chỉ báo</th><th>Hiện tại</th><th>Ngưỡng / mốc</th><th>Vì sao</th></tr>`)
		st := map[string][2]string{"ok": {"good", "ổn"}, "watch": {"warning", "để mắt"}, "alert": {"critical", "cảnh báo"}}
		order := map[string]int{"alert": 0, "watch": 1, "ok": 2}
		inds := append([]research.VizIndic(nil), v.Indicators...)
		sort.SliceStable(inds, func(i, j int) bool { return order[inds[i].Status] < order[inds[j].Status] })
		for _, in := range inds {
			k := st[in.Status]
			if k[0] == "" {
				k = [2]string{"neutral", in.Status}
			}
			fmt.Fprintf(&h, `<tr><td>%s</td><td><b>%s</b></td><td class="num">%s</td><td>%s</td><td>%s</td></tr>`,
				statusHTML(k[0], k[1]), esc(in.Name), esc(in.Current), esc(in.Threshold), esc(in.Why))
		}
		h.WriteString(`</table>`)
		panel(&b, panelOpt{Title: "Chỉ báo cần theo dõi", Sub: "Sắp theo mức khẩn: cảnh báo → để mắt → ổn", HTML: h.String()})
	}
	b.WriteString(`</div>`)
	return b.String()
}

// DeepURL trả về đường dẫn trang deep cho date + slug.
func DeepURL(date, slug string) string { return "/deep/" + date + "/" + url.PathEscape("deep:"+slug) }
