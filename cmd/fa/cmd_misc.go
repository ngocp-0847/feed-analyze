package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/skills"
	"github.com/ngocp-0847/feed-analyze/internal/web"
)

func cmdServe(a *app, args []string) error {
	fs := newFlags("serve")
	addr := fs.String("addr", "127.0.0.1:8787", "địa chỉ lắng nghe")
	open := fs.Bool("open", false, "mở trình duyệt")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	logf("▶ feed-analyze web: %s  (Ctrl+C để dừng)", url)
	if *open {
		openBrowser(url)
	}
	srv := &http.Server{Handler: web.New(a.cfg, a.st).Handler()}
	go func() { <-a.ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func cmdSkills(a *app, args []string) error {
	fs := newFlags("skills")
	global := fs.Bool("global", false, "cài vào ~/.claude/skills (dùng được trong mọi phiên Claude Code)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sub := "list"
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "list":
		for _, n := range skills.Names() {
			fmt.Println(" •", n)
		}
		return nil
	case "install":
		dir := filepath.Join(a.home, ".claude", "skills")
		if *global {
			uh, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir = filepath.Join(uh, ".claude", "skills")
		}
		files, err := skills.Install(dir)
		if err != nil {
			return err
		}
		fmt.Printf("✓ cài %d skill (%d file) vào %s\n", len(skills.Names()), len(files), dir)
		if *global {
			fmt.Println("  Lưu ý: skills gọi lệnh `fa` — cần `fa` trong PATH (go install ./cmd/fa).")
		}
		return nil
	}
	return fmt.Errorf("lệnh con không hợp lệ: %s (install | list)", sub)
}

const taskName = "feed-analyze-daily"

var timeRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

func cmdSchedule(a *app, args []string) error {
	fs := newFlags("schedule")
	at := fs.String("time", "06:30", "giờ chạy hằng ngày HH:MM")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sub := "show"
	if len(pos) > 0 {
		sub = pos[0]
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		h, m, _ := strings.Cut(*at, ":")
		fmt.Printf("Thêm dòng sau vào `crontab -e`:\n\n%s %s * * * FA_HOME=%q %q daily --log\n", strings.TrimLeft(m, "0"), h, a.home, exe)
		return nil
	}
	switch sub {
	case "install":
		if !timeRe.MatchString(*at) {
			return fmt.Errorf("giờ không hợp lệ %q (HH:MM)", *at)
		}
		tr := fmt.Sprintf(`"%s" --home "%s" daily --log`, exe, a.home)
		out, err := exec.Command("schtasks", "/Create", "/F", "/SC", "DAILY", "/TN", taskName, "/ST", *at, "/TR", tr).CombinedOutput()
		if err != nil {
			return fmt.Errorf("schtasks: %v %s", err, out)
		}
		fmt.Printf("✓ Đã lên lịch %q chạy lúc %s hằng ngày\n  Lệnh: %s\n  Log:  %s\n", taskName, *at, tr, filepath.Join(a.home, "logs"))
		fmt.Println("  Lưu ý: task chạy khi bạn đã đăng nhập Windows; máy tắt lúc đó thì lần sau mở `fa daily` thủ công.")
		return nil
	case "remove":
		out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("schtasks: %v %s", err, out)
		}
		fmt.Println("✓ Đã gỡ lịch", taskName)
		return nil
	case "show":
		out, err := exec.Command("schtasks", "/Query", "/TN", taskName, "/FO", "LIST").CombinedOutput()
		if err != nil {
			fmt.Println("Chưa lên lịch. Dùng: fa schedule install --time 06:30")
			return nil
		}
		fmt.Print(string(out))
		return nil
	}
	return fmt.Errorf("lệnh con không hợp lệ: %s (install | remove | show)", sub)
}

func cmdDoctor(a *app, _ []string) error {
	ok := true
	check := func(name string, err error, detail string) {
		if err != nil {
			ok = false
			fmt.Printf("✗ %-12s %v\n", name, err)
			return
		}
		fmt.Printf("✓ %-12s %s\n", name, detail)
	}
	fmt.Println("fa", version, "·", runtime.GOOS+"/"+runtime.GOARCH)
	cfg, err := config.Load(a.home)
	check("config", err, config.Path(a.home))
	bin := "claude"
	if cfg != nil {
		bin = cfg.Claude.Bin
		check("topics", nil, strings.Join(cfg.TopicIDs(), ", "))
		check("feeds", nil, fmt.Sprintf("%d đang bật", len(cfg.ActiveFeeds())))
	}
	r := &llm.Runner{Bin: bin}
	v, err := r.Check(a.ctx)
	check("claude", err, v)
	if _, err := exec.LookPath("fa"); err != nil {
		fmt.Printf("⚠ %-12s `fa` chưa có trong PATH — chạy `go install ./cmd/fa` để skills gọi được\n", "path")
	} else {
		check("path", nil, "fa có trong PATH")
	}
	if !ok {
		return errors.New("có lỗi cần xử lý")
	}
	return nil
}
