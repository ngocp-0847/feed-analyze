package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// DeepViz là dữ liệu có cấu trúc rút từ báo cáo phân tích sâu — nguồn cho dashboard.
type DeepViz struct {
	Headline     string        `json:"headline"`
	KeyMetrics   []VizMetric   `json:"key_metrics"`
	Timeline     []VizEvent    `json:"timeline"`
	CausalNodes  []VizNode     `json:"causal_nodes"`
	CausalEdges  []VizEdge     `json:"causal_edges"`
	Stakeholders []VizActor    `json:"stakeholders"`
	Scenarios    []VizScenario `json:"scenarios"`
	VNImpacts    []VizImpact   `json:"vn_impacts"`
	Indicators   []VizIndic    `json:"indicators"`
}

type VizMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Unit   string `json:"unit"`
	Change string `json:"change"`
	Note   string `json:"note"`
}

type VizEvent struct {
	Date  string `json:"date"`
	Event string `json:"event"`
	Kind  string `json:"kind"` // fact | claim | forecast
}

type VizNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Level string `json:"level"` // micro | meso | macro
}

type VizEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Channel  string `json:"channel"`
	Strength int    `json:"strength"` // 1..3
}

type VizActor struct {
	Name      string `json:"name"`
	Stance    int    `json:"stance"`    // -2 bị thiệt / phản đối … +2 hưởng lợi / ủng hộ
	Influence int    `json:"influence"` // 1..5
	Interest  string `json:"interest"`
}

type VizScenario struct {
	Name        string `json:"name"`
	Probability int    `json:"probability"` // 0..100
	Conditions  string `json:"conditions"`
	Signals     string `json:"signals"`
}

type VizImpact struct {
	Sector string `json:"sector"`
	Score  int    `json:"score"` // -2..+2
	Why    string `json:"why"`
}

type VizIndic struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Threshold string `json:"threshold"`
	Why       string `json:"why"`
	Status    string `json:"status"` // ok | watch | alert
}

func s(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func in(desc string, lo, hi int) map[string]any {
	return map[string]any{"type": "integer", "minimum": lo, "maximum": hi, "description": desc}
}
func en(vals ...string) map[string]any { return map[string]any{"type": "string", "enum": vals} }
func ar(item any) map[string]any       { return map[string]any{"type": "array", "items": item} }
func ob(props map[string]any, req ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": req}
}

var vizSchema = ob(map[string]any{
	"headline": s("1 câu kết luận chính"),
	"key_metrics": ar(ob(map[string]any{
		"label": s("tên chỉ số"), "value": s("giá trị ngắn, vd 25.830 hoặc 13.500"), "unit": s("đơn vị, vd đ/USD, tỷ đồng, %"),
		"change": s("biến động có dấu, vd -130 đ/tuần; rỗng nếu không có"), "note": s("ghi chú rất ngắn"),
	}, "label", "value", "unit", "change", "note")),
	"timeline": ar(ob(map[string]any{
		"date": s("YYYY-MM-DD hoặc YYYY-MM"), "event": s("sự kiện ngắn"), "kind": en("fact", "claim", "forecast"),
	}, "date", "event", "kind")),
	"causal_nodes": ar(ob(map[string]any{
		"id": s("mã ngắn duy nhất, vd n1"), "label": s("mắt xích, <= 12 từ"), "level": en("micro", "meso", "macro"),
	}, "id", "label", "level")),
	"causal_edges": ar(ob(map[string]any{
		"from": s("id nút nguồn"), "to": s("id nút đích"), "channel": s("kênh truyền dẫn ngắn"), "strength": in("độ mạnh 1–3", 1, 3),
	}, "from", "to", "channel", "strength")),
	"stakeholders": ar(ob(map[string]any{
		"name": s("bên liên quan"), "stance": in("-2 bị thiệt/phản đối … +2 hưởng lợi/ủng hộ", -2, 2),
		"influence": in("ảnh hưởng tới kết cục 1–5", 1, 5), "interest": s("lợi ích/động cơ ngắn"),
	}, "name", "stance", "influence", "interest")),
	"scenarios": ar(ob(map[string]any{
		"name": s("tên kịch bản"), "probability": in("xác suất chủ quan %", 0, 100),
		"conditions": s("điều kiện"), "signals": s("tín hiệu sớm"),
	}, "name", "probability", "conditions", "signals")),
	"vn_impacts": ar(ob(map[string]any{
		"sector": s("lĩnh vực/nhóm ở VN"), "score": in("-2 rất tiêu cực … +2 rất tích cực", -2, 2), "why": s("lý do ngắn"),
	}, "sector", "score", "why")),
	"indicators": ar(ob(map[string]any{
		"name": s("chỉ báo"), "current": s("giá trị hiện tại, rỗng nếu không rõ"), "threshold": s("ngưỡng/mốc cần chú ý"),
		"why": s("vì sao"), "status": en("ok", "watch", "alert"),
	}, "name", "current", "threshold", "why", "status")),
}, "headline", "key_metrics", "timeline", "causal_nodes", "causal_edges", "stakeholders", "scenarios", "vn_impacts", "indicators")

const vizSystem = `Bạn chuyển một báo cáo phân tích (markdown tiếng Việt) thành dữ liệu có cấu trúc để vẽ dashboard.
CHỈ dùng thông tin có trong báo cáo, không thêm dữ kiện mới. Giữ tiếng Việt, ngắn gọn để hiển thị trên chart.
- key_metrics: 3–6 con số quan trọng nhất (giá trị định lượng có trong báo cáo).
- timeline: 4–12 mốc, sắp theo thời gian; kind=forecast cho dự báo/kế hoạch, claim cho tuyên bố chưa kiểm chứng.
- causal_nodes/edges: 5–12 nút đi từ vi mô → trung mô → vĩ mô, đồ thị liên thông, edge.channel là kênh truyền dẫn.
- stakeholders: 3–8 bên; scenarios: 2–4 kịch bản, tổng xác suất ≈ 100 nếu báo cáo có xác suất (nếu không, ước theo mô tả của báo cáo).
- vn_impacts: 2–6 lĩnh vực ở Việt Nam; indicators: 3–8 chỉ báo cần theo dõi, status=alert nếu đang vượt/sát ngưỡng, watch nếu cần để mắt.`

// ExtractViz rút dữ liệu dashboard từ báo cáo markdown (Claude, structured output).
func ExtractViz(ctx context.Context, cfg *config.Config, r *llm.Runner, md string) (*DeepViz, error) {
	var v DeepViz
	if _, err := r.JSON(ctx, llm.Request{
		Purpose: "deepviz", Model: cfg.Claude.AnalyzeModel, System: vizSystem,
		Prompt: "# Báo cáo\n\n" + md, Schema: vizSchema, Isolated: true,
	}, &v); err != nil {
		return nil, err
	}
	v.normalize()
	return &v, nil
}

// normalize bỏ cạnh trỏ tới nút không tồn tại, kẹp giá trị vào miền hợp lệ.
func (v *DeepViz) normalize() {
	ids := map[string]bool{}
	for _, n := range v.CausalNodes {
		ids[n.ID] = true
	}
	edges := v.CausalEdges[:0]
	for _, e := range v.CausalEdges {
		if ids[e.From] && ids[e.To] && e.From != e.To {
			e.Strength = clampInt(e.Strength, 1, 3)
			edges = append(edges, e)
		}
	}
	v.CausalEdges = edges
	for i := range v.Stakeholders {
		v.Stakeholders[i].Stance = clampInt(v.Stakeholders[i].Stance, -2, 2)
		v.Stakeholders[i].Influence = clampInt(v.Stakeholders[i].Influence, 1, 5)
	}
	for i := range v.Scenarios {
		v.Scenarios[i].Probability = clampInt(v.Scenarios[i].Probability, 0, 100)
	}
	for i := range v.VNImpacts {
		v.VNImpacts[i].Score = clampInt(v.VNImpacts[i].Score, -2, 2)
	}
}

func clampInt(x, lo, hi int) int { return max(lo, min(hi, x)) }

// VizKind: "deep:<slug>" → "deepviz:<slug>".
func VizKind(deepKind string) string { return "deepviz:" + strings.TrimPrefix(deepKind, "deep:") }

// SaveViz sinh & lưu dashboard cho 1 báo cáo deep đã có.
func SaveViz(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, date, deepKind string) (*DeepViz, error) {
	rep, err := st.GetReport(date, deepKind)
	if err != nil {
		return nil, err
	}
	if rep == nil {
		return nil, fmt.Errorf("không có báo cáo %s ngày %s", deepKind, date)
	}
	v, err := ExtractViz(ctx, cfg, r, rep.Markdown)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(v)
	return v, st.SaveReport(store.Report{Date: date, Kind: VizKind(deepKind), JSON: string(b), Model: cfg.Claude.AnalyzeModel})
}

func LoadViz(st *store.Store, date, deepKind string) (*DeepViz, error) {
	rep, err := st.GetReport(date, VizKind(deepKind))
	if err != nil || rep == nil {
		return nil, err
	}
	v := &DeepViz{}
	return v, json.Unmarshal([]byte(rep.JSON), v)
}
