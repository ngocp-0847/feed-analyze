package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/store"
)

// shouldTrack: lệnh nào được ghi vào lịch sử run (xem `fa open runs`).
// Chỉ ghi lệnh tạo/biến đổi dữ liệu; lệnh đọc (brief, search, stats, sec status, publish preview…) thì không.
func shouldTrack(name string, args []string) bool {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
	}
	switch name {
	case "daily", "fetch", "classify", "analyze", "deep":
		return true
	case "sec":
		return sub != "" && sub != "status"
	case "publish":
		return sub == "" || sub == "build"
	}
	return false
}

// runTracked chạy lệnh, ghi lại run (bắt đầu/kết thúc/trạng thái) vào DB. Lỗi ghi lịch sử không bao giờ làm hỏng lệnh.
func (a *app) runTracked(cmd *command, args []string) (err error) {
	if cmd.noConfig || !shouldTrack(cmd.name, args) {
		return cmd.run(a, args)
	}
	id, serr := a.st.StartRun(cmd.name, runArgs(args))
	if serr != nil {
		logf("⚠ không ghi được lịch sử chạy: %v", serr)
		return cmd.run(a, args)
	}
	defer func() {
		if p := recover(); p != nil {
			a.st.FinishRun(id, store.RunError, fmt.Errorf("panic: %v", p))
			panic(p)
		}
	}()
	err = cmd.run(a, args)
	if errors.Is(err, flag.ErrHelp) {
		a.st.DropRun(id) // chỉ in --help: không có gì để lưu
		return err
	}
	a.st.FinishRun(id, runStatus(a.ctx, err), err)
	return err
}

func runStatus(ctx context.Context, err error) string {
	switch {
	case err == nil:
		return store.RunOK
	case ctx.Err() != nil:
		return store.RunCancelled
	}
	return store.RunError
}

func runArgs(args []string) string {
	s := strings.Join(args, " ")
	if r := []rune(s); len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}

// step bắt đầu đo 1 bước của run hiện hành; gọi hàm trả về khi bước xong (err != nil → bước lỗi).
// Không có run hiện hành thì không ghi gì.
func (a *app) step(name string) func(detail string, err error) {
	start := time.Now()
	return func(detail string, err error) {
		status := "ok"
		if err != nil {
			status = store.RunError
		}
		a.st.AddRunStep(name, start, status, detail, err)
	}
}
