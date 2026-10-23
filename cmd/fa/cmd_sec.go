package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/feed"
	"github.com/ngocp-0847/feed-analyze/internal/security"
)

// fa sec — bản đồ mối đe doạ an ninh mạng.
func cmdSec(a *app, args []string) error {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := newFlags("sec " + sub)
	days := fs.Int("days", 0, "số ngày (mặc định: update 7, backfill theo security.backfill_days, status/brief 30)")
	open := fs.Bool("open", false, "mở bản đồ trên trình duyệt khi xong")
	asJSON := fs.Bool("json", false, "status: in dữ liệu JSON")
	redo := fs.Bool("redo", false, "extract: xoá kết quả cũ trong cửa sổ và trích xuất lại")
	noBrief := fs.Bool("no-brief", false, "update/backfill: bỏ bước tổng hợp xu hướng")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if !a.cfg.SecurityOn() {
		return fmt.Errorf("bản đồ an ninh mạng đang tắt: cần `security.enabled: true` và topic %q trong %s (mẫu: internal/config/default.yaml)",
			a.cfg.Security.Topic, config.Path(a.home))
	}
	d := func(def int) int {
		if *days > 0 {
			return *days
		}
		return def
	}
	var err error
	switch sub {
	case "", "status":
		return secStatus(a, d(30), *asJSON)
	case "update":
		err = runSecUpdate(a, d(7), !*noBrief)
	case "backfill":
		err = secBackfill(a, d(a.cfg.Security.BackfillDays), !*noBrief)
	case "extract":
		err = secExtract(a, d(7), *redo)
	case "sources":
		err = secSources(a, d(7))
		if e2 := secEPSS(a, d(7)); err == nil {
			err = e2
		}
	case "brief":
		err = secBrief(a, d(30))
	default:
		return fmt.Errorf("fa sec: không có lệnh con %q (status | update | backfill | extract | sources | brief)", sub)
	}
	if err == nil && *open {
		err = openPath(a, "/security")
	}
	return err
}

// runSecUpdate: KEV + EPSS + leak site + trích xuất tin mới, rồi (tuỳ chọn) tổng hợp. Dùng trong fa daily.
func runSecUpdate(a *app, days int, brief bool) error {
	logf("▶ An ninh mạng: cập nhật nguồn tình báo + trích xuất %d ngày gần nhất…", days)
	if err := secSources(a, max(days, 3)); err != nil {
		logf("⚠ %v", err)
	}
	if err := secExtract(a, days, false); err != nil {
		return err
	}
	if err := secEPSS(a, days); err != nil {
		logf("⚠ %v", err)
	}
	if brief {
		return secBrief(a, 30)
	}
	return nil
}

func secSources(a *app, days int) error {
	var firstErr error
	keep := func(err error) {
		if err != nil {
			logf("  ✗ %v", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	total, added, err := security.UpdateKEV(a.ctx, a.cfg, a.st)
	keep(err)
	if err == nil {
		logf("  ✓ CISA KEV: %d CVE bị khai thác thực tế (+%d mới)", total, added)
	}
	if a.cfg.Security.RansomURL != "" {
		n, nw, err := security.UpdateRansom(a.ctx, a.cfg, a.st, min(days, 30))
		keep(err)
		if err == nil {
			logf("  ✓ Leak site ransomware: %d bài đăng %d ngày qua (+%d mới)", n, min(days, 30), nw)
		}
	}
	return firstErr
}

// secEPSS: điểm EPSS cho CVE trong KEV gần đây + CVE nhắc trong tin (chạy sau trích xuất).
func secEPSS(a *app, days int) error {
	if !a.cfg.Security.EPSS {
		return nil
	}
	var cves []string
	kevs, _ := a.st.KEVSince(time.Now().AddDate(0, 0, -max(days, 90)).Format("2006-01-02"))
	for _, k := range kevs {
		cves = append(cves, k.CVE)
	}
	if b, err := security.BuildBoard(a.cfg, a.st, max(days, 30)); err == nil {
		for id := range b.CVEs {
			cves = append(cves, id)
		}
	}
	n, err := security.UpdateEPSS(a.ctx, a.st, cves)
	if err != nil {
		return err
	}
	logf("  ✓ EPSS: cập nhật %d CVE", n)
	return nil
}

func secExtract(a *app, days int, redo bool) error {
	since := time.Now().In(a.cfg.Loc).AddDate(0, 0, -days+1)
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, a.cfg.Loc)
	if redo {
		n, err := a.st.DeleteSecEvents(since.Format("2006-01-02"))
		if err != nil {
			return err
		}
		logf("  • xoá %d kết quả trích xuất cũ từ %s", n, since.Format("2006-01-02"))
	}
	n := a.st.CountSecPending(a.cfg.Security.Topic, a.cfg.Analyze.MinRelevance, since)
	if n == 0 {
		logf("  ✓ Không có bài an ninh mạng mới cần trích xuất")
		return nil
	}
	logf("  … trích xuất tình báo %d bài (%s, batch %d)", n, a.cfg.Security.ExtractModel, a.cfg.Security.BatchSize)
	start := time.Now()
	res, err := security.Extract(a.ctx, a.cfg, a.st, a.runner, security.ExtractOptions{Since: since, Log: logf})
	logf("  ✓ Trích xuất %d/%d bài → %d sự cố/chiến dịch (%s)", res.Done, res.Total, res.Incidents, time.Since(start).Round(time.Second))
	if err != nil && res.Done > 0 {
		logf("  ⚠ %v — bài lỗi sẽ được thử lại lần sau", err)
		return nil
	}
	return err
}

func secBrief(a *app, days int) error {
	logf("  … tổng hợp xu hướng tấn công + playbook phòng thủ %d ngày (%s)", days, a.cfg.Security.BriefModel)
	start := time.Now()
	b, err := security.MakeBrief(a.ctx, a.cfg, a.st, a.runner, days)
	if err != nil {
		return err
	}
	logf("  ✓ %s [mức đe doạ: %s] (%s)", b.Headline, b.ThreatLevel, time.Since(start).Round(time.Second))
	return nil
}

// secBackfill: quét lại các nguồn security với hạn tuổi bài dài hơn để có dữ liệu xu hướng ngay.
func secBackfill(a *app, days int, brief bool) error {
	var feeds []config.Feed
	for _, f := range a.cfg.ActiveFeeds() {
		for _, t := range f.Topics {
			if t == a.cfg.Security.Topic {
				feeds = append(feeds, f)
				break
			}
		}
	}
	c := *a.cfg
	c.Fetch.MaxAgeHours = days * 24
	c.Fetch.MaxItemsPerFeed = 300
	logf("▶ Backfill an ninh mạng: quét %d nguồn, lấy bài tới %d ngày…", len(feeds), days)
	total := 0
	feed.FetchAll(a.ctx, &c, a.st, feeds, func(r feed.Result) {
		if r.Err != nil {
			logf("  ✗ %-26s %v", r.Feed.Name, r.Err)
		} else {
			logf("  ✓ %-26s +%d mới (%d trong hạn / %d item)", r.Feed.Name, r.New, r.Fresh, r.Items)
		}
		total += r.New
	})
	logf("✓ %d bài mới", total)
	if err := runClassify(a, 5000); err != nil {
		return err
	}
	if err := secSources(a, days); err != nil {
		logf("⚠ %v", err)
	}
	if err := secExtract(a, days, false); err != nil {
		return err
	}
	if err := secEPSS(a, days); err != nil {
		logf("⚠ %v", err)
	}
	if brief {
		return secBrief(a, min(days, 30))
	}
	return nil
}

func secStatus(a *app, days int, asJSON bool) error {
	b, err := security.BuildBoard(a.cfg, a.st, days)
	if err != nil {
		return err
	}
	br, bdate, _ := security.LoadBrief(a.st, "")
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		return enc.Encode(map[string]any{"board": b, "brief": br, "brief_date": bdate})
	}
	fmt.Printf("🛡️  Bản đồ mối đe doạ %s → %s\n", b.Since, b.Until)
	fmt.Printf("   %d bài → %d sự kiện · %d sự cố/chiến dịch · %d liên quan VN · KEV mới %d · leak site %d nạn nhân\n\n",
		b.Articles, len(b.Incidents), b.Counts["threats"], b.Counts["vn"], len(b.KEV), b.Ransom.Total)
	if br != nil {
		fmt.Printf("Tổng hợp (%s) — mức đe doạ: %s\n  %s\n\n", bdate, br.ThreatLevel, br.Headline)
		for _, t := range br.Trends {
			fmt.Printf("  • [%s] %s — %s\n", t.Direction, t.Title, t.Detail)
		}
		fmt.Println()
	}
	row := func(title string, kvs []security.KV, label func(string) string) {
		var parts []string
		for i, kv := range kvs {
			if i >= 8 {
				break
			}
			parts = append(parts, fmt.Sprintf("%s %d", label(kv.K), kv.N))
		}
		if len(parts) > 0 {
			fmt.Printf("%-18s %s\n", title, strings.Join(parts, " · "))
		}
	}
	id := func(s string) string { return s }
	vi := func(ls []security.Label) func(string) string {
		return func(s string) string { return security.LabelVI(ls, s) }
	}
	row("Họ tấn công", b.Tally(func(i *security.Incident) []string { return i.Fam }, true), vi(security.Families))
	row("Ngành bị nhắm", b.Tally(func(i *security.Incident) []string { return i.Sec }, true), vi(security.Sectors))
	row("Nạn nhân (nước)", b.Tally(func(i *security.Incident) []string { return append(append([]string{}, i.VC...), i.TC...) }, true), id)
	row("Nguồn tấn công", b.Tally(func(i *security.Incident) []string { return []string{i.Origin} }, true), id)
	row("Nhóm tấn công", b.Tally(func(i *security.Incident) []string { return []string{i.Actor} }, false), id)
	row("Kỹ thuật", b.Tally(func(i *security.Incident) []string { return i.Tech }, false), id)
	row("CVE", b.Tally(func(i *security.Incident) []string { return i.CVE }, false), id)
	fmt.Println("\nXem bản đồ: fa open security   ·   cập nhật: fa sec update   ·   JSON: fa sec --json")
	return nil
}
