package security

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// Brief: bản tổng hợp tình hình đe doạ + playbook phòng thủ (lưu reports kind=secbrief).
type Brief struct {
	Days        int          `json:"days"`
	Headline    string       `json:"headline"`
	ThreatLevel string       `json:"threat_level"` // low | guarded | elevated | high | severe
	LevelWhy    string       `json:"threat_level_why"`
	Trends      []Trend      `json:"trends"`
	Territory   string       `json:"territory"`
	Targets     string       `json:"targets"`
	Vietnam     string       `json:"vietnam"`
	Techniques  []TechNote   `json:"techniques"`
	Controls    []Control    `json:"controls"`
	PatchNow    []PatchItem  `json:"patch_now"`
	Watch       []string     `json:"watch"`
	Actors      []ActorBrief `json:"actors"`
	StackNotes  []StackNote  `json:"stack_notes"`
}

// StackNote: nguy cơ & việc cần làm cho 1 công nghệ trong stack của người đọc.
type StackNote struct {
	Stack    string   `json:"stack"`
	Exposure string   `json:"exposure"` // high | medium | low | none
	Risk     string   `json:"risk"`
	Actions  []string `json:"actions"`
}

type Trend struct {
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Direction string `json:"direction"` // rising | new | stable | falling
	Evidence  string `json:"evidence"`
}

type TechNote struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	How     string `json:"how"`
	Detect  string `json:"detect"`
	Prevent string `json:"prevent"`
}

type Control struct {
	Control  string   `json:"control"`
	Layer    string   `json:"layer"`
	Priority string   `json:"priority"` // P0 | P1 | P2
	Effort   string   `json:"effort"`   // S | M | L
	Impact   int      `json:"impact"`   // 1..5
	Counters []string `json:"counters"`
	Why      string   `json:"why"`
	Steps    []string `json:"steps"`
}

type PatchItem struct {
	CVE     string `json:"cve"`
	Product string `json:"product"`
	Why     string `json:"why"`
}

type ActorBrief struct {
	Name    string `json:"name"`
	Origin  string `json:"origin"`
	Profile string `json:"profile"`
}

func briefSchema() map[string]any {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	enum := func(v ...string) map[string]any { return map[string]any{"type": "string", "enum": v} }
	arr := func(it any) map[string]any { return map[string]any{"type": "array", "items": it} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req}
	}
	return obj(map[string]any{
		"headline":         str("1 câu kết luận lớn nhất về tình hình đe doạ"),
		"threat_level":     enum("low", "guarded", "elevated", "high", "severe"),
		"threat_level_why": str("1–2 câu vì sao mức này"),
		"trends": arr(obj(map[string]any{
			"title": str("xu hướng"), "detail": str("2–3 câu"), "direction": enum("rising", "new", "stable", "falling"),
			"evidence": str("dẫn chứng cụ thể: sự cố, số liệu, nhóm"),
		}, "title", "detail", "direction", "evidence")),
		"territory": str("2 đoạn: ai tấn công ai — nguồn gốc nhóm, quốc gia/khu vực bị nhắm, động cơ địa chính trị/tài chính"),
		"targets":   str("1–2 đoạn: ngành & loại hệ thống bị nhắm nhiều nhất và vì sao"),
		"vietnam":   str("1 đoạn: liên hệ tới Việt Nam & Đông Nam Á (nạn nhân, nhóm nhắm khu vực, sản phẩm phổ biến ở VN); nói rõ nếu dữ liệu ít"),
		"techniques": arr(obj(map[string]any{
			"id": str("ATT&CK id"), "name": str("tên"), "how": str("cách kẻ tấn công dùng trong các vụ gần đây"),
			"detect": str("cách phát hiện (log/tín hiệu cụ thể)"), "prevent": str("cách ngăn chặn cụ thể"),
		}, "id", "name", "how", "detect", "prevent")),
		"controls": arr(obj(map[string]any{
			"control": str("biện pháp, ngắn"), "layer": enum(ids(Layers)...), "priority": enum("P0", "P1", "P2"),
			"effort": enum("S", "M", "L"), "impact": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
			"counters": arr(str("technique id ATT&CK hoặc họ tấn công mà biện pháp này chặn")),
			"why":      str("vì sao — dẫn chứng từ sự cố gần đây"), "steps": arr(str("bước làm cụ thể trong dự án")),
		}, "control", "layer", "priority", "effort", "impact", "counters", "why", "steps")),
		"patch_now": arr(obj(map[string]any{"cve": str("CVE id"), "product": str("sản phẩm"), "why": str("vì sao vá ngay")}, "cve", "product", "why")),
		"watch":     arr(str("điều cần theo dõi tuần tới")),
		"stack_notes": arr(obj(map[string]any{
			"stack": str("tên công nghệ, đúng như trong danh sách stack"), "exposure": enum("high", "medium", "low", "none"),
			"risk":    str("1–2 câu: mối đe doạ cụ thể kỳ này chạm tới công nghệ này (dẫn sự cố/CVE), hoặc rủi ro nền nếu không có sự cố"),
			"actions": arr(str("việc cụ thể cho dự án dùng công nghệ này: lệnh, cấu hình, phiên bản, công cụ")),
		}, "stack", "exposure", "risk", "actions")),
		"actors": arr(obj(map[string]any{
			"name": str("nhóm tấn công nổi bật"), "origin": str("ISO alpha-2 hoặc rỗng"), "profile": str("1–2 câu: động cơ, mục tiêu, thủ đoạn đặc trưng"),
		}, "name", "origin", "profile")),
	}, "headline", "threat_level", "threat_level_why", "trends", "territory", "targets", "vietnam", "techniques", "controls", "patch_now", "watch", "actors", "stack_notes")
}

const briefSystem = `Bạn là trưởng nhóm threat intelligence kiêm kiến trúc sư bảo mật, viết bản tình hình đe doạ cho một kỹ sư phần mềm Việt Nam
đang xây các dự án (web/API, mobile backend, cloud, CI/CD, dùng nhiều thư viện mã nguồn mở, có tích hợp AI agent).
Mục tiêu: người đọc hiểu XU HƯỚNG tấn công (ai, vào đâu, bằng cách nào, vì sao) và biết NGAY cần làm gì để phòng thủ dự án của mình.
Quy tắc: chỉ dựa trên dữ liệu được cung cấp (thống kê + sự cố + KEV + leak site); nêu dẫn chứng cụ thể; phân biệt dữ kiện và suy luận;
biện pháp phải thực thi được (cấu hình, công cụ, quy trình), xếp ưu tiên theo rủi ro thực tế, không liệt kê chung chung.
Viết tiếng Việt, súc tích, giữ nguyên thuật ngữ kỹ thuật & tên riêng.`

func kvLine(kvs []KV, label func(string) string, n int) string {
	var parts []string
	for i, kv := range kvs {
		if i >= n {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", label(kv.K), kv.N))
	}
	if len(parts) == 0 {
		return "(không có)"
	}
	return strings.Join(parts, ", ")
}

func countryVI(c string) string {
	if _, ct, ok := CountryOf(c); ok {
		return ct.VI + " " + c
	}
	return c
}

// briefPrompt: dữ liệu đầu vào cho bản tổng hợp, đọc từ Board.
func briefPrompt(cfg *config.Config, b *Board) string {
	var w strings.Builder
	fmt.Fprintf(&w, "# Dữ liệu đe doạ %s → %s (%d ngày)\n\n## Người đọc\n%s\n\n", b.Since, b.Until, len(b.Days), strings.TrimSpace(cfg.Profile))
	fmt.Fprintf(&w, "## Tổng quan\n- %d bài an ninh mạng → %d sự kiện; %d sự cố/chiến dịch tấn công; %d liên quan Việt Nam\n",
		b.Articles, len(b.Incidents), b.Counts["threats"], b.Counts["vn"])
	id := func(s string) string { return s }
	fam := func(s string) string { return labelOf(Families, s) }
	fmt.Fprintf(&w, "- Họ tấn công: %s\n", kvLine(b.Tally(func(i *Incident) []string { return i.Fam }, true), fam, 8))
	fmt.Fprintf(&w, "- Ngành bị nhắm: %s\n", kvLine(b.Tally(func(i *Incident) []string { return i.Sec }, true), func(s string) string { return labelOf(Sectors, s) }, 10))
	fmt.Fprintf(&w, "- Quốc gia nạn nhân: %s\n", kvLine(b.Tally(func(i *Incident) []string { return append(append([]string{}, i.VC...), i.TC...) }, true), countryVI, 15))
	fmt.Fprintf(&w, "- Nguồn gốc tác nhân: %s\n", kvLine(b.Tally(func(i *Incident) []string { return []string{i.Origin} }, true), countryVI, 10))
	fmt.Fprintf(&w, "- Động cơ: %s\n", kvLine(b.Tally(func(i *Incident) []string { return []string{i.Mot} }, true), func(s string) string { return labelOf(Motivations, s) }, 5))
	fmt.Fprintf(&w, "- Cửa ngõ xâm nhập: %s\n", kvLine(b.Tally(func(i *Incident) []string { return i.Vec }, true), func(s string) string { return labelOf(Vectors, s) }, 10))
	fmt.Fprintf(&w, "- Nhóm tấn công: %s\n", kvLine(b.Tally(func(i *Incident) []string { return []string{i.Actor} }, false), id, 15))
	tech := b.Tally(func(i *Incident) []string { return i.Tech }, false)
	fmt.Fprintf(&w, "- Kỹ thuật ATT&CK: %s\n", kvLine(tech, func(s string) string { return s + " " + b.Tech[s].Name }, 20))
	cv := b.Tally(func(i *Incident) []string { return i.CVE }, false)
	fmt.Fprintf(&w, "- CVE được nhắc: %s\n", kvLine(cv, func(s string) string {
		m := b.CVEs[s]
		x := s + " " + m.Product
		if m.KEV != "" {
			x += " [KEV]"
		}
		if m.EPSS > 0 {
			x += fmt.Sprintf(" EPSS %.2f", m.EPSS)
		}
		return x
	}, 20))
	vend, rw := map[string]int{}, 0
	cwe := map[string]int{}
	for _, k := range b.KEV {
		vend[k.Vendor]++
		if k.Ransom {
			rw++
		}
		for _, c := range k.CWEs {
			cwe[c]++
		}
	}
	fmt.Fprintf(&w, "\n## CISA KEV trong kỳ: %d CVE mới bị khai thác thực tế (%d dùng trong ransomware)\n- Vendor: %s\n- Lớp lỗi CWE: %s\n",
		len(b.KEV), rw, kvLine(topKV(vend, 0), id, 12), kvLine(topKV(cwe, 0), func(s string) string { return s + " " + CWEVI[s] }, 10))
	for i, k := range b.KEV {
		if i >= 25 {
			break
		}
		fmt.Fprintf(&w, "  - %s %s %s (%s) %s\n", k.CVE, k.Vendor, k.Product, k.Date, k.Name)
	}
	if b.Ransom.Total > 0 {
		fmt.Fprintf(&w, "\n## Leak site ransomware: %d nạn nhân mới được đăng\n- Nhóm: %s\n- Ngành: %s\n", b.Ransom.Total,
			kvLine(b.Ransom.Groups, id, 12), kvLine(b.Ransom.Sectors, func(s string) string { return labelOf(Sectors, s) }, 10))
	}
	if len(b.Stack) > 0 {
		w.WriteString("\n## Stack công nghệ trong các dự án của người đọc (khớp từ khoá với sự kiện & KEV)\n")
		for _, h := range b.Stack {
			fmt.Fprintf(&w, "- %s: %d sự cố/chiến dịch, %d sự kiện, sev tối đa %d", h.Name, h.Incidents, h.Events, h.MaxSev)
			if len(h.CVEs) > 0 {
				fmt.Fprintf(&w, "; CVE trong tin: %s", strings.Join(h.CVEs, ", "))
			}
			if len(h.KEV) > 0 {
				fmt.Fprintf(&w, "; KEV: %s", strings.Join(h.KEV, ", "))
			}
			if len(h.Top) > 0 {
				fmt.Fprintf(&w, "; nổi bật: %s", strings.Join(h.Top, " | "))
			}
			w.WriteString("\n")
		}
	}
	// sự cố nổi bật: theo mức nghiêm trọng, số nguồn
	top := append([]*Incident(nil), b.Incidents...)
	sort.SliceStable(top, func(i, j int) bool {
		if top[i].Sev != top[j].Sev {
			return top[i].Sev > top[j].Sev
		}
		return len(top[i].Src) > len(top[j].Src)
	})
	w.WriteString("\n## Sự kiện nổi bật (mức nghiêm trọng giảm dần)\n")
	for i, inc := range top {
		if i >= 45 {
			break
		}
		fmt.Fprintf(&w, "\n### [%s] %s — %s, sev %d, %d nguồn, %s→%s\n", inc.Key, inc.Title, labelOf(Kinds, inc.Kind), inc.Sev, len(inc.Src), inc.First, inc.Last)
		if inc.Actor != "" {
			fmt.Fprintf(&w, "Tác nhân: %s (%s, %s, %s)\n", inc.Actor, countryVI(inc.Origin), labelOf(Motivations, inc.Mot), inc.Attr)
		}
		if len(inc.VC)+len(inc.TC) > 0 {
			fmt.Fprintf(&w, "Quốc gia: %s\n", strings.Join(append(append([]string{}, inc.VC...), inc.TC...), ", "))
		}
		if len(inc.Sec) > 0 {
			fmt.Fprintf(&w, "Ngành: %s · Cửa ngõ: %s\n", strings.Join(inc.Sec, ", "), strings.Join(inc.Vec, ", "))
		}
		if len(inc.Tech)+len(inc.CVE) > 0 {
			fmt.Fprintf(&w, "Kỹ thuật: %s · CVE: %s\n", strings.Join(inc.Tech, ", "), strings.Join(inc.CVE, ", "))
		}
		fmt.Fprintf(&w, "%s\n", inc.Summary)
		if inc.Detail != "" {
			fmt.Fprintf(&w, "Kỹ thuật chi tiết: %s\n", trunc(inc.Detail, 500))
		}
	}
	w.WriteString(`
## Yêu cầu
- trends: 4–6 xu hướng (rising/new trước), mỗi xu hướng có dẫn chứng.
- territory / targets / vietnam: như mô tả trường.
- techniques: 4–6 kỹ thuật ATT&CK đáng lo nhất cho dự án phần mềm (ưu tiên kỹ thuật xuất hiện nhiều + nguy hiểm cho web/API/cloud/CI/CD/supply chain/AI agent).
- controls: 8–14 biện pháp phòng thủ cho dự án, phủ nhiều tầng (layer), P0 = làm ngay tuần này; counters ghi technique id hoặc họ tấn công.
- patch_now: tối đa 8 CVE nên vá ngay (ưu tiên KEV + sản phẩm phổ biến trong stack web/cloud/dev), rỗng nếu không có.
- actors: 3–6 nhóm nổi bật trong kỳ.
- watch: 3–5 điều cần theo dõi.
- stack_notes: MỘT mục cho MỖI công nghệ trong danh sách stack (giữ đúng tên). exposure=high nếu có sự cố/CVE bị khai thác chạm trực tiếp;
  medium nếu có lỗ hổng/chiến dịch liên quan; low nếu chỉ rủi ro nền; none nếu không liên quan. actions: 2–4 việc cụ thể (phiên bản cần vá,
  cấu hình, công cụ quét, chính sách) — ưu tiên điều suy ra từ sự cố kỳ này. patch_now và controls cũng ưu tiên những gì chạm stack này.
`)
	return w.String()
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// MakeBrief dựng Board `days` ngày và nhờ Claude tổng hợp; lưu reports(date=hôm nay, kind=secbrief).
func MakeBrief(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, days int) (*Brief, error) {
	b, err := BuildBoard(cfg, st, days)
	if err != nil {
		return nil, err
	}
	if len(b.Incidents) == 0 {
		return nil, fmt.Errorf("chưa có dữ liệu an ninh mạng trong %d ngày — chạy `fa sec update` (hoặc `fa sec backfill`) trước", days)
	}
	var out Brief
	if _, err := r.JSON(ctx, llm.Request{Purpose: "secbrief", Model: cfg.Security.BriefModel, System: briefSystem,
		Prompt: briefPrompt(cfg, b), Schema: briefSchema(), Isolated: true}, &out); err != nil {
		return nil, err
	}
	out.Days = days
	for i := range out.Controls {
		out.Controls[i].Impact = max(1, min(5, out.Controls[i].Impact))
	}
	js, _ := json.Marshal(out)
	return &out, st.SaveReport(store.Report{Date: cfg.Today(), Kind: "secbrief", JSON: string(js), Model: cfg.Security.BriefModel})
}

// LoadBrief: bản tổng hợp mới nhất (tới ngày date nếu khác rỗng) — trả về cả ngày tạo.
func LoadBrief(st *store.Store, upTo string) (*Brief, string, error) {
	dates, err := st.ReportDates("secbrief", 400)
	if err != nil {
		return nil, "", err
	}
	for _, d := range dates {
		if upTo != "" && d > upTo {
			continue
		}
		rep, err := st.GetReport(d, "secbrief")
		if err != nil || rep == nil {
			return nil, "", err
		}
		var b Brief
		return &b, d, json.Unmarshal([]byte(rep.JSON), &b)
	}
	return nil, "", nil
}
