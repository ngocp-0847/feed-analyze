package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/research"
	"github.com/ngocp-0847/feed-analyze/internal/web"
)

func cmdServe(a *app, args []string) error {
	fs := newFlags("serve")
	addr := fs.String("addr", a.cfg.Web.Addr, "địa chỉ lắng nghe")
	open := fs.Bool("open", false, "mở trình duyệt")
	stop := fs.Bool("stop", false, "tắt server đang chạy nền")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *stop {
		return stopServer(*addr)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if running, _ := serverRunning(*addr, a.home); running {
			logf("• server đã chạy sẵn tại http://%s", *addr)
			if *open {
				openBrowser("http://" + *addr)
			}
			return nil
		}
		return err
	}
	base := "http://" + ln.Addr().String()
	logf("▶ feed-analyze web: %s  (dashboard: %s/dashboard · tắt: Ctrl+C hoặc fa serve --stop)", base, base)
	if *open {
		openBrowser(base)
	}
	h := web.New(a.cfg, a.st)
	srv := &http.Server{Handler: h.Handler(), ReadHeaderTimeout: 10 * time.Second}
	h.Shutdown = func() { _ = srv.Close() }
	go func() { <-a.ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// serverRunning: có server fa đang chạy ở addr không; nếu có nhưng khác home → lỗi.
func serverRunning(addr, home string) (bool, error) {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := string(b)
	if !strings.HasPrefix(body, web.HealthToken) {
		return false, fmt.Errorf("cổng %s đang bị chương trình khác dùng — đổi web.addr trong config", addr)
	}
	if got := strings.TrimSpace(strings.TrimPrefix(body, web.HealthToken)); !samePath(got, home) {
		return true, fmt.Errorf("server tại %s đang phục vụ home khác (%s) — `fa serve --stop` rồi chạy lại", addr, got)
	}
	return true, nil
}

func samePath(a, b string) bool {
	pa, _ := filepath.Abs(a)
	pb, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(pa), filepath.Clean(pb))
}

// ensureServer bật server chạy nền (tiến trình tách rời) nếu chưa có; trả về base URL.
func ensureServer(a *app) (string, error) {
	addr := a.cfg.Web.Addr
	base := "http://" + addr
	ok, err := serverRunning(addr, a.home)
	if err != nil {
		return "", err
	}
	if ok {
		return base, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	logDir := filepath.Join(a.home, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	lf, err := os.OpenFile(filepath.Join(logDir, "serve.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer lf.Close()
	cmd := exec.Command(exe, "--home", a.home, "serve", "--addr", addr)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = lf, lf, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("không bật được server: %w", err)
	}
	_ = cmd.Process.Release()
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if ok, err := serverRunning(addr, a.home); err != nil {
			return "", err
		} else if ok {
			logf("▶ đã bật web server nền tại %s (tắt: fa serve --stop)", base)
			return base, nil
		}
	}
	return "", fmt.Errorf("server không phản hồi — xem %s", filepath.Join(logDir, "serve.log"))
}

func stopServer(addr string) error {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Post("http://"+addr+"/api/shutdown", "text/plain", nil)
	if err != nil {
		fmt.Println("Không có server nào đang chạy tại", addr)
		return nil
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server từ chối tắt (HTTP %d)", resp.StatusCode)
	}
	fmt.Println("✓ đã tắt web server tại", addr)
	return nil
}

// openPath bảo đảm server chạy rồi mở trình duyệt tới path.
func openPath(a *app, path string) error {
	base, err := ensureServer(a)
	if err != nil {
		return err
	}
	u := base + path
	logf("🌐 %s", u)
	openBrowser(u)
	return nil
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func cmdOpen(a *app, args []string) error {
	fs := newFlags("open")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	return openPath(a, resolveWebPath(a, pos))
}

func atoiPos(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// resolveWebPath: "" → bản tin mới nhất · dashboard · runs [id] · security [N] · YYYY-MM-DD · stories · articles · deep [slug] · <slug storyline> · <từ khoá>.
func resolveWebPath(a *app, pos []string) string {
	if len(pos) == 0 {
		return "/"
	}
	first := pos[0]
	switch {
	case first == "today":
		return "/r/" + a.cfg.Today()
	case dateRe.MatchString(first):
		return "/r/" + first
	case first == "dashboard" || first == "dash":
		return "/dashboard"
	case first == "stories" || first == "storyline":
		return "/stories"
	case first == "articles":
		return "/articles"
	case first == "runs" || first == "history":
		if len(pos) > 1 && atoiPos(pos[1]) > 0 {
			return fmt.Sprintf("/runs/%d", atoiPos(pos[1]))
		}
		return "/runs"
	case first == "security" || first == "sec" || first == "threats":
		if len(pos) > 1 && atoiPos(pos[1]) > 0 {
			return fmt.Sprintf("/security/%d", atoiPos(pos[1]))
		}
		return "/security"
	case first == "deep":
		if len(pos) > 1 {
			if p := deepPath(a, pos[1]); p != "" {
				return p
			}
		}
		return "/deep"
	}
	if sl, _ := a.st.Storyline(first); sl != nil {
		return "/s/" + url.PathEscape(first)
	}
	if p := deepPath(a, first); p != "" {
		return p
	}
	return "/articles?days=14&q=" + url.QueryEscape(strings.Join(pos, " "))
}

// deepPath: trang deep mới nhất có slug khớp.
func deepPath(a *app, slug string) string {
	reps, _ := a.st.ReportsByKindPrefix("deep:", 500)
	for _, r := range reps {
		if strings.TrimPrefix(r.Kind, "deep:") == slug {
			return web.DeepURL(r.Date, slug)
		}
	}
	return ""
}

func cmdViz(a *app, args []string) error {
	fs := newFlags("viz")
	all := fs.Bool("all", false, "tạo dashboard cho mọi báo cáo deep chưa có")
	open := fs.Bool("open", false, "mở dashboard trên trình duyệt")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	reps, err := a.st.ReportsByKindPrefix("deep:", 500)
	if err != nil {
		return err
	}
	var todo []struct{ date, kind string }
	for _, r := range reps {
		slug := strings.TrimPrefix(r.Kind, "deep:")
		switch {
		case *all:
			if v, _ := research.LoadViz(a.st, r.Date, r.Kind); v == nil {
				todo = append(todo, struct{ date, kind string }{r.Date, r.Kind})
			}
		case len(pos) > 0 && slug == pos[0]:
			todo = append(todo, struct{ date, kind string }{r.Date, r.Kind})
		}
		if !*all && len(todo) > 0 {
			break
		}
	}
	if len(todo) == 0 {
		if !*all && len(pos) == 0 {
			return errors.New("cần slug báo cáo deep (xem /deep trên web) hoặc --all")
		}
		if *all {
			fmt.Println("✓ mọi báo cáo deep đã có dashboard")
			return nil
		}
		return fmt.Errorf("không có báo cáo deep %q — tạo bằng `fa deep %s`", pos[0], pos[0])
	}
	for _, t := range todo {
		logf("▶ Dựng dashboard cho %s (%s)…", t.kind, a.cfg.Claude.AnalyzeModel)
		start := time.Now()
		v, err := research.SaveViz(a.ctx, a.cfg, a.st, a.runner, t.date, t.kind)
		if err != nil {
			return err
		}
		logf("✓ %d chỉ số · %d nút nhân quả · %d kịch bản · %d bên liên quan · %d chỉ báo (%s)",
			len(v.KeyMetrics), len(v.CausalNodes), len(v.Scenarios), len(v.Stakeholders), len(v.Indicators), time.Since(start).Round(time.Second))
	}
	if *open {
		last := todo[len(todo)-1]
		return openPath(a, web.DeepURL(last.date, strings.TrimPrefix(last.kind, "deep:")))
	}
	return nil
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}
