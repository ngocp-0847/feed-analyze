package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// ---------- board lịch sử run (chỉ bản cục bộ: có chi phí Claude và lỗi nội bộ nên không xuất bản công khai) ----------

var runStatus = map[string][2]string{
	store.RunOK:          {"good", "Thành công"},
	store.RunPartial:     {"warning", "Một phần"},
	store.RunError:       {"critical", "Lỗi"},
	store.RunInterrupted: {"serious", "Bị ngắt"},
	store.RunCancelled:   {"neutral", "Đã huỷ"},
	store.RunRunning:     {"neutral", "Đang chạy"},
}

func runStatusHTML(status string) string {
	v, ok := runStatus[status]
	if !ok {
		v = [2]string{"neutral", status}
	}
	return statusHTML(v[0], v[1])
}

func (s *Server) runTime(t time.Time) string { return t.In(s.cfg.Loc).Format("02/01 15:04:05") }

func fmtDur(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func fmtCost(c float64) string {
	if c <= 0 {
		return "—"
	}
	return fmt.Sprintf("$%.3f", c)
}

func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	limit := min(atoi(q.Get("limit"), 50), 500)
	rs, err := s.st.Runs(kind, limit)
	var b strings.Builder
	b.WriteString("<h1>🕘 Lịch sử chạy</h1>")
	if err != nil {
		fmt.Fprintf(&b, `<div class="empty">Lỗi đọc lịch sử: %s</div>`, esc(err.Error()))
		s.render(w, r, "runs", "Lịch sử chạy", b.String())
		return
	}

	// bộ lọc theo loại lệnh
	b.WriteString(`<form class="filters" method="get" action="/runs"><select name="kind"><option value="">Mọi lệnh</option>`)
	for _, k := range s.st.RunKinds() {
		sel := ""
		if k == kind {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>fa %s</option>`, esc(k), sel, esc(k))
	}
	fmt.Fprintf(&b, `</select><input name="limit" type="number" min="1" max="500" value="%d"> run gần nhất <button>Lọc</button></form>`, limit)

	if len(rs) == 0 {
		b.WriteString(`<div class="empty">Chưa có lần chạy nào được ghi lại. Chạy <code>fa daily</code> (hoặc <code>fetch</code>, <code>classify</code>, <code>analyze</code>, <code>deep</code>, <code>sec</code>) — mỗi lần chạy sẽ hiện ở đây.</div>`)
		s.render(w, r, "runs", "Lịch sử chạy", b.String())
		return
	}

	// KPI trên các run đang hiển thị
	var okN, failN int
	var cost float64
	for _, x := range rs {
		cost += x.CostUSD
		switch x.Status {
		case store.RunOK:
			okN++
		case store.RunError, store.RunInterrupted, store.RunPartial:
			failN++
		}
	}
	b.WriteString(`<div class="kpis">`)
	kpi(&b, "Số run", strconv.Itoa(len(rs)), "", "", "trong danh sách bên dưới")
	kpi(&b, "Thành công", strconv.Itoa(okN), "", "", fmt.Sprintf("%d lỗi / một phần / bị ngắt", failN))
	kpi(&b, "Chi phí Claude", fmtCost(cost), "", "", "tổng các run đang hiển thị")
	b.WriteString(`</div>`)

	b.WriteString(`<table class="runs"><thead><tr><th>#</th><th>Lệnh</th><th>Bắt đầu</th><th>Thời gian</th><th>Trạng thái</th><th>Gọi Claude</th><th>Chi phí</th></tr></thead><tbody>`)
	for _, x := range rs {
		calls := "—"
		if x.Calls > 0 {
			calls = strconv.Itoa(x.Calls)
			if x.Failed > 0 {
				calls += fmt.Sprintf(` <span class="badge">%d lỗi</span>`, x.Failed)
			}
		}
		args := ""
		if x.Args != "" {
			args = ` <small>` + esc(x.Args) + `</small>`
		}
		fmt.Fprintf(&b, `<tr><td><a href="/runs/%d">%d</a></td><td><b>fa %s</b>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			x.ID, x.ID, esc(x.Kind), args, s.runTime(x.Started), fmtDur(x.Duration()), runStatusHTML(x.Status), calls, fmtCost(x.CostUSD))
	}
	b.WriteString(`</tbody></table>`)
	s.render(w, r, "runs", "Lịch sử chạy", b.String())
}

func (s *Server) runDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	run, err := s.st.Run(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.NotFound(w, r)
		return
	}
	calls, _ := s.st.RunCalls(id)

	var b strings.Builder
	b.WriteString(`<p><a href="/runs">← Lịch sử chạy</a></p>`)
	fmt.Fprintf(&b, `<h1>Run #%d · fa %s</h1>`, run.ID, esc(run.Kind))
	if run.Args != "" {
		fmt.Fprintf(&b, `<p class="sub"><code>fa %s %s</code></p>`, esc(run.Kind), esc(run.Args))
	}
	b.WriteString(`<div class="kpis">`)
	b.WriteString(`<div class="kpi"><div class="lab">Trạng thái</div><div class="val">` + runStatusHTML(run.Status) + `</div></div>`)
	kpi(&b, "Bắt đầu", s.runTime(run.Started), "", "", "")
	kpi(&b, "Thời gian", fmtDur(run.Duration()), "", "", "")
	kpi(&b, "Gọi Claude", strconv.Itoa(run.Calls), "", "", fmt.Sprintf("%d lỗi", run.Failed))
	kpi(&b, "Chi phí", fmtCost(run.CostUSD), "", "", "")
	b.WriteString(`</div>`)
	if run.Status == store.RunInterrupted {
		b.WriteString(`<div class="empty">Tiến trình bị dừng đột ngột (tắt máy, bị kill…) nên không kịp ghi kết thúc.</div>`)
	}
	if run.Err != "" {
		fmt.Fprintf(&b, `<h2>Lỗi</h2><pre>%s</pre>`, esc(run.Err))
	}

	if len(run.Steps) > 0 {
		b.WriteString(`<h2>Các bước</h2><table><thead><tr><th>Bước</th><th>Bắt đầu</th><th>Thời gian</th><th>Kết quả</th><th>Ghi chú</th></tr></thead><tbody>`)
		for _, st := range run.Steps {
			note := esc(st.Detail)
			if st.Err != "" {
				note += ` <small>` + esc(st.Err) + `</small>`
			}
			state := "good"
			label := "Xong"
			if st.Status == store.RunError {
				state, label = "critical", "Lỗi"
			}
			fmt.Fprintf(&b, `<tr><td><b>%s</b></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				esc(st.Name), s.runTime(st.Started), fmtDur(st.Duration), statusHTML(state, label), note)
		}
		b.WriteString(`</tbody></table>`)
	}

	if len(calls) > 0 {
		fmt.Fprintf(&b, `<h2>Lần gọi Claude (%d)</h2><table><thead><tr><th>Lúc</th><th>Mục đích</th><th>Model</th><th>Thời gian</th><th>Chi phí</th><th>Kết quả</th></tr></thead><tbody>`, len(calls))
		for _, c := range calls {
			state, label, note := "good", "OK", ""
			if !c.OK {
				state, label = "critical", "Lỗi"
				note = ` <small>` + esc(trunc(c.Err, 200)) + `</small>`
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s%s</td></tr>`,
				s.runTime(c.Time), esc(c.Purpose), esc(c.Model), fmtDur(c.Duration), fmtCost(c.CostUSD), statusHTML(state, label), note)
		}
		b.WriteString(`</tbody></table>`)
	}
	if len(run.Steps) == 0 && len(calls) == 0 && run.Err == "" {
		b.WriteString(`<div class="empty">Run này không có bước hay lần gọi Claude nào được ghi lại.</div>`)
	}
	s.render(w, r, "runs", fmt.Sprintf("Run #%d", run.ID), `<div class="prose">`+b.String()+`</div>`)
}
