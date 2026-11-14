// fa — feed-analyze: quét tin RSS hằng ngày, Claude phân loại & phân tích sâu theo chủ đề quan tâm.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	_ "time/tzdata" // Windows không có tzdata hệ thống

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/llm"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

const version = "1.0.0"

type command struct {
	name, args, help string
	run              func(a *app, args []string) error
	noConfig         bool
}

var commands []command

func init() {
	commands = []command{
		{name: "init", args: "[--force]", help: "Tạo config, database và cài skills vào thư mục home", run: cmdInit, noConfig: true},
		{name: "daily", args: "[--skip-fetch] [--no-analyze] [--log] [--open] [--publish]", help: "Chạy trọn quy trình ngày: fetch → classify → analyze → bản tin", run: cmdDaily},
		{name: "fetch", args: "[--feed <tên>]", help: "Quét RSS, lưu bài mới", run: cmdFetch},
		{name: "classify", args: "[--limit N] [--redo --days N]", help: "Claude phân loại bài chưa phân loại", run: cmdClassify},
		{name: "analyze", args: "[--date D] [--topic a,b] [--open]", help: "Claude phân tích storyline từng chủ đề + tổng hợp liên chủ đề", run: cmdAnalyze},
		{name: "brief", args: "[--date D] [--web]", help: "In bản tin ngày (markdown); --web mở trên trình duyệt", run: cmdBrief},
		{name: "open", args: "[dashboard | YYYY-MM-DD | deep [slug] | <slug> | <từ khoá>]", help: "Mở web (tự bật server nền): bản tin, dashboard, storyline, phân tích sâu", run: cmdOpen},
		{name: "context", args: "[--date D] [--days N]", help: "Bối cảnh gọn cho AI: bản tin + storyline đang hoạt động", run: cmdContext},
		{name: "stories", args: "[--topic T] [--days N] [--json]", help: "Liệt kê storyline", run: cmdStories},
		{name: "story", args: "<slug> [--json]", help: "Timeline + bài viết của 1 storyline", run: cmdStory},
		{name: "search", args: "<từ khoá> [--topic T] [--days N] [--limit N] [--json]", help: "Tìm bài (hỗ trợ không dấu)", run: cmdSearch},
		{name: "show", args: "<id> [--json]", help: "Chi tiết 1 bài", run: cmdShow},
		{name: "deep", args: "<slug | id | \"câu hỏi\"> [--open]", help: "Phân tích sâu có nghiên cứu web + dashboard chart", run: cmdDeep},
		{name: "viz", args: "<slug> [--open] | --all", help: "Dựng (lại) dashboard chart cho báo cáo deep đã có", run: cmdViz},
		{name: "sec", args: "[status | update | backfill | extract | sources | brief] [--days N] [--open]", help: "An ninh mạng: bản đồ mối đe doạ, xu hướng tấn công, playbook phòng thủ", run: cmdSec},
		{name: "ask", args: "\"câu hỏi\" [--web]", help: "Hỏi đáp nhanh dựa trên kho tin", run: cmdAsk},
		{name: "chat", args: "[--continue] [lời nhắn đầu]", help: "Mở phiên Claude Code tương tác với skills bản tin", run: cmdChat},
		{name: "serve", args: "[--addr A] [--open] [--stop]", help: "Chạy web server (foreground); --stop tắt server nền", run: cmdServe},
		{name: "publish", args: "[build | preview | setup | status] [--no-deploy]", help: "Xuất bản site tĩnh công khai lên Cloudflare Pages (free)", run: cmdPublish},
		{name: "feeds", args: "[check] [--url U]", help: "Trạng thái / kiểm tra nguồn RSS", run: cmdFeeds},
		{name: "topics", help: "Liệt kê chủ đề đang theo dõi", run: cmdTopics},
		{name: "stats", help: "Thống kê kho tin, chi phí Claude, đường dẫn", run: cmdStats},
		{name: "skills", args: "[install [--global] | list]", help: "Cài/liệt kê Claude Code skills", run: cmdSkills, noConfig: true},
		{name: "schedule", args: "[install --time 06:30 | remove | show]", help: "Lên lịch chạy `fa daily` hằng ngày", run: cmdSchedule},
		{name: "doctor", help: "Kiểm tra môi trường (Claude CLI, config, DB)", run: cmdDoctor, noConfig: true},
		{name: "version", help: "In phiên bản", run: func(*app, []string) error { fmt.Println("fa", version); return nil }, noConfig: true},
	}
}

type app struct {
	home   string
	cfg    *config.Config
	st     *store.Store
	runner *llm.Runner
	ctx    context.Context
}

func main() {
	setupConsole()
	args := os.Args[1:]
	home := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "--home") {
		if v, ok := strings.CutPrefix(args[0], "--home="); ok {
			home, args = v, args[1:]
		} else if len(args) > 1 {
			home, args = args[1], args[2:]
		} else {
			args = args[1:]
		}
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return
	}
	var cmd *command
	for i := range commands {
		if commands[i].name == args[0] {
			cmd = &commands[i]
		}
	}
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "lệnh không tồn tại: %s\n\n", args[0])
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a := &app{home: config.HomeDir(home), ctx: ctx}
	if !cmd.noConfig {
		if err := a.load(); err != nil {
			fail(err)
		}
		defer a.st.Close()
	}
	if err := a.runTracked(cmd, args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fail(err)
	}
}

func (a *app) load() error {
	cfg, err := config.Load(a.home)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	a.cfg, a.st = cfg, st
	a.runner = &llm.Runner{
		Bin: cfg.Claude.Bin, Dir: cfg.Home, Timeout: time.Duration(cfg.Claude.TimeoutMinutes) * time.Minute,
		Log: st.LogLLMCall,
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "✗", err)
	os.Exit(1)
}

func usage() {
	fmt.Println(`fa — feed-analyze ` + version + `: bản tin RSS cá nhân + Claude phân tích vi mô ↔ vĩ mô

Cách dùng: fa [--home DIR] <lệnh> [tuỳ chọn]
  (home mặc định: $FA_HOME hoặc ~/.feed-analyze)

Lệnh:`)
	for _, c := range commands {
		fmt.Printf("  %-9s %-46s %s\n", c.name, c.args, c.help)
	}
	fmt.Println(`
Bắt đầu nhanh:
  fa init            # tạo config ~/.feed-analyze/config.yaml (sửa chủ đề/nguồn tại đây)
  fa daily           # quét + phân tích → bản tin hôm nay
  fa open            # mở bản tin trên web (tự bật server nền) · fa open dashboard · fa brief --web
  fa sec backfill --open   # an ninh mạng: dựng bản đồ mối đe doạ 30 ngày · sau đó: fa open security
  fa chat            # trò chuyện với Claude về tin hôm nay (zoom vi mô ↔ vĩ mô)
  fa schedule install --time 06:30   # tự chạy mỗi sáng
  fa publish setup && fa publish     # đưa bản tin + dashboard lên web công khai (Cloudflare Pages)`)
}

// parseFlags cho phép flag đứng sau đối số vị trí (vd: fa search "x" --days 7).
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("fa "+name, flag.ContinueOnError)
	return fs
}

func logf(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(p)
}
