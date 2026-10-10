package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/classify"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/feed"
	"github.com/ngocp-0847/feed-analyze/internal/report"
	"github.com/ngocp-0847/feed-analyze/internal/skills"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func cmdInit(a *app, args []string) error {
	fs := newFlags("init")
	force := fs.Bool("force", false, "ghi đè config.yaml đã có")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := os.MkdirAll(a.home, 0o755); err != nil {
		return err
	}
	cp := config.Path(a.home)
	if _, err := os.Stat(cp); err == nil && !*force {
		fmt.Println("• config đã có:", cp, "(dùng --force để ghi đè)")
	} else {
		if err := os.WriteFile(cp, config.DefaultYAML, 0o644); err != nil {
			return err
		}
		fmt.Println("✓ tạo config:", cp)
	}
	if err := a.load(); err != nil {
		return err
	}
	defer a.st.Close()
	fmt.Println("✓ database:", a.cfg.DBPath())
	if err := os.MkdirAll(a.cfg.Report.Dir, 0o755); err != nil {
		return err
	}
	files, err := skills.Install(filepath.Join(a.home, ".claude", "skills"))
	if err != nil {
		return err
	}
	if err := skills.WriteClaudeMD(a.home); err != nil {
		return err
	}
	fmt.Printf("✓ cài %d skill (%d file) vào %s\n", len(skills.Names()), len(files), filepath.Join(a.home, ".claude", "skills"))
	if v, err := a.runner.Check(a.ctx); err != nil {
		fmt.Println("⚠", err)
	} else {
		fmt.Println("✓ Claude CLI:", v)
	}
	warnPath()
	fmt.Printf(`
Tiếp theo:
  1. Sửa chủ đề / profile / nguồn:  %s
  2. Chạy bản tin đầu tiên:        fa daily
  3. Đọc:                          fa brief   |  fa serve --open  |  fa chat
`, cp)
	return nil
}

// warnPath nhắc nếu `fa` chưa nằm trong PATH (skills gọi `fa ...`).
func warnPath() {
	dir := exeDir()
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return
		}
	}
	fmt.Printf("⚠ thư mục chứa fa (%s) chưa có trong PATH — skills trong `fa chat` vẫn chạy (fa tự thêm), nhưng nên `go install ./cmd/fa`.\n", dir)
}

func cmdFetch(a *app, args []string) error {
	fs := newFlags("fetch")
	only := fs.String("feed", "", "chỉ quét feed có tên chứa chuỗi này")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	_, err := runFetch(a, *only)
	return err
}

func runFetch(a *app, only string) (int, error) {
	feeds := a.cfg.ActiveFeeds()
	if only != "" {
		var sel []config.Feed
		for _, f := range feeds {
			if strings.Contains(strings.ToLower(f.Name), strings.ToLower(only)) {
				sel = append(sel, f)
			}
		}
		feeds = sel
	}
	if len(feeds) == 0 {
		return 0, errors.New("không có feed nào để quét")
	}
	logf("▶ Quét %d nguồn RSS…", len(feeds))
	start := time.Now()
	res := feed.FetchAll(a.ctx, a.cfg, a.st, feeds, func(r feed.Result) {
		if r.Err != nil {
			logf("  ✗ %-26s %v", r.Feed.Name, r.Err)
		} else if r.New > 0 {
			logf("  ✓ %-26s +%d mới (%d trong hạn)", r.Feed.Name, r.New, r.Fresh)
		}
	})
	total, failed := 0, 0
	for _, r := range res {
		total += r.New
		if r.Err != nil {
			failed++
		}
	}
	logf("✓ %d bài mới từ %d nguồn (%d lỗi) trong %s", total, len(feeds)-failed, failed, time.Since(start).Round(time.Second))
	return total, nil
}

func cmdClassify(a *app, args []string) error {
	fs := newFlags("classify")
	limit := fs.Int("limit", 1500, "số bài tối đa")
	redo := fs.Bool("redo", false, "phân loại lại bài đã phân loại (vd sau khi đổi topic)")
	days := fs.Int("days", 2, "với --redo: số ngày gần nhất")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *redo {
		logf("▶ Phân loại lại bài %d ngày qua (%s)…", *days, a.cfg.Claude.ClassifyModel)
		p, err := classify.Redo(a.ctx, a.cfg, a.st, a.runner, *days, classifyProgress())
		logf("✓ %d/%d bài, %d liên quan", p.Done, p.Total, p.Relevant)
		return err
	}
	return runClassify(a, *limit)
}

func classifyProgress() func(classify.Progress) {
	return func(p classify.Progress) {
		if p.Err != nil {
			logf("  ✗ batch lỗi: %v", p.Err)
			return
		}
		logf("  … %d/%d bài (%d liên quan)", p.Done, p.Total, p.Relevant)
	}
}

func runClassify(a *app, limit int) error {
	n := a.st.CountUnclassified()
	if n == 0 {
		logf("✓ Không có bài cần phân loại")
		return nil
	}
	logf("▶ Phân loại %d bài (%s, batch %d, song song %d)…", min(n, limit), a.cfg.Claude.ClassifyModel, a.cfg.Claude.BatchSize, a.cfg.Claude.Concurrency)
	start := time.Now()
	p, err := classify.Run(a.ctx, a.cfg, a.st, a.runner, limit, classifyProgress())
	logf("✓ Phân loại %d/%d bài, %d liên quan tới chủ đề (%s)", p.Done, p.Total, p.Relevant, time.Since(start).Round(time.Second))
	if err != nil && p.Done > 0 {
		logf("⚠ %v — bài lỗi sẽ được thử lại lần chạy sau", err)
		return nil
	}
	return err
}

func cmdAnalyze(a *app, args []string) error {
	fs := newFlags("analyze")
	date := fs.String("date", "", "ngày bản tin YYYY-MM-DD (mặc định hôm nay)")
	topics := fs.String("topic", "", "chỉ phân tích các topic (phân cách dấu phẩy)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	var ts []string
	if *topics != "" {
		ts = strings.Split(*topics, ",")
	}
	_, err := runAnalyze(a, *date, ts)
	return err
}

func runAnalyze(a *app, date string, topics []string) (string, error) {
	if date == "" {
		date = a.cfg.Today()
	}
	logf("▶ Phân tích bản tin %s (chủ đề: %s, tổng hợp: %s)…", date, a.cfg.Claude.AnalyzeModel, a.cfg.Claude.SynthesisModel)
	start := time.Now()
	d, err := analyze.Run(a.ctx, a.cfg, a.st, a.runner, analyze.Options{Date: date, Topics: topics, Log: logf})
	if d == nil {
		return "", err
	}
	if err != nil {
		logf("⚠ %v", err)
	}
	path, werr := writeDaily(a, d)
	if werr != nil {
		return path, werr
	}
	logf("✓ Bản tin: %s (%s)", path, time.Since(start).Round(time.Second))
	return path, err
}

func writeDaily(a *app, d *analyze.Daily) (string, error) {
	in, err := report.Collect(a.st, d)
	if err != nil {
		return "", err
	}
	md := report.Markdown(a.cfg, in, report.Options{})
	_ = a.st.SaveReport(store.Report{Date: d.Date, Kind: "daily-md", Markdown: md})
	return report.WriteFiles(a.cfg, d.Date, md)
}

func cmdDaily(a *app, args []string) (err error) {
	fs := newFlags("daily")
	skipFetch := fs.Bool("skip-fetch", false, "bỏ bước quét RSS")
	noAnalyze := fs.Bool("no-analyze", false, "chỉ quét + phân loại")
	logFile := fs.Bool("log", false, "ghi log vào <home>/logs/ (dùng khi chạy theo lịch)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *logFile {
		closeLog, err := teeLog(a)
		if err != nil {
			return err
		}
		defer closeLog()
		defer func() {
			if err != nil {
				logf("✗ %v", err) // ghi lỗi vào log trước khi đóng
			}
		}()
	}
	start := time.Now()
	logf("══ fa daily — %s ══", time.Now().In(a.cfg.Loc).Format("2006-01-02 15:04"))
	if !*skipFetch {
		if _, err := runFetch(a, ""); err != nil {
			return err
		}
	}
	if err := runClassify(a, 2000); err != nil {
		return err
	}
	if *noAnalyze {
		return nil
	}
	path, err := runAnalyze(a, "", nil)
	if path != "" {
		st, _ := a.st.Stats(time.Now().Add(-24 * time.Hour))
		logf("══ Xong trong %s · chi phí Claude 24h ≈ $%.2f ══", time.Since(start).Round(time.Second), st.Cost24h)
		logf("Đọc: fa brief  |  fa serve --open  |  fa chat")
	}
	return err
}

// teeLog chuyển stderr sang file log + stderr gốc; hàm trả về đóng & flush log.
func teeLog(a *app) (func(), error) {
	dir := filepath.Join(a.home, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "daily-"+a.cfg.Today()+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.MultiWriter(f, orig), r); close(done) }()
	return func() {
		os.Stderr = orig
		w.Close()
		<-done
		f.Close()
	}, nil
}
