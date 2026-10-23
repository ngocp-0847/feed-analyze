// Package analyze: Claude gom bài thành storyline theo từng chủ đề, dựng chuỗi nhân quả vi mô → vĩ mô,
// rồi tổng hợp liên chủ đề thành bản tin ngày.
package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/skills"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

type Options struct {
	Date   string   // YYYY-MM-DD (theo timezone cấu hình); rỗng = hôm nay
	Topics []string // rỗng = mọi topic
	Log    func(format string, args ...any)
}

// Window trả về khoảng thời gian bài viết cho bản tin của 1 ngày.
func Window(cfg *config.Config, date string) (since, until time.Time, err error) {
	if date == "" || date == cfg.Today() {
		until = time.Now()
	} else {
		d, err := time.ParseInLocation("2006-01-02", date, cfg.Loc)
		if err != nil {
			return since, until, fmt.Errorf("ngày không hợp lệ %q (cần YYYY-MM-DD)", date)
		}
		until = d.Add(24 * time.Hour)
	}
	since = until.Add(-time.Duration(cfg.Analyze.WindowHours) * time.Hour)
	return since, until, nil
}

// Run phân tích từng topic rồi tổng hợp; lưu storyline + report. Trả về bản tin ngày.
func Run(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, o Options) (*Daily, error) {
	if o.Date == "" {
		o.Date = cfg.Today()
	}
	logf := o.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	since, until, err := Window(cfg, o.Date)
	if err != nil {
		return nil, err
	}
	topics := cfg.Topics
	if len(o.Topics) > 0 {
		topics = nil
		for _, id := range o.Topics {
			t, ok := cfg.Topic(id)
			if !ok {
				return nil, fmt.Errorf("không có topic %q (xem `fa topics`)", id)
			}
			topics = append(topics, t)
		}
	}

	results := make([]TopicAnalysis, len(topics))
	errs := make([]error, len(topics))
	sem := make(chan struct{}, cfg.Claude.Concurrency)
	var wg sync.WaitGroup
	for i, t := range topics {
		wg.Add(1)
		go func(i int, t config.Topic) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			start := time.Now()
			ta, err := analyzeTopic(ctx, cfg, st, r, t, o.Date, since, until)
			if err != nil {
				errs[i] = fmt.Errorf("topic %s: %w", t.ID, err)
				logf("  ✗ %s %s: %v", t.Emoji, t.Name, err)
				return
			}
			results[i] = *ta
			logf("  ✓ %s %s: %d bài → %d storyline (%s)", t.Emoji, t.Name, ta.ArticleCount, len(ta.Storylines), time.Since(start).Round(time.Second))
		}(i, t)
	}
	wg.Wait()

	daily := &Daily{Date: o.Date}
	// giữ kết quả cũ của topic không chạy lại (khi --topic) để bản tin ngày vẫn đầy đủ
	if prev, _ := LoadDaily(st, o.Date); prev != nil && len(o.Topics) > 0 {
		done := map[string]bool{}
		for _, t := range topics {
			done[t.ID] = true
		}
		for _, pt := range prev.Topics {
			if !done[pt.Topic] {
				daily.Topics = append(daily.Topics, pt)
			}
		}
	}
	var firstErr error
	for i := range topics {
		if errs[i] != nil {
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		daily.Topics = append(daily.Topics, results[i])
	}
	order := map[string]int{}
	for i, t := range cfg.Topics {
		order[t.ID] = i
	}
	sort.SliceStable(daily.Topics, func(i, j int) bool { return order[daily.Topics[i].Topic] < order[daily.Topics[j].Topic] })
	if len(daily.Topics) == 0 {
		return nil, fmt.Errorf("mọi topic đều lỗi: %w", firstErr)
	}

	logf("  … tổng hợp liên chủ đề (%s)", cfg.Claude.SynthesisModel)
	start := time.Now()
	syn, err := synthesize(ctx, cfg, r, daily)
	if err != nil {
		logf("  ✗ tổng hợp lỗi: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		daily.Synthesis = syn
		logf("  ✓ tổng hợp xong (%s)", time.Since(start).Round(time.Second))
	}

	b, _ := json.Marshal(daily)
	if err := st.SaveReport(store.Report{Date: o.Date, Kind: "daily", JSON: string(b), Model: cfg.Claude.SynthesisModel}); err != nil {
		return daily, err
	}
	return daily, firstErr
}

func LoadDaily(st *store.Store, date string) (*Daily, error) {
	rep, err := st.GetReport(date, "daily")
	if err != nil || rep == nil {
		return nil, err
	}
	d := &Daily{}
	if err := json.Unmarshal([]byte(rep.JSON), d); err != nil {
		return nil, err
	}
	return d, nil
}

// ---------- per-topic ----------

const topicSystem = `Bạn là nhà phân tích tin tức cấp cao (kinh tế, chính trị, công nghệ), viết cho một người Việt muốn hiểu sâu nhưng có ít thời gian.
Nhiệm vụ: từ danh sách bài (tiêu đề + tóm tắt) của MỘT chủ đề trong ~1 ngày, gom thành các storyline (câu chuyện) và phân tích theo khung vi mô → trung mô → vĩ mô.
Viết tiếng Việt, súc tích, có số liệu và tên riêng. Chỉ dựa trên dữ kiện được cung cấp; suy luận phải hợp lý và thể hiện qua confidence.`

func analyzeTopic(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, t config.Topic, date string, since, until time.Time) (*TopicAnalysis, error) {
	arts, err := st.TopicArticles(t.ID, since, until, cfg.Analyze.MinRelevance, cfg.Analyze.MaxArticles)
	if err != nil {
		return nil, err
	}
	ta := &TopicAnalysis{Topic: t.ID, ArticleCount: len(arts)}
	if len(arts) == 0 {
		ta.Headline = "Không có tin đáng chú ý trong cửa sổ thời gian này."
		return ta, nil
	}
	memSince := mustDate(date, cfg.Loc).AddDate(0, 0, -cfg.Analyze.StorylineMemDays).Format("2006-01-02")
	prev, err := st.Storylines(t.ID, memSince, 40)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Chủ đề: %s %s\n%s\n\n", t.Emoji, t.Name, strings.TrimSpace(t.Description))
	fmt.Fprintf(&b, "## Người đọc\n%s\n\n", strings.TrimSpace(cfg.Profile))
	fmt.Fprintf(&b, "## Ngày bản tin: %s\n\n", date)
	b.WriteString("## Storyline đã theo dõi gần đây (dùng lại đúng slug nếu tin hôm nay tiếp nối câu chuyện)\n")
	if len(prev) == 0 {
		b.WriteString("(chưa có)\n")
	}
	for _, p := range prev {
		fmt.Fprintf(&b, "- `%s` [%s, %s, %d ngày, lần cuối %s] %s — %s\n", p.Slug, p.Status, p.Level, p.DaysActive, p.LastDate, p.Title, oneLine(p.Summary, 220))
	}
	fmt.Fprintf(&b, "\n## Bài viết (%d, xếp theo mức quan trọng)\n", len(arts))
	ids := map[int64]bool{}
	for _, a := range arts {
		ids[a.ID] = true
		fmt.Fprintf(&b, "#%d [%s|%s|%s] imp=%d %s %s", a.ID, a.Region, a.Source, a.Published.In(cfg.Loc).Format("02/01 15:04"),
			a.Importance, a.Scale, a.Title)
		if a.SummaryVI != "" {
			fmt.Fprintf(&b, " — %s", a.SummaryVI)
		}
		if len(a.Entities) > 0 {
			fmt.Fprintf(&b, " {%s}", strings.Join(a.Entities, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString(`
## Yêu cầu
1. Gom bài thành 3–8 storyline quan trọng nhất (bỏ qua tin vụn). Một storyline = một câu chuyện có diễn biến; gộp các bài cùng sự kiện từ nhiều nguồn (VN & quốc tế).
2. Với storyline tiếp nối câu chuyện cũ: dùng lại slug cũ, status = developing/escalating/cooling/resolved, what_changed nêu rõ điều mới.
3. causal_chain: 2–5 mắt xích từ sự kiện gốc tới tác động ở tầng cao hơn (hoặc ngược lại từ vĩ mô xuống vi mô); mỗi mắt xích (trừ đầu tiên) ghi channel — kênh truyền dẫn.
4. vn_angle: tác động/liên hệ với Việt Nam (ai được, ai mất, qua kênh nào) — rỗng nếu thực sự không có.
5. watch_next: 1–3 chỉ báo/sự kiện cụ thể để kiểm chứng.
6. article_ids: chỉ dùng id có trong danh sách.
7. signals: tín hiệu yếu (bài ít được chú ý nhưng có thể quan trọng). questions: 2–3 câu hỏi đáng đào sâu.
Xếp storyline theo importance giảm dần.

## Khung phân tích tham chiếu
`)
	b.WriteString(skills.Lens())

	var out TopicAnalysis
	if _, err := r.JSON(ctx, llm.Request{
		Purpose: "analyze:" + t.ID, Model: cfg.Claude.AnalyzeModel, System: topicSystem,
		Prompt: b.String(), Schema: topicSchema, Isolated: true,
	}, &out); err != nil {
		return nil, err
	}
	out.Topic, out.ArticleCount = t.ID, len(arts)
	if err := st.ResetTopicDate(t.ID, date); err != nil {
		return nil, err
	}

	prevSlugs := map[string]bool{}
	for _, p := range prev {
		if p.FirstDate < date {
			prevSlugs[p.Slug] = true
		}
	}
	seen := map[string]bool{}
	kept := out.Storylines[:0]
	for _, sl := range out.Storylines {
		sl.Slug = Slugify(sl.Slug)
		if sl.Slug == "" {
			sl.Slug = Slugify(t.ID + "-" + sl.Title)
		}
		if seen[sl.Slug] {
			continue
		}
		seen[sl.Slug] = true
		// trạng thái theo góc nhìn của hệ thống: lần đầu thấy = new, đã thấy trước đó ≠ new
		if sl.Status == "new" && prevSlugs[sl.Slug] {
			sl.Status = "developing"
		} else if sl.Status == "developing" && !prevSlugs[sl.Slug] {
			sl.Status = "new"
		}
		var good []int64
		for _, id := range sl.ArticleIDs {
			if ids[id] {
				good = append(good, id)
			}
		}
		sl.ArticleIDs = good
		detail, _ := json.Marshal(sl)
		if err := st.UpsertStoryline(store.Storyline{
			Slug: sl.Slug, Topic: t.ID, Title: sl.Title, Status: sl.Status, Level: sl.Level,
			Summary: sl.Summary, Why: sl.WhyItMatters,
		}, date, string(detail), good); err != nil {
			return nil, err
		}
		kept = append(kept, sl)
	}
	out.Storylines = kept
	return &out, nil
}

// ---------- synthesis ----------

const synthesisSystem = `Bạn là tổng biên tập kiêm chiến lược gia, viết bản tin buổi sáng cho một người Việt bận rộn.
Bạn nhận phân tích theo từng chủ đề; nhiệm vụ là nhìn XUYÊN chủ đề: các lực vĩ mô nào đang vận động, chúng tương tác ra sao (AI ↔ kinh tế ↔ chính trị ↔ an ninh mạng — theo các chủ đề được cung cấp),
sự kiện vi mô nào là triệu chứng của xu hướng lớn, và điều đó có ý nghĩa gì với Việt Nam và với chính người đọc.
Viết tiếng Việt, sắc, cụ thể, không sáo rỗng. Không lặp lại nguyên văn phân tích chủ đề — hãy tổng hợp và nâng tầm.`

func synthesize(ctx context.Context, cfg *config.Config, r *llm.Runner, d *Daily) (*Synthesis, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bản tin ngày %s\n\n## Người đọc\n%s\n\n", d.Date, strings.TrimSpace(cfg.Profile))
	for _, ta := range d.Topics {
		t, _ := cfg.Topic(ta.Topic)
		fmt.Fprintf(&b, "## %s %s (topic id: %s, %d bài)\n**Headline:** %s\n", t.Emoji, t.Name, ta.Topic, ta.ArticleCount, ta.Headline)
		for _, sl := range ta.Storylines {
			fmt.Fprintf(&b, "\n### `%s` — %s [%s, %s, imp %d, tin cậy %s]\n%s\n", sl.Slug, sl.Title, sl.Status, sl.Level, sl.Importance, sl.Confidence, sl.Summary)
			if sl.WhatChanged != "" {
				fmt.Fprintf(&b, "- Mới: %s\n", sl.WhatChanged)
			}
			fmt.Fprintf(&b, "- Vì sao quan trọng: %s\n", sl.WhyItMatters)
			if len(sl.CausalChain) > 0 {
				fmt.Fprintf(&b, "- Chuỗi: %s\n", ChainString(sl.CausalChain))
			}
			if sl.VNAngle != "" {
				fmt.Fprintf(&b, "- VN: %s\n", sl.VNAngle)
			}
		}
		if len(ta.Signals) > 0 {
			fmt.Fprintf(&b, "\nTín hiệu yếu: %s\n", strings.Join(ta.Signals, "; "))
		}
		b.WriteString("\n")
	}
	b.WriteString(`## Yêu cầu
- tldr: 5–7 ý quan trọng nhất toàn bản tin (xếp theo tác động), mỗi ý ghi rõ tầng nếu hữu ích.
- macro_picture: 2–3 đoạn về các lực vĩ mô đang vận động và cách chúng tương tác; nối sự kiện vi mô với xu hướng lớn.
- cross_links: 2–6 liên kết chéo GIỮA các chủ đề khác nhau (from_topic ≠ to_topic), dùng slug storyline; mechanism = kênh truyền dẫn.
- vn_impact: 3–6 tác động tới Việt Nam (lĩnh vực, chiều, kỳ hạn, độ tin cậy).
- for_you: 2–4 ý nghĩa thiết thực với người đọc theo profile (nghề nghiệp, tài chính cá nhân, kỹ năng).
- watchlist: 3–6 sự kiện/chỉ báo sắp tới.
- deep_dives: 2–4 storyline đáng đào sâu nhất kèm câu hỏi sắc.
- contrarian: 1 góc nhìn ngược có lập luận.
`)
	var out Synthesis
	if _, err := r.JSON(ctx, llm.Request{
		Purpose: "synthesis", Model: cfg.Claude.SynthesisModel, System: synthesisSystem,
		Prompt: b.String(), Schema: synthesisSchema, Isolated: true,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- helpers ----------

var levelVI = map[string]string{"micro": "vi mô", "meso": "trung mô", "macro": "vĩ mô"}

func LevelVI(l string) string {
	if v, ok := levelVI[l]; ok {
		return v
	}
	return l
}

// ChainString: "[vi mô] A →(kênh) [trung mô] B → …"
func ChainString(c []ChainStep) string {
	var parts []string
	for i, s := range c {
		p := fmt.Sprintf("[%s] %s", LevelVI(s.Level), s.Step)
		if i > 0 && s.Channel != "" {
			p = fmt.Sprintf("—(%s)→ %s", s.Channel, p)
		} else if i > 0 {
			p = "→ " + p
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

var viFold = strings.NewReplacer(
	"à", "a", "á", "a", "ạ", "a", "ả", "a", "ã", "a", "â", "a", "ầ", "a", "ấ", "a", "ậ", "a", "ẩ", "a", "ẫ", "a",
	"ă", "a", "ằ", "a", "ắ", "a", "ặ", "a", "ẳ", "a", "ẵ", "a", "è", "e", "é", "e", "ẹ", "e", "ẻ", "e", "ẽ", "e",
	"ê", "e", "ề", "e", "ế", "e", "ệ", "e", "ể", "e", "ễ", "e", "ì", "i", "í", "i", "ị", "i", "ỉ", "i", "ĩ", "i",
	"ò", "o", "ó", "o", "ọ", "o", "ỏ", "o", "õ", "o", "ô", "o", "ồ", "o", "ố", "o", "ộ", "o", "ổ", "o", "ỗ", "o",
	"ơ", "o", "ờ", "o", "ớ", "o", "ợ", "o", "ở", "o", "ỡ", "o", "ù", "u", "ú", "u", "ụ", "u", "ủ", "u", "ũ", "u",
	"ư", "u", "ừ", "u", "ứ", "u", "ự", "u", "ử", "u", "ữ", "u", "ỳ", "y", "ý", "y", "ỵ", "y", "ỷ", "y", "ỹ", "y", "đ", "d",
)

// Slugify: chữ thường, bỏ dấu tiếng Việt, kebab-case, tối đa 60 ký tự.
func Slugify(s string) string {
	s = viFold.Replace(strings.ToLower(strings.TrimSpace(s)))
	s = strings.Trim(nonSlug.ReplaceAllString(s, "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
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
