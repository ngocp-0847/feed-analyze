// Package research: dựng bối cảnh cho AI (fa context), phân tích sâu (fa deep) và hỏi đáp (fa ask).
package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/report"
	"github.com/ngocp-0847/feed-analyze/internal/skills"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// ---------- context ----------

// Context trả về markdown gọn: bản tin gần nhất (<= date) + storyline hoạt động trong N ngày.
func Context(cfg *config.Config, st *store.Store, date string, days int) (string, error) {
	if date == "" {
		date = cfg.Today()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Bối cảnh feed-analyze — %s\n\n**Người đọc:** %s\n\n", report.DateTitle(date, cfg.Loc), oneLine(cfg.Profile, 400))

	d, usedDate, err := latestDaily(st, date)
	if err != nil {
		return "", err
	}
	if d == nil {
		b.WriteString("⚠️ Chưa có bản tin nào. Chạy `fa daily` để tạo.\n\n")
	} else {
		if usedDate != date {
			fmt.Fprintf(&b, "⚠️ Chưa có bản tin ngày %s — dưới đây là bản tin gần nhất (%s). Chạy `fa daily` để cập nhật.\n\n", date, usedDate)
		}
		writeDailyBrief(&b, cfg, d)
	}

	since := mustDate(date, cfg.Loc).AddDate(0, 0, -days+1).Format("2006-01-02")
	sls, err := st.Storylines("", since, 150)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "## Storyline hoạt động %d ngày qua (%d)\n", days, len(sls))
	b.WriteString("_Chi tiết: `fa story <slug>` · tìm bài: `fa search \"...\"`_\n\n")
	byTopic := map[string][]store.Storyline{}
	for _, s := range sls {
		byTopic[s.Topic] = append(byTopic[s.Topic], s)
	}
	for _, t := range cfg.Topics {
		if len(byTopic[t.ID]) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s %s\n", t.Emoji, t.Name)
		for _, s := range byTopic[t.ID] {
			fmt.Fprintf(&b, "- `%s` [%s · %s · %d ngày · cuối %s] **%s** — %s\n", s.Slug, s.Status, analyze.LevelVI(s.Level),
				s.DaysActive, s.LastDate, s.Title, oneLine(s.Summary, 260))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func latestDaily(st *store.Store, date string) (*analyze.Daily, string, error) {
	dates, err := st.ReportDates("daily", 60)
	if err != nil {
		return nil, "", err
	}
	for _, d := range dates {
		if d <= date {
			dl, err := analyze.LoadDaily(st, d)
			return dl, d, err
		}
	}
	return nil, "", nil
}

func writeDailyBrief(b *strings.Builder, cfg *config.Config, d *analyze.Daily) {
	s := d.Synthesis
	if s == nil {
		fmt.Fprintf(b, "## Bản tin %s (chưa có phần tổng hợp)\n\n", d.Date)
	} else {
		fmt.Fprintf(b, "## Bản tin %s: %s\n\n**TL;DR**\n", d.Date, s.Title)
		for _, x := range s.TLDR {
			fmt.Fprintf(b, "- %s\n", x)
		}
		fmt.Fprintf(b, "\n**Bức tranh vĩ mô:** %s\n\n", oneLine(s.MacroPicture, 1500))
		if len(s.CrossLinks) > 0 {
			b.WriteString("**Liên kết chéo:**\n")
			for _, l := range s.CrossLinks {
				fmt.Fprintf(b, "- %s/%s → %s/%s: %s ⇒ %s\n", l.FromTopic, l.FromStory, l.ToTopic, l.ToStory, l.Mechanism, l.Implication)
			}
		}
		if len(s.VNImpact) > 0 {
			b.WriteString("\n**Tác động VN:**\n")
			for _, v := range s.VNImpact {
				fmt.Fprintf(b, "- %s (%s, %s): %s\n", v.Area, v.Direction, v.Horizon, v.Impact)
			}
		}
		if len(s.DeepDives) > 0 {
			b.WriteString("\n**Gợi ý đào sâu:**\n")
			for _, x := range s.DeepDives {
				fmt.Fprintf(b, "- `%s`: %s\n", x.Slug, x.Question)
			}
		}
		b.WriteString("\n")
	}
	for _, ta := range d.Topics {
		t, _ := cfg.Topic(ta.Topic)
		fmt.Fprintf(b, "**%s %s** (%d bài): %s\n", t.Emoji, t.Name, ta.ArticleCount, ta.Headline)
		for _, sl := range ta.Storylines {
			fmt.Fprintf(b, "- `%s` [%s, %s, imp %d] %s\n", sl.Slug, sl.Status, analyze.LevelVI(sl.Level), sl.Importance, sl.Title)
		}
	}
	b.WriteString("\n")
}

// ---------- search helpers ----------

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`là của và có cho những các được trong với này đó thì gì nào như thế sao không về một để tới từ bị ra vào hay hoặc đang sẽ đã
		tại vì nên làm ảnh hưởng tác động hiện nay hôm qua tuần tháng năm người việc nhiều ít rất cũng còn khi nếu
		the a an of to in and or is are was were what how why who which on for with by at from as be this that it its do does about`) {
		stop[w] = true
	}
}

// NaturalQuery biến câu hỏi tự nhiên thành truy vấn FTS5: cụm 2 từ liền kề OR từ đơn (bỏ stopword).
func NaturalQuery(q string) string {
	var toks []string
	for _, f := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' }) {
		f = strings.Trim(f, "-")
		if f != "" {
			toks = append(toks, f)
		}
	}
	var parts []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			parts = append(parts, `"`+p+`"`)
		}
	}
	for i := 0; i+1 < len(toks); i++ {
		if !stop[toks[i]] && !stop[toks[i+1]] {
			add(toks[i] + " " + toks[i+1])
		}
	}
	for _, t := range toks {
		if !stop[t] && len([]rune(t)) >= 2 {
			add(t)
		}
	}
	return strings.Join(parts, " OR ")
}

func writeArticles(b *strings.Builder, cfg *config.Config, vs []store.ArticleView, withLink bool) {
	for _, a := range vs {
		fmt.Fprintf(b, "- #%d [%s|%s|%s] %s", a.ID, a.Region, a.Source, a.Published.In(cfg.Loc).Format("2006-01-02"), a.Title)
		if a.Classification != nil && a.SummaryVI != "" {
			fmt.Fprintf(b, " — %s", a.SummaryVI)
		} else if a.Summary != "" {
			fmt.Fprintf(b, " — %s", oneLine(a.Summary, 250))
		}
		if withLink && a.Link != "" {
			fmt.Fprintf(b, " <%s>", a.Link)
		}
		b.WriteString("\n")
	}
}

// ---------- deep ----------

type Target struct {
	Kind  string // storyline | article | question
	Slug  string
	ID    int64
	Query string
}

func Resolve(st *store.Store, s string) Target {
	s = strings.TrimSpace(s)
	if id, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64); err == nil {
		return Target{Kind: "article", ID: id}
	}
	if sl, _ := st.Storyline(s); sl != nil {
		return Target{Kind: "storyline", Slug: s}
	}
	return Target{Kind: "question", Query: s}
}

// gather dựng phần dữ liệu nội bộ cho deep/ask; trả về (markdown, slug lưu file, tiêu đề).
func gather(cfg *config.Config, st *store.Store, t Target) (string, string, string, error) {
	var b strings.Builder
	since := time.Now().AddDate(0, 0, -30)
	switch t.Kind {
	case "storyline":
		sl, err := st.Storyline(t.Slug)
		if err != nil || sl == nil {
			return "", "", "", fmt.Errorf("không tìm thấy storyline %q", t.Slug)
		}
		fmt.Fprintf(&b, "## Storyline `%s`: %s\nChủ đề: %s · tầng %s · trạng thái %s · %d ngày (%s → %s)\n\n%s\n\nVì sao quan trọng: %s\n\n",
			sl.Slug, sl.Title, sl.Topic, analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.FirstDate, sl.LastDate, sl.Summary, sl.Why)
		evs, _ := st.StorylineEvents(sl.Slug)
		b.WriteString("### Diễn biến theo ngày\n")
		for _, e := range evs {
			var d analyze.StorylineOut
			_ = json.Unmarshal([]byte(e.Detail), &d)
			fmt.Fprintf(&b, "- **%s** [%s] %s", e.Date, e.Status, e.Summary)
			if len(d.CausalChain) > 0 {
				fmt.Fprintf(&b, " | chuỗi: %s", analyze.ChainString(d.CausalChain))
			}
			b.WriteString("\n")
		}
		ids, _ := st.StorylineArticleIDs(sl.Slug)
		vs, _ := st.ArticlesByIDs(ids)
		fmt.Fprintf(&b, "\n### Bài viết của storyline (%d)\n", len(vs))
		writeArticles(&b, cfg, vs, true)
		return b.String(), sl.Slug, sl.Title, nil
	case "article":
		vs, err := st.ArticlesByIDs([]int64{t.ID})
		if err != nil || len(vs) == 0 {
			return "", "", "", fmt.Errorf("không tìm thấy bài #%d", t.ID)
		}
		a := vs[0]
		b.WriteString("## Bài viết trọng tâm\n")
		writeArticles(&b, cfg, vs, true)
		if a.Summary != "" {
			fmt.Fprintf(&b, "\nMô tả RSS: %s\n", a.Summary)
		}
		q := a.Title
		if a.Classification != nil && len(a.Entities) > 0 {
			q = strings.Join(a.Entities, " ") + " " + a.Title
		}
		rel, _ := st.Search(store.SearchOpts{Raw: NaturalQuery(q), Since: since, Limit: 15, OnlyOn: true})
		b.WriteString("\n### Bài liên quan (30 ngày)\n")
		writeArticles(&b, cfg, without(rel, a.ID), true)
		return b.String(), fmt.Sprintf("article-%d", a.ID), a.Title, nil
	default:
		rel, err := st.Search(store.SearchOpts{Raw: NaturalQuery(t.Query), Since: since, Limit: 25})
		if err != nil {
			return "", "", "", err
		}
		fmt.Fprintf(&b, "## Câu hỏi\n%s\n\n### Bài liên quan trong kho (30 ngày, %d)\n", t.Query, len(rel))
		writeArticles(&b, cfg, rel, true)
		sls, _ := st.Storylines("", time.Now().AddDate(0, 0, -14).Format("2006-01-02"), 80)
		b.WriteString("\n### Storyline 14 ngày qua\n")
		for _, s := range sls {
			fmt.Fprintf(&b, "- `%s` [%s, %s] %s — %s\n", s.Slug, s.Topic, s.LastDate, s.Title, oneLine(s.Summary, 160))
		}
		slug := analyze.Slugify(t.Query)
		if len(slug) > 50 {
			slug = slug[:50]
		}
		return b.String(), "q-" + strings.Trim(slug, "-"), t.Query, nil
	}
}

func without(vs []store.ArticleView, id int64) []store.ArticleView {
	out := vs[:0:0]
	for _, v := range vs {
		if v.ID != id {
			out = append(out, v)
		}
	}
	return out
}

const deepInstructions = `## Nhiệm vụ: PHÂN TÍCH SÂU
Bạn có WebSearch và WebFetch. Quy trình:
1. Đọc dữ liệu nội bộ ở trên. Chọn 2–4 bài/nguồn quan trọng nhất và WebFetch để đọc toàn văn (ưu tiên nguồn gốc: văn bản chính thức, thông cáo, số liệu).
2. WebSearch bổ sung: bối cảnh, số liệu mới nhất, phản ứng các bên, góc nhìn trái chiều. Tối đa ~8 lượt tool — tập trung.
3. Áp dụng khung vi mô ↔ vĩ mô bên dưới.

Viết báo cáo markdown tiếng Việt (chỉ báo cáo, không kể quá trình), đúng cấu trúc:
# <tiêu đề>
## 1. Tóm tắt 30 giây
## 2. Diễn biến & bối cảnh  (timeline có ngày)
## 3. Chuỗi tác động vi mô → trung mô → vĩ mô  (mỗi mắt xích ghi kênh truyền dẫn; có thể kèm sơ đồ mermaid)
## 4. Các bên liên quan & động cơ
## 5. Liên kết chéo sang chủ đề khác
## 6. Tác động tới Việt Nam & tới người đọc
## 7. Kịch bản  (bảng: kịch bản | điều kiện | xác suất chủ quan | tín hiệu sớm)
## 8. Chỉ báo cần theo dõi  (cụ thể, có ngưỡng/mốc thời gian)
## 9. Điểm mù & phản biện
## 10. Nguồn  (link đã đọc)
Ghi rõ đâu là dữ kiện, đâu là suy luận; mức tin cậy cho kết luận lớn.
`

type ProgressFunc func(llm.Event)

// Deep chạy phân tích sâu; lưu report (kind deep:<slug>) + file reports/deep/. Trả về (markdown, path).
func Deep(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, target string, onEvent ProgressFunc) (string, string, error) {
	t := Resolve(st, target)
	data, slug, title, err := gather(cfg, st, t)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Yêu cầu phân tích sâu: %s\nHôm nay: %s\n\n## Người đọc\n%s\n\n# Dữ liệu nội bộ (từ kho tin RSS đã phân loại)\n%s\n%s\n## Khung phân tích\n%s",
		title, report.DateTitle(cfg.Today(), cfg.Loc), strings.TrimSpace(cfg.Profile), data, deepInstructions, skills.Lens())
	md, _, err := r.Agent(ctx, llm.Request{
		Purpose: "deep", Model: cfg.Claude.DeepModel, Prompt: b.String(), Tools: []string{"WebSearch", "WebFetch"},
		AppendSystem: "Bạn là nhà phân tích chiến lược. Trả lời bằng tiếng Việt. Kết quả cuối cùng chỉ gồm báo cáo markdown.",
		Isolated:     true,
	}, onEvent)
	if err != nil {
		return md, "", err
	}
	md = strings.TrimSpace(md) + "\n"
	date := cfg.Today()
	_ = st.SaveReport(store.Report{Date: date, Kind: "deep:" + slug, Markdown: md, Model: cfg.Claude.DeepModel})
	path, err := report.WriteFiles(cfg, date+"-"+slug, md, "deep")
	return md, path, err
}

// Ask trả lời câu hỏi dựa trên kho tin (+ web nếu web=true). Kết quả được ghi ra w.
func Ask(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, question string, web bool, onEvent ProgressFunc) (string, error) {
	t := Resolve(st, question)
	if t.Kind != "question" {
		t = Target{Kind: "question", Query: question}
	}
	data, _, _, err := gather(cfg, st, t)
	if err != nil {
		return "", err
	}
	brief, _ := Context(cfg, st, "", 3)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n# Dữ liệu tìm được cho câu hỏi\n%s\n\n## Yêu cầu\nTrả lời câu hỏi của người đọc bằng tiếng Việt, ngắn gọn (<= 400 từ trừ khi cần), dựa trên dữ liệu trên",
		brief, data)
	var tools []string
	if web {
		tools = []string{"WebSearch", "WebFetch"}
		b.WriteString(" và nghiên cứu web khi kho tin chưa đủ (tối đa ~5 lượt tool)")
	}
	b.WriteString(". Trích #id bài hoặc link làm dẫn chứng. Nối vi mô ↔ vĩ mô khi phù hợp. Nếu dữ liệu không đủ, nói rõ và gợi ý `fa deep` hoặc `fa ask --web`.\n\nCâu hỏi: ")
	b.WriteString(question)
	ans, _, err := r.Agent(ctx, llm.Request{
		Purpose: "ask", Model: cfg.Claude.DeepModel, Prompt: b.String(), Tools: tools, Isolated: true,
		AppendSystem: "Bạn là trợ lý tin tức cá nhân. Trả lời bằng tiếng Việt.",
	}, onEvent)
	return ans, err
}

// PrintEvents in các bước dùng tool ra terminal.
func PrintEvents(w io.Writer) ProgressFunc {
	return func(e llm.Event) {
		if e.Kind == "tool" {
			fmt.Fprintf(w, "  🔧 %s %s\n", e.Name, e.Text)
		}
	}
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func mustDate(d string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", d, loc)
	if err != nil {
		return time.Now().In(loc)
	}
	return t
}
