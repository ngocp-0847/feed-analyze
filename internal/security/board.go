package security

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// Board: toàn bộ dữ liệu bản đồ mối đe doạ cho 1 cửa sổ thời gian — nguồn cho dashboard (lọc chéo ở trình duyệt)
// và cho bản tổng hợp xu hướng. Tên trường JSON ngắn để trang nhẹ.
type Board struct {
	Since     string              `json:"since"`
	Until     string              `json:"until"`
	Days      []string            `json:"days"`
	Incidents []*Incident         `json:"incidents"`
	Tech      map[string]TechMeta `json:"tech"`
	CVEs      map[string]CVEMeta  `json:"cves"`
	KEV       []KEVItem           `json:"kev"`
	KEVTotal  int                 `json:"kev_total"`
	Ransom    RansomStats         `json:"ransom"`
	Countries map[string][3]any   `json:"countries"` // ISO2 → [tên VI, lon, lat]
	Labels    map[string]any      `json:"labels"`
	Counts    map[string]int      `json:"counts"`
	Articles  int                 `json:"articles"`
	Stack     []StackHit          `json:"stack"` // công nghệ của người dùng bị chạm tới
	byKey     map[string]*Incident
}

// StackHit: mức phơi nhiễm của 1 công nghệ trong stack người dùng.
type StackHit struct {
	Name      string   `json:"name"`
	Incidents int      `json:"n"`      // sự cố/chiến dịch tấn công
	Events    int      `json:"events"` // mọi sự kiện (cả lỗ hổng, cảnh báo…)
	MaxSev    int      `json:"sev"`    //
	CVEs      []string `json:"cves"`   // CVE trong tin chạm tới công nghệ này
	KEV       []string `json:"kev"`    // CVE trong KEV kỳ này chạm tới công nghệ này
	Top       []string `json:"top"`    // tiêu đề sự kiện nghiêm trọng nhất
}

type Incident struct {
	Key      string   `json:"k"`
	Title    string   `json:"t"`
	First    string   `json:"d"`
	Last     string   `json:"l"`
	Kind     string   `json:"kind"`
	Fam      []string `json:"fam"`
	Types    []string `json:"types"`
	Sev      int      `json:"sev"`
	Actor    string   `json:"actor,omitempty"`
	Aliases  []string `json:"alias,omitempty"`
	Origin   string   `json:"origin,omitempty"`
	Mot      string   `json:"mot"`
	Attr     string   `json:"attr,omitempty"`
	VC       []string `json:"vc,omitempty"` // quốc gia nạn nhân
	TC       []string `json:"tc,omitempty"` // quốc gia bị nhắm
	Sec      []string `json:"sec,omitempty"`
	Vec      []string `json:"vec,omitempty"`
	Tech     []string `json:"tech,omitempty"`
	CVE      []string `json:"cve,omitempty"`
	Prod     []string `json:"prod,omitempty"`
	Victims  []string `json:"vic,omitempty"`
	Records  int64    `json:"rec,omitempty"`
	RansomUS int64    `json:"usd,omitempty"`
	Summary  string   `json:"sum"`
	Detail   string   `json:"det,omitempty"`
	Impact   string   `json:"imp,omitempty"`
	Defenses []string `json:"def,omitempty"`
	VN       string   `json:"vn,omitempty"`
	Stk      []int    `json:"stk,omitempty"` // chỉ số trong Board.Stack
	Src      []Src    `json:"src"`
}

type Src struct {
	Title  string `json:"t"`
	URL    string `json:"u"`
	Source string `json:"s"`
	Date   string `json:"d"`
}

type TechMeta struct {
	Name   string `json:"n"`
	Tactic string `json:"tac"`
}

type CVEMeta struct {
	Product   string  `json:"p"`
	CVSS      float64 `json:"cvss,omitempty"`
	Exploited bool    `json:"x,omitempty"`
	KEV       string  `json:"kev,omitempty"` // ngày thêm vào KEV
	Ransom    bool    `json:"rw,omitempty"`
	EPSS      float64 `json:"epss,omitempty"`
	Pct       float64 `json:"pct,omitempty"`
}

type KEVItem struct {
	CVE     string   `json:"cve"`
	Vendor  string   `json:"v"`
	Product string   `json:"p"`
	Name    string   `json:"n"`
	Date    string   `json:"d"`
	Due     string   `json:"due"`
	Ransom  bool     `json:"rw,omitempty"`
	CWEs    []string `json:"cwe,omitempty"`
	EPSS    float64  `json:"epss,omitempty"`
	Pct     float64  `json:"pct,omitempty"`
	Desc    string   `json:"desc,omitempty"`
	Stk     []int    `json:"stk,omitempty"`
}

type RansomStats struct {
	Total   int            `json:"total"`
	Days    map[string]int `json:"days"`
	Groups  []KV           `json:"groups"`
	Sectors []KV           `json:"sectors"`
	Source  string         `json:"source,omitempty"`
}

type KV struct {
	K string `json:"k"`
	N int    `json:"n"`
}

var kindRank = map[string]int{"incident": 0, "campaign": 1, "vulnerability": 2, "takedown": 3, "advisory": 4, "research": 5, "policy": 6, "other": 7}

// IsThreat: sự kiện là hoạt động tấn công thực (sự cố / chiến dịch) — dùng cho bản đồ, luồng, ngành.
func (i *Incident) IsThreat() bool { return i.Kind == "incident" || i.Kind == "campaign" }

// BuildBoard gom dữ liệu `days` ngày gần nhất (tới hôm nay theo timezone cấu hình).
func BuildBoard(cfg *config.Config, st *store.Store, days int) (*Board, error) {
	now := time.Now().In(cfg.Loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cfg.Loc).AddDate(0, 0, -days+1)
	b := &Board{Since: start.Format("2006-01-02"), Until: now.Format("2006-01-02"), Tech: map[string]TechMeta{},
		CVEs: map[string]CVEMeta{}, Countries: map[string][3]any{}, byKey: map[string]*Incident{}, Counts: map[string]int{}}
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		b.Days = append(b.Days, d.Format("2006-01-02"))
	}
	evs, err := st.SecEvents(b.Since)
	if err != nil {
		return nil, err
	}
	for _, se := range evs {
		var ev Event
		if json.Unmarshal([]byte(se.Detail), &ev) != nil || ev.Kind == "other" {
			continue
		}
		b.Articles++
		b.add(se, &ev)
	}
	// CVE: KEV + EPSS
	var cves []string
	for id := range b.CVEs {
		cves = append(cves, id)
	}
	kevs, err := st.KEVSince(b.Since)
	if err != nil {
		return nil, err
	}
	b.KEVTotal = st.KEVCount()
	for _, k := range kevs {
		b.KEV = append(b.KEV, KEVItem{CVE: k.CVE, Vendor: k.Vendor, Product: k.Product, Name: k.Name, Date: k.DateAdded, Due: k.DueDate,
			Ransom: k.Ransomware == "Known", CWEs: k.CWEs, EPSS: k.EPSS, Pct: k.EPSSPct, Desc: k.Description})
	}
	inKEV, _ := st.KEVByCVE(cves)
	scores := st.EPSS(cves)
	for id, m := range b.CVEs {
		if k, ok := inKEV[id]; ok {
			m.KEV, m.Ransom = k.DateAdded, k.Ransomware == "Known"
			if m.Product == "" {
				m.Product = k.Vendor + " " + k.Product
			}
		}
		if s, ok := scores[id]; ok {
			m.EPSS, m.Pct = s[0], s[1]
		}
		b.CVEs[id] = m
	}
	// leak site ransomware
	posts, err := st.RansomPosts(start)
	if err != nil {
		return nil, err
	}
	b.Ransom = ransomStats(posts, cfg.Loc)
	if cfg.Security.RansomURL != "" {
		b.Ransom.Source = "RansomLook"
	}
	// sắp: mới nhất trước, rồi mức nghiêm trọng
	sort.SliceStable(b.Incidents, func(i, j int) bool {
		x, y := b.Incidents[i], b.Incidents[j]
		if x.Last != y.Last {
			return x.Last > y.Last
		}
		return x.Sev > y.Sev
	})
	for _, inc := range b.Incidents {
		for _, c := range append(append(append([]string{}, inc.VC...), inc.TC...), inc.Origin) {
			if code, ct, ok := CountryOf(c); ok {
				b.Countries[code] = [3]any{ct.VI, ct.Lon, ct.Lat}
			}
		}
		b.Counts["kind:"+inc.Kind]++
		if inc.IsThreat() {
			b.Counts["threats"]++
			if inc.VN != "" || contains(inc.VC, "VN") || contains(inc.TC, "VN") {
				b.Counts["vn"]++
			}
		}
	}
	b.matchStack(cfg.Security.Stack)
	b.Labels = map[string]any{
		"families": Families, "sectors": Sectors, "vectors": Vectors, "tactics": Tactics, "motivations": Motivations,
		"kinds": Kinds, "types": AttackTypeVI, "cwe": CWEVI, "layers": Layers,
	}
	return b, nil
}

func (b *Board) add(se store.SecEvent, ev *Event) {
	key := ev.IncidentKey
	if key == "" {
		key = "a" + itoa(se.ArticleID)
	}
	inc := b.byKey[key]
	if inc == nil {
		inc = &Incident{Key: key, First: se.Date, Last: se.Date, Kind: ev.Kind, Title: ev.Title, Mot: "unknown"}
		b.byKey[key] = inc
		b.Incidents = append(b.Incidents, inc)
	}
	if se.Date < inc.First {
		inc.First = se.Date
	}
	if se.Date >= inc.Last {
		inc.Last = se.Date
		inc.Summary = ev.Summary
	}
	if kindRank[ev.Kind] < kindRank[inc.Kind] {
		inc.Kind = ev.Kind
	}
	if ev.Severity > inc.Sev {
		inc.Sev, inc.Title = ev.Severity, ev.Title
	}
	if inc.Summary == "" {
		inc.Summary = ev.Summary
	}
	if len(ev.TechDetail) > len(inc.Detail) {
		inc.Detail = ev.TechDetail
	}
	if len(ev.Impact.Summary) > len(inc.Impact) {
		inc.Impact = ev.Impact.Summary
	}
	if ev.VNRelevance != "" && inc.VN == "" {
		inc.VN = ev.VNRelevance
	}
	inc.Records = max(inc.Records, ev.Impact.Records)
	inc.RansomUS = max(inc.RansomUS, ev.Impact.RansomUSD)
	if ev.Actor.Name != "" && (inc.Actor == "" || ev.Actor.Attribution == "confirmed") {
		inc.Actor, inc.Attr = ev.Actor.Name, ev.Actor.Attribution
	}
	if ev.Actor.Origin != "" && (inc.Origin == "" || ev.Actor.Attribution == "confirmed") {
		inc.Origin = ev.Actor.Origin
	}
	if ev.Actor.Motivation != "unknown" {
		inc.Mot = ev.Actor.Motivation
	}
	inc.Aliases = union(inc.Aliases, ev.Actor.Aliases, 6)
	inc.Types = union(inc.Types, ev.AttackTypes, 0)
	inc.Fam = ordered(union(inc.Fam, ev.Families(), 0), Families)
	inc.Sec = ordered(union(inc.Sec, ev.Sectors, 0), Sectors)
	inc.Vec = ordered(union(inc.Vec, ev.Vectors, 0), Vectors)
	inc.TC = union(inc.TC, ev.Targets, 0)
	for _, v := range ev.Victims {
		if v.Country != "" {
			inc.VC = union(inc.VC, []string{v.Country}, 0)
		}
		if v.Name != "" {
			inc.Victims = union(inc.Victims, []string{v.Name}, 12)
		}
	}
	inc.TC = minus(inc.TC, inc.VC)
	for _, t := range ev.Techniques {
		inc.Tech = union(inc.Tech, []string{t.ID}, 0)
		if _, ok := b.Tech[t.ID]; !ok {
			b.Tech[t.ID] = TechMeta{Name: t.Name, Tactic: t.Tactic}
		}
	}
	for _, c := range ev.CVEs {
		inc.CVE = union(inc.CVE, []string{c.ID}, 0)
		m := b.CVEs[c.ID]
		if m.Product == "" {
			m.Product = c.Product
		}
		m.CVSS = max(m.CVSS, c.CVSS)
		m.Exploited = m.Exploited || c.Exploited
		b.CVEs[c.ID] = m
	}
	inc.Prod = union(inc.Prod, ev.Products, 8)
	inc.Defenses = union(inc.Defenses, ev.Defenses, 10)
	inc.Src = append(inc.Src, Src{Title: se.Title, URL: se.Link, Source: se.Source, Date: se.Date})
}

func ransomStats(posts []store.RansomPost, loc *time.Location) RansomStats {
	rs := RansomStats{Total: len(posts), Days: map[string]int{}}
	g, s := map[string]int{}, map[string]int{}
	for _, p := range posts {
		rs.Days[p.Discovered.In(loc).Format("2006-01-02")]++
		g[p.Group]++
		s[p.Industry]++
	}
	rs.Groups = topKV(g, 15)
	rs.Sectors = topKV(s, 0)
	return rs
}

func topKV(m map[string]int, n int) []KV {
	out := make([]KV, 0, len(m))
	for k, v := range m {
		out = append(out, KV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].K < out[j].K
	})
	if n > 0 && len(out) > n {
		rest := 0
		for _, x := range out[n:] {
			rest += x.N
		}
		out = append(out[:n], KV{"(khác)", rest})
	}
	return out
}

func union(a, b []string, cap int) []string {
	for _, x := range b {
		x = strings.TrimSpace(x)
		if x == "" || containsFold(a, x) {
			continue
		}
		if cap > 0 && len(a) >= cap {
			break
		}
		a = append(a, x)
	}
	return a
}

func containsFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func minus(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// ordered sắp theo thứ tự danh mục (để màu/hàng ổn định).
func ordered(xs []string, ls []Label) []string {
	var out []string
	for _, l := range ls {
		if contains(xs, l.ID) {
			out = append(out, l.ID)
		}
	}
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ---------- thống kê cho bản tổng hợp ----------

// Tally đếm sự cố/chiến dịch theo một chiều.
func (b *Board) Tally(dim func(*Incident) []string, onlyThreats bool) []KV {
	m := map[string]int{}
	for _, inc := range b.Incidents {
		if onlyThreats && !inc.IsThreat() {
			continue
		}
		for _, k := range dim(inc) {
			if k != "" {
				m[k]++
			}
		}
	}
	return topKV(m, 0)
}

// ---------- đối chiếu stack người dùng ----------

func stackRe(terms []string) *regexp.Regexp {
	var parts []string
	for _, t := range terms {
		if t = strings.TrimSpace(strings.ToLower(t)); t != "" {
			parts = append(parts, regexp.QuoteMeta(t))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return regexp.MustCompile(`(?i)(^|[^\pL\pN])(` + strings.Join(parts, "|") + `)($|[^\pL\pN])`)
}

// matchStack đánh dấu sự cố & KEV chạm tới từng công nghệ trong stack (khớp theo từ trong tiêu đề, tóm tắt,
// diễn biến kỹ thuật, sản phẩm, CVE).
func (b *Board) matchStack(items []config.StackItem) {
	b.Stack = nil
	for si, it := range items {
		terms := it.Match
		if len(terms) == 0 {
			terms = []string{it.Name}
		}
		re := stackRe(terms)
		if re == nil {
			continue
		}
		h := StackHit{Name: it.Name}
		type top struct {
			sev   int
			title string
		}
		var tops []top
		for _, inc := range b.Incidents {
			var cveText []string
			for _, c := range inc.CVE {
				cveText = append(cveText, b.CVEs[c].Product)
			}
			text := strings.Join([]string{inc.Title, inc.Summary, inc.Detail, strings.Join(inc.Prod, " ; "), strings.Join(cveText, " ; "),
				strings.Join(inc.Types, " ")}, " \n ")
			if !re.MatchString(text) {
				continue
			}
			inc.Stk = append(inc.Stk, si)
			h.Events++
			if inc.IsThreat() {
				h.Incidents++
			}
			h.MaxSev = max(h.MaxSev, inc.Sev)
			tops = append(tops, top{inc.Sev, inc.Title})
			for _, c := range inc.CVE {
				if re.MatchString(b.CVEs[c].Product) || re.MatchString(inc.Title) {
					h.CVEs = union(h.CVEs, []string{c}, 0)
				}
			}
		}
		for i := range b.KEV {
			k := &b.KEV[i]
			if re.MatchString(k.Vendor + " " + k.Product + " ; " + k.Name) {
				k.Stk = append(k.Stk, si)
				h.KEV = append(h.KEV, k.CVE)
			}
		}
		sort.SliceStable(tops, func(i, j int) bool { return tops[i].sev > tops[j].sev })
		for i, t := range tops {
			if i >= 4 {
				break
			}
			h.Top = append(h.Top, t.title)
		}
		b.Stack = append(b.Stack, h)
	}
}
