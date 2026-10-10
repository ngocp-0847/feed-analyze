// Package classify dùng Claude phân loại bài viết theo topic, mức độ quan trọng và tầng vi mô/vĩ mô.
package classify

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

const systemPrompt = `Bạn là biên tập viên phân loại tin tức cho một bản tin cá nhân. Nhiệm vụ: với mỗi bài (chỉ có tiêu đề + mô tả RSS), đánh giá mức liên quan tới từng chủ đề người dùng theo dõi, mức quan trọng, tầng tác động, và tóm tắt tiếng Việt.

Quy tắc:
- topics: chỉ liệt kê chủ đề có relevance >= 0.3 (0..1). Bài không thuộc chủ đề nào (thể thao, giải trí, đời sống, tai nạn, hình sự địa phương, quảng cáo, khuyến mãi...) → topics = [].
- relevance: 0.9+ = trọng tâm chủ đề; 0.6 = liên quan rõ; 0.3–0.5 = liên quan gián tiếp.
- importance (1–5), đánh giá theo tác động thực chất chứ không theo độ giật tít:
  5 = cú sốc mang tính hệ thống (quyết định lãi suất lớn, luật/nghị quyết quan trọng, leo thang xung đột, ra mắt model/nền tảng định hình ngành);
  4 = quan trọng với cả ngành hoặc cả quốc gia; 3 = đáng chú ý; 2 = nhỏ/cục bộ; 1 = nhiễu, PR, tin lặp.
- scale: micro (1 doanh nghiệp/người/sản phẩm/địa phương) | meso (ngành, thị trường, lĩnh vực chính sách, khu vực) | macro (nền kinh tế quốc gia, hệ thống toàn cầu, địa chính trị).
- geo: vn (về Việt Nam) | intl (quốc tế, không liên quan trực tiếp VN) | both (quốc tế có liên hệ trực tiếp tới VN).
- entities: tối đa 5 thực thể có tên (tổ chức, người, quốc gia, mã cổ phiếu, sản phẩm), giữ tên gốc.
- summary_vi: CHỈ viết khi có topic relevance >= 0.5: 1–2 câu tiếng Việt (<= 45 từ), dữ kiện cụ thể, giữ số liệu; dịch nếu bài tiếng Anh. Ngược lại để "".
- Trả về đủ mọi id được giao, không thêm id lạ.`

type output struct {
	Items []struct {
		ID         int64              `json:"id"`
		Topics     []store.TopicScore `json:"topics"`
		Importance int                `json:"importance"`
		Scale      string             `json:"scale"`
		Geo        string             `json:"geo"`
		Entities   []string           `json:"entities"`
		SummaryVI  string             `json:"summary_vi"`
	} `json:"items"`
}

func schema(topicIDs []string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer"},
						"topics": map[string]any{"type": "array", "items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"topic":     map[string]any{"type": "string", "enum": topicIDs},
								"relevance": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
							},
							"required": []string{"topic", "relevance"},
						}},
						"importance": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
						"scale":      map[string]any{"type": "string", "enum": []string{"micro", "meso", "macro"}},
						"geo":        map[string]any{"type": "string", "enum": []string{"vn", "intl", "both"}},
						"entities":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"summary_vi": map[string]any{"type": "string"},
					},
					"required": []string{"id", "topics", "importance", "scale", "geo", "summary_vi"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func topicBlock(cfg *config.Config) string {
	var b strings.Builder
	for _, t := range cfg.Topics {
		fmt.Fprintf(&b, "- %s — %s: %s", t.ID, t.Name, strings.TrimSpace(t.Description))
		if len(t.Keywords) > 0 {
			fmt.Fprintf(&b, " (từ khoá gợi ý: %s)", strings.Join(t.Keywords, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildPrompt(cfg *config.Config, batch []store.Article, hints map[string][]string) string {
	var b strings.Builder
	b.WriteString("## Chủ đề người dùng theo dõi\n")
	b.WriteString(topicBlock(cfg))
	b.WriteString("\n## Bài cần phân loại\n")
	for _, a := range batch {
		sum := a.Summary
		if r := []rune(sum); len(r) > 280 {
			sum = string(r[:280]) + "…"
		}
		hint := ""
		if h := hints[a.Feed]; len(h) > 0 {
			hint = " chuyên mục≈" + strings.Join(h, "/")
		}
		fmt.Fprintf(&b, "[%d] (%s, %s%s) %s", a.ID, a.Source, a.Region, hint, a.Title)
		if sum != "" && sum != a.Title {
			fmt.Fprintf(&b, " — %s", sum)
		}
		b.WriteString("\n")
	}
	return b.String()
}

type Progress struct {
	Done, Total, Relevant int
	Err                   error
}

// Run phân loại mọi bài chưa phân loại (tối đa limit bài mới nhất). Bài bị model bỏ sót được
// thử lại thêm tối đa 2 vòng.
func Run(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, limit int, progress func(Progress)) (Progress, error) {
	var total Progress
	var lastErr error
	for round := 0; round < 3 && limit > 0; round++ {
		arts, err := st.Unclassified(limit)
		if err != nil {
			return total, err
		}
		if len(arts) == 0 {
			break
		}
		if round == 0 {
			total.Total = len(arts)
		}
		base := total
		p, err := classifyArticles(ctx, cfg, st, r, arts, func(p Progress) {
			if progress != nil {
				progress(Progress{Done: base.Done + p.Done, Total: total.Total, Relevant: base.Relevant + p.Relevant, Err: p.Err})
			}
		})
		total.Done += p.Done
		total.Relevant += p.Relevant
		lastErr = err
		if p.Done == 0 || ctx.Err() != nil {
			break
		}
		limit -= p.Done
	}
	total.Err = lastErr
	return total, lastErr
}

func classifyArticles(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, arts []store.Article, progress func(Progress)) (Progress, error) {
	total := Progress{Total: len(arts)}
	if len(arts) == 0 {
		return total, nil
	}
	hints := map[string][]string{}
	for _, f := range cfg.Feeds {
		hints[f.Name] = f.Topics
	}
	var batches [][]store.Article
	for i := 0; i < len(arts); i += cfg.Claude.BatchSize {
		j := min(i+cfg.Claude.BatchSize, len(arts))
		batches = append(batches, arts[i:j])
	}
	sch := schema(cfg.TopicIDs())
	sem := make(chan struct{}, cfg.Claude.Concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []string
	for _, batch := range batches {
		wg.Add(1)
		go func(batch []store.Article) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			n, rel, err := classifyBatch(ctx, cfg, st, r, batch, hints, sch)
			mu.Lock()
			defer mu.Unlock()
			total.Done += n
			total.Relevant += rel
			if err != nil {
				errs = append(errs, err.Error())
			}
			if progress != nil {
				progress(Progress{Done: total.Done, Total: total.Total, Relevant: total.Relevant, Err: err})
			}
		}(batch)
	}
	wg.Wait()
	if len(errs) > 0 {
		total.Err = fmt.Errorf("%d/%d batch lỗi: %s", len(errs), len(batches), errs[0])
	}
	return total, total.Err
}

func classifyBatch(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, batch []store.Article,
	hints map[string][]string, sch map[string]any) (done, relevant int, err error) {
	var out output
	_, err = r.JSON(ctx, llm.Request{
		Purpose: "classify", Model: cfg.Claude.ClassifyModel, System: systemPrompt,
		Prompt: buildPrompt(cfg, batch, hints), Schema: sch, Isolated: true,
	}, &out)
	if err != nil {
		return 0, 0, err
	}
	valid := map[int64]bool{}
	for _, a := range batch {
		valid[a.ID] = true
	}
	topicOK := map[string]bool{}
	for _, id := range cfg.TopicIDs() {
		topicOK[id] = true
	}
	for _, it := range out.Items {
		if !valid[it.ID] {
			continue
		}
		delete(valid, it.ID)
		c := store.Classification{ArticleID: it.ID, Importance: clamp(it.Importance, 1, 5), Scale: it.Scale, Geo: it.Geo,
			Entities: it.Entities, SummaryVI: strings.TrimSpace(it.SummaryVI), Model: cfg.Claude.ClassifyModel}
		for _, t := range it.Topics {
			if topicOK[t.Topic] && t.Relevance >= 0.3 {
				c.Topics = append(c.Topics, t)
			}
		}
		sort.Slice(c.Topics, func(i, j int) bool { return c.Topics[i].Relevance > c.Topics[j].Relevance })
		if len(c.Topics) > 0 {
			c.Primary, c.Relevance = c.Topics[0].Topic, c.Topics[0].Relevance
			if c.Relevance >= cfg.Analyze.MinRelevance {
				relevant++
			}
		}
		if err := st.SaveClassification(c); err != nil {
			return done, relevant, err
		}
		done++
	}
	// id bị model bỏ sót: để lại, lần chạy sau sẽ thử lại
	return done, relevant, nil
}

// Redo phân loại lại các bài trong N ngày gần nhất (vd sau khi đổi topic).
func Redo(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, days int, progress func(Progress)) (Progress, error) {
	vs, err := st.Search(store.SearchOpts{Since: time.Now().Add(-time.Duration(days) * 24 * time.Hour), Limit: 5000})
	if err != nil {
		return Progress{}, err
	}
	arts := make([]store.Article, len(vs))
	for i, v := range vs {
		arts[i] = v.Article
	}
	return classifyArticles(ctx, cfg, st, r, arts, progress)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
