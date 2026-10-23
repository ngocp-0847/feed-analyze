package web

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ngocp-0847/feed-analyze/internal/security"
)

// ---------- bản đồ mối đe doạ an ninh mạng ----------

var threatLevels = map[string][2]string{
	"low": {"good", "Thấp"}, "guarded": {"neutral", "Cảnh giác"}, "elevated": {"warning", "Tăng cao"},
	"high": {"serious", "Cao"}, "severe": {"critical", "Nghiêm trọng"},
}

var trendDir = map[string][2]string{
	"rising": {"serious", "tăng"}, "new": {"warning", "mới"}, "stable": {"neutral", "ổn định"}, "falling": {"good", "giảm"},
}

// secVisible: trang /security có hiển thị không — luôn có ở bản cục bộ, bản công khai chỉ khi publish.include_security.
func (s *Server) secVisible() bool {
	return s.cfg.SecurityOn() && (!s.public() || s.cfg.Publish.IncludeSecurity)
}

func (s *Server) security(w http.ResponseWriter, r *http.Request) {
	if s.public() && !s.secVisible() {
		http.NotFound(w, r)
		return
	}
	days := atoi(r.PathValue("days"), atoi(r.URL.Query().Get("days"), 30))
	days = min(days, 365)
	var b strings.Builder
	b.WriteString(`<div class="sec">`)
	b.WriteString(`<h1>🛡️ Bản đồ mối đe doạ an ninh mạng</h1>`)
	b.WriteString(`<div class="filters">`)
	for _, n := range []int{7, 30, 90} {
		on := ""
		if n == days {
			on = " on"
		}
		fmt.Fprintf(&b, `<a class="btn%s" href="%s">%d ngày</a>`, on, s.link(fmt.Sprintf("/security/%d", n)), n)
	}
	b.WriteString(`</div>`)
	if !s.cfg.SecurityOn() {
		b.WriteString(`<div class="empty">Chưa bật chủ đề an ninh mạng. Thêm topic <code>security</code> và mục <code>security:</code> vào config.yaml (xem config mặc định), rồi chạy <code>fa sec backfill --open</code>.</div></div>`)
		s.render(w, r, "security", "An ninh mạng", b.String())
		return
	}
	board, err := security.BuildBoard(s.cfg, s.st, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	brief, bdate, _ := security.LoadBrief(s.st, "")
	if len(board.Incidents) == 0 && len(board.KEV) == 0 {
		b.WriteString(`<div class="empty">Chưa có dữ liệu an ninh mạng trong khoảng này.`)
		if !s.public() {
			b.WriteString(` Chạy <code>fa sec backfill --open</code> để quét nguồn tin security 30 ngày, trích xuất sự cố/kỹ thuật/CVE và dựng bản đồ.`)
		}
		b.WriteString(`</div></div>`)
		s.render(w, r, "security", "An ninh mạng", b.String())
		return
	}

	// mục lục: tổng quan → chi tiết
	b.WriteString(`<nav class="toc">`)
	for _, x := range [][2]string{{"overview", "Tổng quan"}, {"territory", "Lãnh thổ"}, {"flow", "Ai → cách nào → vào đâu"},
		{"trend", "Xu hướng"}, {"targets", "Mục tiêu"}, {"actors", "Nhóm tấn công"}, {"attack", "Kỹ thuật ATT&CK"},
		{"vulns", "Lỗ hổng"}, {"stack", "Stack của bạn"}, {"defense", "Phòng thủ"}, {"explorer", "Chi tiết sự cố"}} {
		fmt.Fprintf(&b, `<a href="#%s">%s</a>`, x[0], x[1])
	}
	b.WriteString(`</nav>`)

	// ---- tổng quan
	b.WriteString(`<section id="overview" class="sec-block">`)
	if brief != nil {
		lv := threatLevels[brief.ThreatLevel]
		if lv[0] == "" {
			lv = [2]string{"neutral", brief.ThreatLevel}
		}
		fmt.Fprintf(&b, `<div class="threat-hero %s"><div class="lvl"><small>MỨC ĐE DOẠ</small>%s</div><div class="txt"><b>%s</b><p>%s</p><small>Tổng hợp %s · %d ngày · %s</small></div></div>`,
			lv[0], statusHTML(lv[0], lv[1]), esc(brief.Headline), esc(brief.LevelWhy), esc(bdate), brief.Days, esc(s.cfg.Security.BriefModel))
	} else if !s.public() {
		b.WriteString(`<div class="empty">Chưa có bản tổng hợp xu hướng + playbook phòng thủ. Tạo bằng <code>fa sec brief</code>.</div>`)
	}
	b.WriteString(`<div class="secbar" id="sec-filterbar"><span class="lab">Phạm vi:</span>
<label><input type="radio" name="scope" value="threats" checked> Sự cố & chiến dịch tấn công</label>
<label><input type="radio" name="scope" value="all"> Mọi tin (cả lỗ hổng, cảnh báo, nghiên cứu)</label>
<label><input type="checkbox" name="vn"> Chỉ liên quan Việt Nam</label>
<input type="search" name="q" placeholder="Tìm nhóm, sản phẩm, CVE, nạn nhân…">
<span class="chips" id="sec-chips"></span></div>
<p class="hint">Bấm vào quốc gia, nhóm, ngành, kỹ thuật… ở bất kỳ biểu đồ nào để <b>lọc toàn trang</b> — mọi biểu đồ và danh sách sự cố bên dưới cập nhật theo.</p>`)
	b.WriteString(`<div class="kpis" id="sec-kpis"></div>`)
	if brief != nil && len(brief.Trends) > 0 {
		b.WriteString(`<div class="trends">`)
		for _, t := range brief.Trends {
			d := trendDir[t.Direction]
			if d[0] == "" {
				d = [2]string{"neutral", t.Direction}
			}
			fmt.Fprintf(&b, `<div class="trend"><div class="h">%s <b>%s</b></div><p>%s</p><details><summary>Dẫn chứng</summary><small>%s</small></details></div>`,
				statusHTML(d[0], d[1]), esc(t.Title), esc(t.Detail), esc(t.Evidence))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)

	// ---- lãnh thổ
	b.WriteString(`<section id="territory" class="sec-block"><h2>🌍 Lãnh thổ: ai tấn công ai</h2>`)
	if brief != nil && brief.Territory != "" {
		fmt.Fprintf(&b, `<div class="prose lead">%s</div>`, s.markdown(brief.Territory))
	}
	b.WriteString(`<div class="dash">
<section class="panel"><div class="ph"><h3>Bản đồ tấn công toàn cầu</h3><div class="seg" id="map-mode"><button data-m="victim" class="on">Nạn nhân</button><button data-m="origin">Nguồn tấn công</button></div></div>
<p class="sub">Màu = số sự cố/chiến dịch · đường cong = luồng từ quốc gia của nhóm tấn công tới nước nạn nhân (khi được quy kết) · cuộn để zoom, bấm 1 nước để lọc</p>
<div class="legend" id="map-legend"></div><div class="chart" id="sec-map" style="height:520px" role="img" aria-label="Bản đồ tấn công toàn cầu"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-map-tbl"></div></details></section>
<section class="panel half"><h3>Nước bị tấn công nhiều nhất</h3><p class="sub">Nạn nhân cụ thể + nước bị nhắm trong chiến dịch</p><div class="bars" id="sec-victims"></div></section>
<section class="panel half"><h3>Nguồn gốc nhóm tấn công</h3><p class="sub">Theo quy kết trong tin (xác nhận hoặc nghi vấn); phần lớn sự cố chưa được quy kết</p><div class="bars" id="sec-origins"></div></section>
</div></section>`)

	// ---- luồng
	b.WriteString(`<section id="flow" class="sec-block"><h2>🔀 Ai → bằng cách nào → vào đâu</h2><div class="dash">
<section class="panel"><h3>Dòng chảy tấn công</h3><p class="sub">Nguồn tấn công (động cơ) → hình thức tấn công → ngành bị nhắm · độ dày = số sự cố · bấm nút để lọc</p>
<div class="chart" id="sec-sankey" style="height:520px" role="img" aria-label="Sơ đồ Sankey tác nhân, hình thức, ngành"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-sankey-tbl"></div></details></section>
</div></section>`)

	// ---- xu hướng
	b.WriteString(`<section id="trend" class="sec-block"><h2>📈 Xu hướng theo thời gian</h2><div class="dash">
<section class="panel"><h3>Sự cố & chiến dịch theo ngày, theo hình thức</h3><p class="sub">Đếm theo ngày xuất hiện đầu tiên trong tin · rê chuột để xem từng loại</p>
<div class="chart" id="sec-trend" style="height:300px" role="img" aria-label="Xu hướng sự cố theo ngày"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-trend-tbl"></div></details></section>
<section class="panel half"><h3>Nạn nhân mới trên leak site ransomware</h3><p class="sub">Số tổ chức bị nhóm ransomware đăng tên mỗi ngày (RansomLook) — chỉ báo sớm, độc lập với tin báo chí</p>
<div class="chart" id="sec-leak" style="height:260px" role="img" aria-label="Nạn nhân leak site theo ngày"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-leak-tbl"></div></details></section>
<section class="panel half"><h3>Nhóm ransomware hoạt động mạnh nhất</h3><p class="sub">Số nạn nhân được đăng trong kỳ (leak site)</p>
<div class="chart" id="sec-groups" style="height:260px" role="img" aria-label="Nhóm ransomware hoạt động mạnh nhất"></div>
<p class="sub" id="sec-groups-note"></p><details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-groups-tbl"></div></details></section>
</div></section>`)

	// ---- mục tiêu
	b.WriteString(`<section id="targets" class="sec-block"><h2>🎯 Mục tiêu: ngành & cửa ngõ xâm nhập</h2>`)
	if brief != nil && brief.Targets != "" {
		fmt.Fprintf(&b, `<div class="prose lead">%s</div>`, s.markdown(brief.Targets))
	}
	b.WriteString(`<div class="dash">
<section class="panel"><h3>Ngành × hình thức tấn công</h3><p class="sub">Ô đậm = nhiều sự cố · bấm ô để lọc theo ngành và hình thức</p>
<div class="chart" id="sec-heat" style="height:460px" role="img" aria-label="Ma trận ngành và hình thức tấn công"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-heat-tbl"></div></details></section>
<section class="panel half"><h3>Cửa ngõ xâm nhập ban đầu</h3><p class="sub">Kẻ tấn công vào bằng đường nào — mỗi cửa ngõ ứng với một lớp phòng thủ</p>
<div class="chart" id="sec-vectors" style="height:340px" role="img" aria-label="Cửa ngõ xâm nhập"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-vectors-tbl"></div></details></section>
<section class="panel half"><h3>Ngành bị leak site ransomware đăng tên</h3><p class="sub">Phân bố ngành của nạn nhân ransomware (ánh xạ từ mô tả ngành trên leak site)</p>
<div class="chart" id="sec-leaksec" style="height:340px" role="img" aria-label="Ngành nạn nhân ransomware"></div>
<p class="sub" id="sec-leaksec-note"></p><details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-leaksec-tbl"></div></details></section>
</div>`)
	if brief != nil && brief.Vietnam != "" {
		fmt.Fprintf(&b, `<div class="vnbox"><b>🇻🇳 Việt Nam & Đông Nam Á</b>%s</div>`, s.markdown(brief.Vietnam))
	}
	b.WriteString(`</section>`)

	// ---- nhóm tấn công
	b.WriteString(`<section id="actors" class="sec-block"><h2>🕵️ Nhóm tấn công</h2>`)
	if brief != nil && len(brief.Actors) > 0 {
		b.WriteString(`<div class="cards">`)
		for _, a := range brief.Actors {
			origin := ""
			if code, c, ok := security.CountryOf(a.Origin); ok {
				origin = flag(code) + " " + c.VI
			}
			fmt.Fprintf(&b, `<div class="card actor" data-actor="%s"><span class="d">%s</span><b>%s</b><span>%s</span></div>`,
				esc(a.Name), esc(origin), esc(a.Name), esc(a.Profile))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div class="dash"><section class="panel"><h3>Hồ sơ nhóm trong kỳ</h3><p class="sub">Tổng hợp từ các sự cố được quy cho từng nhóm · bấm tên để lọc</p><div id="sec-actors"></div></section></div></section>`)

	// ---- ATT&CK
	b.WriteString(`<section id="attack" class="sec-block"><h2>🧬 Kỹ thuật tấn công (MITRE ATT&CK)</h2>
<div class="dash"><section class="panel"><h3>Ma trận ATT&CK của các vụ gần đây</h3><p class="sub">14 giai đoạn kill chain từ trái sang phải · ô đậm = kỹ thuật xuất hiện trong nhiều sự cố · bấm để lọc, xem chi tiết kỹ thuật bên dưới</p>
<div class="attack" id="sec-attack"></div></section></div>`)
	if brief != nil && len(brief.Techniques) > 0 {
		b.WriteString(`<div class="technotes">`)
		for _, t := range brief.Techniques {
			fmt.Fprintf(&b, `<div class="technote"><div class="h"><code>%s</code> <b>%s</b></div><dl><dt>Cách bị lợi dụng</dt><dd>%s</dd><dt>🔎 Phát hiện</dt><dd>%s</dd><dt>🧱 Ngăn chặn</dt><dd>%s</dd></dl></div>`,
				esc(t.ID), esc(t.Name), esc(t.How), esc(t.Detect), esc(t.Prevent))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)

	// ---- lỗ hổng
	b.WriteString(`<section id="vulns" class="sec-block"><h2>🩹 Lỗ hổng bị khai thác</h2><div class="dash">`)
	if brief != nil && len(brief.PatchNow) > 0 {
		var h strings.Builder
		h.WriteString(`<table><tr><th>CVE</th><th>Sản phẩm</th><th>Vì sao vá ngay</th></tr>`)
		for _, p := range brief.PatchNow {
			fmt.Fprintf(&h, `<tr><td class="nw"><a href="https://nvd.nist.gov/vuln/detail/%s" rel="noopener" target="_blank"><code>%s</code></a></td><td>%s</td><td>%s</td></tr>`, esc(p.CVE), esc(p.CVE), esc(p.Product), esc(p.Why))
		}
		h.WriteString(`</table>`)
		panel(&b, panelOpt{Title: "⚠️ Nên vá ngay", Sub: "Claude chọn từ KEV + CVE trong tin, ưu tiên sản phẩm phổ biến trong stack web/cloud/dev", HTML: h.String()})
	}
	fmt.Fprintf(&b, `<section class="panel"><h3>CVE trong tin tức kỳ này</h3><p class="sub">Xếp theo: đã bị khai thác thực tế (CISA KEV) → xác suất bị khai thác 30 ngày tới (EPSS) → điểm CVSS</p><div id="sec-cves"></div></section>
<section class="panel half"><h3>CISA KEV: vendor có lỗ hổng bị khai thác</h3><p class="sub">%d CVE được thêm vào KEV trong kỳ (tổng danh mục %d)</p>
<div class="legend" id="sec-kev-legend"></div><div class="chart" id="sec-kev" style="height:360px" role="img" aria-label="Vendor trong KEV"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-kev-tbl"></div></details></section>
<section class="panel half"><h3>Lớp lỗi (CWE) bị khai thác nhiều nhất</h3><p class="sub">Loại lỗi lập trình gốc của các CVE trong KEV kỳ này — thứ cần phòng ngay từ khi viết code</p>
<div class="chart" id="sec-cwe" style="height:360px" role="img" aria-label="CWE trong KEV"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-cwe-tbl"></div></details></section>
<section class="panel"><details><summary><b>Danh sách KEV trong kỳ</b> — hạn vá của CISA, mô tả, EPSS</summary><div id="sec-kevlist"></div></details></section>
</div></section>`, len(board.KEV), board.KEVTotal)

	// ---- stack người dùng
	if len(board.Stack) > 0 {
		b.WriteString(`<section id="stack" class="sec-block"><h2>🧩 Stack của bạn bị chạm tới đâu</h2>
<p class="sub">Đối chiếu công nghệ trong <code>security.stack</code> (config) với sự kiện & CVE kỳ này · mức phơi nhiễm và việc cần làm do Claude đánh giá · bấm thẻ để lọc toàn trang</p>
<div class="stackgrid" id="sec-stack"></div></section>`)
	}

	// ---- phòng thủ
	b.WriteString(`<section id="defense" class="sec-block"><h2>🛡️ Phòng thủ cho dự án của bạn</h2>`)
	if brief != nil && len(brief.Controls) > 0 {
		b.WriteString(`<div class="dash"><section class="panel half"><h3>Ma trận ưu tiên</h3><p class="sub">Tác động giảm rủi ro (dọc) × công sức (ngang) · góc trên-trái = làm trước</p>
<div class="legend" id="sec-prio-legend"></div><div class="chart" id="sec-prio" style="height:380px" role="img" aria-label="Ma trận ưu tiên biện pháp"></div>
<details class="tbl"><summary>Bảng dữ liệu</summary><div class="body" id="sec-prio-tbl"></div></details></section>
<section class="panel half"><h3>Độ phủ theo tầng</h3><p class="sub">Số biện pháp theo tầng phòng thủ và mức ưu tiên · tiến độ checklist lưu trên trình duyệt này</p><div id="sec-layers"></div></section></div>`)
		order := map[string]int{"P0": 0, "P1": 1, "P2": 2}
		ctrls := append([]security.Control(nil), brief.Controls...)
		sort.SliceStable(ctrls, func(i, j int) bool { return order[ctrls[i].Priority] < order[ctrls[j].Priority] })
		b.WriteString(`<div class="controls">`)
		for i, c := range ctrls {
			fmt.Fprintf(&b, `<details class="ctrl %s" data-layer="%s"><summary><input type="checkbox" data-ck="%s" aria-label="đã làm"><span class="pr">%s</span><b>%s</b><span class="meta">%s · công sức %s · tác động %d/5</span></summary><div class="body"><p>%s</p>`,
				strings.ToLower(c.Priority), esc(c.Layer), esc(ckKey(i, c.Control)), esc(c.Priority), esc(c.Control), esc(security.LabelVI(security.Layers, c.Layer)), esc(c.Effort), c.Impact, esc(c.Why))
			if len(c.Steps) > 0 {
				b.WriteString(`<ol>`)
				for _, st := range c.Steps {
					fmt.Fprintf(&b, `<li>%s</li>`, esc(st))
				}
				b.WriteString(`</ol>`)
			}
			if len(c.Counters) > 0 {
				b.WriteString(`<div class="counters">Chặn: `)
				for _, x := range c.Counters {
					fmt.Fprintf(&b, `<span class="chip" data-counter="%s">%s</span>`, esc(x), esc(x))
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div></details>`)
		}
		b.WriteString(`</div>`)
	} else if !s.public() {
		b.WriteString(`<div class="empty">Playbook phòng thủ được tạo cùng bản tổng hợp: <code>fa sec brief</code>.</div>`)
	}
	if brief != nil && len(brief.Watch) > 0 {
		b.WriteString(`<div class="vnbox"><b>👀 Theo dõi tiếp</b><ul>`)
		for _, x := range brief.Watch {
			fmt.Fprintf(&b, `<li>%s</li>`, esc(x))
		}
		b.WriteString(`</ul></div>`)
	}
	b.WriteString(`</section>`)

	// ---- chi tiết
	b.WriteString(`<section id="explorer" class="sec-block"><h2>🔬 Chi tiết sự cố</h2>
<div class="explorer-bar"><span id="sec-count"></span><label>Sắp xếp <select id="sec-sort"><option value="new">mới nhất</option><option value="sev">nghiêm trọng nhất</option><option value="src">nhiều nguồn nhất</option></select></label></div>
<div id="sec-list"></div><button type="button" class="more" id="sec-more">Xem thêm</button></section>`)

	fmt.Fprintf(&b, `<p class="sub foot-note">Nguồn: %d bài từ báo/blog an ninh mạng (Claude %s trích xuất), <a href="https://www.cisa.gov/known-exploited-vulnerabilities-catalog" rel="noopener">CISA KEV</a>, <a href="https://www.first.org/epss/" rel="noopener">FIRST EPSS</a>`,
		board.Articles, esc(s.cfg.Security.ExtractModel))
	if board.Ransom.Source != "" {
		b.WriteString(`, <a href="https://www.ransomlook.io" rel="noopener">RansomLook</a> (chỉ số liệu tổng hợp theo nhóm/ngành — không hiển thị tên nạn nhân)`)
	}
	b.WriteString(`. Quy kết quốc gia/nhóm theo nội dung tin, có thể là nghi vấn. Bản đồ: Natural Earth.</p>`)

	id := dataScript(&b, map[string]any{"board": board, "brief": brief})
	fmt.Fprintf(&b, `<script>window.FA_SEC_SRC=%q</script><script src="/static/sec.js"></script></div>`, id)
	s.render(w, r, "security", "Bản đồ mối đe doạ an ninh mạng", b.String())
}

func ckKey(i int, s string) string {
	r := []rune(s)
	if len(r) > 40 {
		r = r[:40]
	}
	return fmt.Sprintf("%d-%s", i, string(r))
}

// flag: emoji cờ từ mã ISO alpha-2.
func flag(code string) string {
	if len(code) != 2 {
		return ""
	}
	var b strings.Builder
	for _, c := range strings.ToUpper(code) {
		b.WriteRune(0x1F1E6 + (c - 'A'))
	}
	return b.String()
}
