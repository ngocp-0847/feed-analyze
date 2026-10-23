package security

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// Event: dữ liệu tình báo rút từ 1 bài (khớp JSON schema bên dưới; lưu nguyên trong sec_events.detail).
type Event struct {
	ID          int64       `json:"id"`
	Kind        string      `json:"kind"`
	IncidentKey string      `json:"incident_key"`
	Title       string      `json:"title"`
	Summary     string      `json:"summary_vi"`
	AttackTypes []string    `json:"attack_types"`
	Severity    int         `json:"severity"`
	Actor       Actor       `json:"actor"`
	Victims     []Victim    `json:"victims"`
	Targets     []string    `json:"target_countries"`
	Sectors     []string    `json:"sectors"`
	Vectors     []string    `json:"vectors"`
	Techniques  []Technique `json:"techniques"`
	CVEs        []CVERef    `json:"cves"`
	Products    []string    `json:"products"`
	Impact      Impact      `json:"impact"`
	TechDetail  string      `json:"tech_detail"`
	Defenses    []string    `json:"defenses"`
	VNRelevance string      `json:"vn_relevance"`
}

type Actor struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Origin      string   `json:"origin"`
	Motivation  string   `json:"motivation"`
	Attribution string   `json:"attribution"` // confirmed | suspected | unknown
}

type Victim struct {
	Name    string `json:"name"`
	Country string `json:"country"`
	Sector  string `json:"sector"`
}

type Technique struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tactic string `json:"tactic"`
}

type CVERef struct {
	ID        string  `json:"id"`
	Product   string  `json:"product"`
	CVSS      float64 `json:"cvss"`
	Exploited bool    `json:"exploited"`
	Patched   bool    `json:"patched"`
}

type Impact struct {
	Records   int64  `json:"records"`
	RansomUSD int64  `json:"ransom_usd"`
	Summary   string `json:"summary"`
}

func sch() map[string]any {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	enum := func(v []string) map[string]any { return map[string]any{"type": "string", "enum": v} }
	arr := func(it any) map[string]any { return map[string]any{"type": "array", "items": it} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req}
	}
	var types []string
	for k := range AttackTypes {
		types = append(types, k)
	}
	sort.Strings(types)
	item := obj(map[string]any{
		"id":           map[string]any{"type": "integer"},
		"kind":         enum(ids(Kinds)),
		"incident_key": str("kebab-case tiếng Anh không dấu, ổn định: cùng sự cố/chiến dịch/lỗ hổng → cùng key (dùng lại key đã có nếu khớp); rỗng nếu kind=policy/other"),
		"title":        str("tiêu đề tiếng Việt ngắn (<= 14 từ) nêu ai/cái gì bị tấn công hoặc lỗ hổng gì"),
		"summary_vi":   str("2–3 câu tiếng Việt: chuyện gì xảy ra, quy mô, số liệu"),
		"attack_types": arr(enum(types)),
		"severity":     map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
		"actor": obj(map[string]any{
			"name":        str("tên nhóm/tác nhân (vd Lazarus, Scattered Spider, LockBit); rỗng nếu không rõ"),
			"aliases":     arr(map[string]any{"type": "string"}),
			"origin":      str("mã ISO 3166-1 alpha-2 quốc gia gốc/được quy cho; rỗng nếu không rõ"),
			"motivation":  enum(ids(Motivations)),
			"attribution": enum([]string{"confirmed", "suspected", "unknown"}),
		}, "name", "origin", "motivation", "attribution"),
		"victims": arr(obj(map[string]any{
			"name": str("tổ chức nạn nhân (rỗng nếu không nêu)"), "country": str("ISO alpha-2"), "sector": enum(ids(Sectors)),
		}, "name", "country", "sector")),
		"target_countries": arr(str("ISO alpha-2 các nước bị nhắm (chiến dịch diện rộng)")),
		"sectors":          arr(enum(ids(Sectors))),
		"vectors":          arr(enum(ids(Vectors))),
		"techniques": arr(obj(map[string]any{
			"id": str("MITRE ATT&CK technique id, vd T1566.001, T1190, T1078"), "name": str("tên technique tiếng Anh"), "tactic": enum(ids(Tactics)),
		}, "id", "name", "tactic")),
		"cves": arr(obj(map[string]any{
			"id": str("CVE-YYYY-NNNN"), "product": str("vendor + sản phẩm"), "cvss": map[string]any{"type": "number", "minimum": 0, "maximum": 10},
			"exploited": map[string]any{"type": "boolean"}, "patched": map[string]any{"type": "boolean"},
		}, "id", "product", "cvss", "exploited", "patched")),
		"products": arr(str("vendor/sản phẩm/thư viện bị ảnh hưởng")),
		"impact": obj(map[string]any{
			"records":    map[string]any{"type": "integer", "description": "số bản ghi/người bị lộ, 0 nếu không rõ"},
			"ransom_usd": map[string]any{"type": "integer", "description": "tiền chuộc/thiệt hại USD, 0 nếu không rõ"},
			"summary":    str("tác động ngắn: dừng dịch vụ, lộ dữ liệu gì…"),
		}, "records", "ransom_usd", "summary"),
		"tech_detail":  str("3–6 câu tiếng Việt, đặc tả KỸ THUẬT: chuỗi tấn công từng bước, lỗ hổng/cơ chế bị lợi dụng, công cụ/malware, cách duy trì & tuồn dữ liệu, dấu hiệu nhận biết"),
		"defenses":     arr(str("biện pháp phòng/phát hiện cụ thể, làm được cho đội phát triển & vận hành dự án phần mềm")),
		"vn_relevance": str("liên hệ tới Việt Nam (nạn nhân VN, sản phẩm phổ biến ở VN, nhóm nhắm Đông Nam Á…); rỗng nếu không có"),
	}, "id", "kind", "incident_key", "title", "summary_vi", "attack_types", "severity", "actor", "victims", "target_countries",
		"sectors", "vectors", "techniques", "cves", "products", "impact", "tech_detail", "defenses", "vn_relevance")
	return obj(map[string]any{"items": arr(item)}, "items")
}

const extractSystem = `Bạn là chuyên gia threat intelligence (CTI). Từ bài báo an ninh mạng (tiêu đề + nội dung), trích dữ liệu có cấu trúc để dựng bản đồ mối đe doạ.
Quy tắc:
- CHỈ dựa vào nội dung bài. Không bịa tên nhóm, quốc gia, CVE, số liệu. Không rõ → để rỗng / 0 / unknown.
- kind: incident = vụ tấn công/sự cố cụ thể đã xảy ra; campaign = hoạt động của nhóm/chiến dịch/malware đang diễn ra (báo cáo threat research về nhóm cũng là campaign);
  vulnerability = lỗ hổng/CVE/bản vá; advisory = cảnh báo của cơ quan/nhà cung cấp; takedown = bắt giữ/triệt phá/truy tố;
  research = kỹ thuật tấn công mới, báo cáo xu hướng; policy = luật/chính sách/thị trường; other = không phải tin an ninh thực chất (quảng cáo, podcast...).
- incident_key: nhiều bài cùng 1 sự cố/chiến dịch/lỗ hổng phải cùng key. Nếu khớp danh sách "sự cố đã biết" → dùng lại đúng key đó.
  Ví dụ: "salesforce-salesloft-drift-oauth-breach", "cve-2025-53770-sharepoint-toolshell", "lazarus-npm-contagious-interview".
- Quốc gia: mã ISO 3166-1 alpha-2 in hoa (US, CN, RU, KP, IR, VN, GB...). actor.origin = quốc gia được quy trách nhiệm/gốc của nhóm (theo bài hoặc kiến thức CTI phổ biến về nhóm đó nếu bài nêu tên nhóm; attribution=suspected nếu chỉ là nghi vấn).
- victims: nạn nhân cụ thể (tổ chức), kèm quốc gia & ngành. target_countries: nước bị nhắm khi chiến dịch diện rộng (không lặp nạn nhân).
- techniques: ánh xạ hành vi trong bài sang MITRE ATT&CK Enterprise (id chuẩn, tactic tương ứng), 2–10 technique, chỉ khi bài mô tả hành vi.
- severity 1–5: 5 = ảnh hưởng hệ thống/quốc gia hoặc lỗ hổng bị khai thác diện rộng sản phẩm phổ biến; 4 = tổ chức lớn/dữ liệu triệu người/CVE critical bị khai thác; 3 = đáng chú ý; 2 = nhỏ; 1 = nhiễu.
- tech_detail & defenses viết cho kỹ sư phần mềm: cụ thể (vd "ghim phiên bản + bật npm provenance", "chặn token OAuth của app bên thứ ba không dùng", "vá X lên bản Y", "bật phishing-resistant MFA (passkey/FIDO2) cho admin").
- Tiếng Việt cho mọi trường văn bản (giữ nguyên tên riêng, tên kỹ thuật). Trả về đủ mọi id được giao.`

var cveRe = regexp.MustCompile(`^CVE-\d{4}-\d{4,7}$`)
var techRe = regexp.MustCompile(`^T\d{4}(\.\d{3})?$`)

// ExtractOptions điều khiển 1 lượt trích xuất.
type ExtractOptions struct {
	Since time.Time // chỉ bài đăng từ thời điểm này
	Limit int
	Log   func(format string, args ...any)
}

type ExtractResult struct{ Done, Total, Incidents, Failed int }

// Extract: trích xuất tình báo cho bài topic security chưa xử lý.
func Extract(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, o ExtractOptions) (ExtractResult, error) {
	logf := o.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if o.Limit <= 0 {
		o.Limit = 1000
	}
	var res ExtractResult
	arts, err := st.SecPending(cfg.Security.Topic, cfg.Analyze.MinRelevance, o.Since, o.Limit)
	if err != nil || len(arts) == 0 {
		return res, err
	}
	res.Total = len(arts)
	var texts map[int64]string
	if cfg.Security.FullText {
		logf("  … tải nội dung %d bài gốc", len(arts))
		texts = EnsureTexts(ctx, st, arts, cfg.Security.TextChars)
		logf("  ✓ có nội dung đầy đủ %d/%d bài (còn lại dùng tóm tắt RSS)", len(texts), len(arts))
	}
	var batches [][]store.ArticleView
	for i := 0; i < len(arts); i += cfg.Security.BatchSize {
		batches = append(batches, arts[i:min(i+cfg.Security.BatchSize, len(arts))])
	}
	schema := sch()
	sem := make(chan struct{}, cfg.Claude.Concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	for _, b := range batches {
		wg.Add(1)
		go func(b []store.ArticleView) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			n, inc, err := extractBatch(ctx, cfg, st, r, b, texts, schema)
			mu.Lock()
			defer mu.Unlock()
			res.Done += n
			res.Incidents += inc
			if err != nil {
				res.Failed++
				if firstErr == nil {
					firstErr = err
				}
				logf("  ✗ batch lỗi: %v", err)
			}
			logf("  … %d/%d bài", res.Done, res.Total)
		}(b)
	}
	wg.Wait()
	return res, firstErr
}

func extractBatch(ctx context.Context, cfg *config.Config, st *store.Store, r *llm.Runner, batch []store.ArticleView,
	texts map[int64]string, schema map[string]any) (int, int, error) {
	var b strings.Builder
	known, _ := st.RecentIncidents(time.Now().In(cfg.Loc).AddDate(0, 0, -21).Format("2006-01-02"), 150)
	b.WriteString("## Sự cố đã biết (dùng lại incident_key nếu bài nói về cùng sự việc)\n")
	if len(known) == 0 {
		b.WriteString("(chưa có)\n")
	}
	for _, k := range known {
		fmt.Fprintf(&b, "- %s — %s (%s, %d bài)\n", k.Key, k.Title, k.LastDate, k.Articles)
	}
	b.WriteString("\n## Bài cần trích xuất\n")
	for _, a := range batch {
		fmt.Fprintf(&b, "\n### [%d] %s\nNguồn: %s · %s\n", a.ID, a.Title, a.Source, a.Published.In(cfg.Loc).Format("2006-01-02"))
		if t := texts[a.ID]; t != "" {
			b.WriteString(t)
		} else {
			b.WriteString(a.Summary)
		}
		b.WriteString("\n")
	}
	var out struct {
		Items []Event `json:"items"`
	}
	if _, err := r.JSON(ctx, llm.Request{Purpose: "secintel", Model: cfg.Security.ExtractModel, System: extractSystem,
		Prompt: b.String(), Schema: schema, Isolated: true}, &out); err != nil {
		return 0, 0, err
	}
	byID := map[int64]store.ArticleView{}
	for _, a := range batch {
		byID[a.ID] = a
	}
	done, inc := 0, 0
	for _, ev := range out.Items {
		a, ok := byID[ev.ID]
		if !ok {
			continue
		}
		delete(byID, ev.ID)
		ev.normalize()
		if ev.Kind == "incident" || ev.Kind == "campaign" {
			inc++
		}
		detail, _ := json.Marshal(ev)
		if err := st.SaveSecEvent(store.SecEvent{ArticleID: a.ID, Key: ev.IncidentKey, Kind: ev.Kind,
			Date: a.Published.In(cfg.Loc).Format("2006-01-02"), Detail: string(detail)}, cfg.Security.ExtractModel); err != nil {
			return done, inc, err
		}
		done++
	}
	return done, inc, nil
}

func inSet(v string, ls []Label) bool {
	for _, l := range ls {
		if l.ID == v {
			return true
		}
	}
	return false
}

// normalize: loại giá trị ngoài danh mục, chuẩn hoá mã quốc gia / CVE / technique, kẹp severity.
func (e *Event) normalize() {
	if !inSet(e.Kind, Kinds) {
		e.Kind = "other"
	}
	e.IncidentKey = analyze.Slugify(e.IncidentKey)
	e.Severity = max(1, min(5, e.Severity))
	var types []string
	for _, t := range e.AttackTypes {
		if _, ok := AttackTypes[t]; ok && !contains(types, t) {
			types = append(types, t)
		}
	}
	e.AttackTypes = types
	if c, _, ok := CountryOf(e.Actor.Origin); ok {
		e.Actor.Origin = c
	} else {
		e.Actor.Origin = ""
	}
	if !inSet(e.Actor.Motivation, Motivations) {
		e.Actor.Motivation = "unknown"
	}
	e.Actor.Name = strings.TrimSpace(e.Actor.Name)
	if l := strings.ToLower(e.Actor.Name); l == "unknown" || l == "không rõ" || l == "n/a" {
		e.Actor.Name = ""
	}
	vs := e.Victims[:0]
	for _, v := range e.Victims {
		if c, _, ok := CountryOf(v.Country); ok {
			v.Country = c
		} else {
			v.Country = ""
		}
		if !inSet(v.Sector, Sectors) {
			v.Sector = "other"
		}
		vs = append(vs, v)
	}
	e.Victims = vs
	var tc []string
	for _, c := range e.Targets {
		if code, _, ok := CountryOf(c); ok && !contains(tc, code) {
			tc = append(tc, code)
		}
	}
	e.Targets = tc
	e.Sectors = filterSet(e.Sectors, Sectors)
	for _, v := range e.Victims {
		if v.Sector != "other" && !contains(e.Sectors, v.Sector) {
			e.Sectors = append(e.Sectors, v.Sector)
		}
	}
	e.Vectors = filterSet(e.Vectors, Vectors)
	ts := e.Techniques[:0]
	seen := map[string]bool{}
	for _, t := range e.Techniques {
		t.ID = strings.ToUpper(strings.TrimSpace(t.ID))
		if techRe.MatchString(t.ID) && inSet(t.Tactic, Tactics) && !seen[t.ID] {
			seen[t.ID] = true
			ts = append(ts, t)
		}
	}
	e.Techniques = ts
	cs := e.CVEs[:0]
	seenC := map[string]bool{}
	for _, c := range e.CVEs {
		c.ID = strings.ToUpper(strings.TrimSpace(c.ID))
		if cveRe.MatchString(c.ID) && !seenC[c.ID] {
			seenC[c.ID] = true
			c.CVSS = max(0, min(10, c.CVSS))
			cs = append(cs, c)
		}
	}
	e.CVEs = cs
	e.Impact.Records = max(0, e.Impact.Records)
	e.Impact.RansomUSD = max(0, e.Impact.RansomUSD)
}

func filterSet(vs []string, ls []Label) []string {
	var out []string
	for _, v := range vs {
		if inSet(v, ls) && !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Families của 1 sự kiện (theo thứ tự cố định).
func (e *Event) Families() []string {
	set := map[string]bool{}
	for _, t := range e.AttackTypes {
		set[AttackTypes[t]] = true
	}
	var out []string
	for _, f := range Families {
		if set[f.ID] {
			out = append(out, f.ID)
		}
	}
	return out
}
