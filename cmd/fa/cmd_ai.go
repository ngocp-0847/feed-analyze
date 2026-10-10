package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/research"
	"github.com/ngocp-0847/feed-analyze/internal/skills"
)

func cmdDeep(a *app, args []string) error {
	fs := newFlags("deep")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New(`cần mục tiêu: fa deep <slug | id | "câu hỏi">  (gợi ý: fa stories)`)
	}
	target := strings.Join(pos, " ")
	t := research.Resolve(a.st, target)
	logf("▶ Phân tích sâu [%s] %s (%s, có WebSearch/WebFetch — có thể mất vài phút)…", t.Kind, target, a.cfg.Claude.DeepModel)
	start := time.Now()
	md, path, err := research.Deep(a.ctx, a.cfg, a.st, a.runner, target, research.PrintEvents(os.Stderr))
	if md != "" {
		fmt.Println()
		fmt.Print(md)
	}
	if err != nil {
		return err
	}
	logf("\n✓ Lưu: %s (%s)", path, time.Since(start).Round(time.Second))
	return nil
}

func cmdAsk(a *app, args []string) error {
	fs := newFlags("ask")
	web := fs.Bool("web", false, "cho phép nghiên cứu web (WebSearch/WebFetch)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New(`cần câu hỏi: fa ask "lãi suất NHNN tuần này thế nào?"`)
	}
	q := strings.Join(pos, " ")
	logf("▶ Đang trả lời (%s)…", a.cfg.Claude.DeepModel)
	ans, err := research.Ask(a.ctx, a.cfg, a.st, a.runner, q, *web, research.PrintEvents(os.Stderr))
	if ans != "" {
		fmt.Println(strings.TrimSpace(ans))
	}
	return err
}

const chatSystem = `Bạn là trợ lý bản tin cá nhân chạy trong thư mục home của feed-analyze (CLI "fa"). Luôn trả lời tiếng Việt.
Bối cảnh bản tin mới nhất nằm ở file .fa-context.md trong thư mục hiện tại — đọc nó trước khi trả lời câu đầu tiên.
Dùng các skill: daily-catchup (bản tin 5 phút), news-deep-dive (đào sâu), micro-macro-lens (khung vi mô ↔ vĩ mô), connect-the-dots (xu hướng nhiều ngày), feed-curator (sửa chủ đề/nguồn).
Lấy dữ liệu qua lệnh: fa context | fa brief | fa stories | fa story <slug> | fa search "<q>" | fa show <id>. Không bịa tin; trích #id/link.
Hỏi người dùng trước khi chạy lệnh tốn phí: fa daily, fa analyze, fa deep.`

func cmdChat(a *app, args []string) error {
	fs := newFlags("chat")
	cont := fs.Bool("continue", false, "tiếp tục phiên chat gần nhất")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	// đảm bảo skills & CLAUDE.md mới nhất
	if _, err := skills.Install(filepath.Join(a.home, ".claude", "skills")); err != nil {
		return err
	}
	if err := skills.WriteClaudeMD(a.home); err != nil {
		return err
	}
	ctxMD, err := research.Context(a.cfg, a.st, "", 3)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(a.home, ".fa-context.md"), []byte(ctxMD), 0o644); err != nil {
		return err
	}
	// skills gọi `fa ...` — bảo đảm binary hiện tại có trong PATH của phiên claude
	if d := exeDir(); d != "" {
		os.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.Setenv("FA_HOME", a.home)
	cargs := []string{"--append-system-prompt", chatSystem,
		"--allowedTools", "Bash(fa *)", "Read", "Glob", "Grep", "WebSearch", "WebFetch"}
	if *cont {
		cargs = append(cargs, "--continue")
	}
	first := strings.Join(pos, " ")
	if first == "" && !*cont {
		first = "Cho tôi bản tin catch-up hôm nay (dùng skill daily-catchup)."
	}
	if first != "" {
		cargs = append(cargs, first)
	}
	logf("▶ Mở Claude Code tại %s (thoát: /exit)…", a.home)
	return a.runner.Interactive(a.ctx, cargs...)
}
