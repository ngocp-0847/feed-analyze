package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/publish"
	"github.com/ngocp-0847/feed-analyze/internal/web"
)

func cmdPublish(a *app, args []string) error {
	fs := newFlags("publish")
	noDeploy := fs.Bool("no-deploy", false, "chỉ build site vào out_dir, không deploy")
	addr := fs.String("addr", "127.0.0.1:8788", "với `preview`: địa chỉ xem thử")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	sub := ""
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "":
		if _, err := buildSite(a); err != nil {
			return err
		}
		if *noDeploy {
			return nil
		}
		_, err := deploySite(a)
		return err
	case "build":
		_, err := buildSite(a)
		return err
	case "preview":
		if _, err := buildSite(a); err != nil {
			return err
		}
		return previewSite(a, *addr)
	case "setup":
		return setupPages(a)
	case "status":
		return pagesStatus(a)
	}
	return fmt.Errorf("lệnh con không hợp lệ: %s (build | preview | setup | status)", sub)
}

func publicURL(a *app) string {
	if d := a.cfg.Publish.Domain; d != "" {
		return "https://" + d
	}
	return "https://" + a.cfg.Publish.Project + ".pages.dev"
}

func buildSite(a *app) (web.ExportStats, error) {
	p := a.cfg.Publish
	since := ""
	if p.Days > 0 {
		since = time.Now().In(a.cfg.Loc).AddDate(0, 0, -p.Days+1).Format("2006-01-02")
	}
	logf("▶ Build site tĩnh → %s (bản tin từ %s)", p.OutDir, map[bool]string{true: since, false: "đầu"}[since != ""])
	s := web.NewPublic(a.cfg, a.st, web.PublicOptions{SiteTitle: p.SiteTitle, Description: p.Description, BaseURL: publicURL(a), Since: since, Exclude: p.Exclude})
	start := time.Now()
	st, err := s.Export(p.OutDir)
	if err != nil {
		return st, err
	}
	logf("✓ %d trang (%d bản tin · %d storyline · %d phân tích sâu) + %d asset · %.1f MB (%s)",
		st.Pages, st.Dailies, st.Stories, st.Deeps, st.Assets, float64(st.Bytes)/1e6, time.Since(start).Round(time.Millisecond))
	if st.Dailies == 0 {
		logf("⚠ chưa có bản tin nào trong phạm vi — chạy `fa daily` trước")
	}
	return st, nil
}

func deploySite(a *app) (string, error) {
	p := a.cfg.Publish
	w := &publish.Wrangler{Dir: a.home, Out: os.Stderr}
	if p.Target == "workers" {
		return deployWorkers(a, w)
	}
	logf("▶ Deploy lên Cloudflare Pages (project %s, nhánh %s)…", p.Project, p.Branch)
	url, err := w.Deploy(a.ctx, p.OutDir, p.Project, p.Branch, "fa publish "+time.Now().In(a.cfg.Loc).Format("2006-01-02 15:04"))
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not authenticated") || strings.Contains(msg, "wrangler login") || strings.Contains(msg, "CLOUDFLARE_API_TOKEN"):
			return "", publish.ErrNotLoggedIn
		case strings.Contains(msg, "Project not found") || strings.Contains(msg, "8000007"):
			return "", fmt.Errorf("chưa có Pages project %q — chạy `fa publish setup` trước", p.Project)
		}
		return "", err
	}
	logf("✓ Đã deploy: %s", url)
	logf("🌐 Site: %s", publicURL(a))
	return url, nil
}

// deployWorkers: Workers Static Assets — wrangler tự tạo Worker, custom domain, DNS và SSL.
func deployWorkers(a *app, w *publish.Wrangler) (string, error) {
	p := a.cfg.Publish
	cfgPath := filepath.Join(a.home, "wrangler.jsonc")
	if err := os.WriteFile(cfgPath, publish.WorkersConfig(p.Project, p.OutDir, p.Domain), 0o644); err != nil {
		return "", err
	}
	where := p.Domain
	if where == "" {
		where = "workers.dev"
	}
	logf("▶ Deploy lên Cloudflare Workers Static Assets (worker %s → %s)…", p.Project, where)
	devURL, err := w.DeployWorkers(a.ctx, cfgPath)
	if err != nil {
		if strings.Contains(err.Error(), "not authenticated") || strings.Contains(err.Error(), "wrangler login") {
			return "", publish.ErrNotLoggedIn
		}
		return "", err
	}
	site := publicURL(a)
	if p.Domain == "" && devURL != "" {
		site = devURL
	}
	logf("✓ Đã deploy · 🌐 %s", site)
	return site, nil
}

func previewSite(a *app, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	u := "http://" + ln.Addr().String()
	logf("▶ Xem thử site tĩnh: %s  (Ctrl+C để dừng — giống hệt bản sẽ deploy)", u)
	openBrowser(u)
	srv := &http.Server{Handler: http.FileServer(http.Dir(a.cfg.Publish.OutDir)), ReadHeaderTimeout: 10 * time.Second}
	go func() { <-a.ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// cfAPI: đăng nhập wrangler → token → API client + account id (theo zone của domain nếu có).
func cfAPI(a *app) (*publish.API, string, *publish.Zone, error) {
	w := &publish.Wrangler{Dir: a.home, Out: os.Stderr}
	who, err := w.Whoami(a.ctx)
	if err != nil {
		return nil, "", nil, err
	}
	tok, err := w.Token(a.ctx)
	if err != nil {
		return nil, "", nil, err
	}
	api := &publish.API{Token: tok}
	logf("✓ Cloudflare: %s (%d tài khoản)", who.Email, len(who.Accounts))
	var zone *publish.Zone
	account := ""
	if d := a.cfg.Publish.Domain; d != "" {
		zone, err = api.FindZone(a.ctx, d)
		if err != nil {
			return nil, "", nil, err
		}
		account = zone.Account.ID
		logf("✓ Zone %s (account %s…)", zone.Name, account[:min(8, len(account))])
	} else if len(who.Accounts) == 1 {
		account = who.Accounts[0].ID
	} else if len(who.Accounts) > 1 {
		return nil, "", nil, errors.New("có nhiều tài khoản Cloudflare — đặt publish.domain để chọn đúng tài khoản chứa domain")
	} else {
		return nil, "", nil, errors.New("không thấy tài khoản Cloudflare nào")
	}
	return api, account, zone, nil
}

func setupPages(a *app) error {
	p := a.cfg.Publish
	if p.Target == "workers" {
		w := &publish.Wrangler{Dir: a.home, Out: os.Stderr}
		who, err := w.Whoami(a.ctx)
		if err != nil {
			return err
		}
		logf("✓ Cloudflare: %s", who.Email)
		logf("• target=workers: không cần setup riêng — `fa publish` tự tạo Worker %q, gắn %s, tạo DNS & SSL.", p.Project, orDash(p.Domain))
		return nil
	}
	api, account, zone, err := cfAPI(a)
	if err != nil {
		return err
	}
	proj, err := api.Project(a.ctx, account, p.Project)
	if err != nil {
		return err
	}
	if proj == nil {
		if proj, err = api.CreateProject(a.ctx, account, p.Project, p.Branch); err != nil {
			return fmt.Errorf("tạo Pages project %q: %w (tên có thể đã bị dùng — đổi publish.project)", p.Project, err)
		}
		logf("✓ Tạo Pages project %s → https://%s", proj.Name, proj.Subdomain)
	} else {
		logf("✓ Pages project %s đã có → https://%s", proj.Name, proj.Subdomain)
	}
	if p.Domain == "" {
		logf("• Chưa đặt publish.domain — site sẽ ở https://%s. Tiếp theo: fa publish", proj.Subdomain)
		return nil
	}

	ds, err := api.Domains(a.ctx, account, p.Project)
	if err != nil {
		return err
	}
	found := false
	for _, d := range ds {
		found = found || strings.EqualFold(d.Name, p.Domain)
	}
	if !found {
		if _, err := api.AddDomain(a.ctx, account, p.Project, p.Domain); err != nil {
			return fmt.Errorf("gắn custom domain %s: %w", p.Domain, err)
		}
		logf("✓ Gắn custom domain %s vào project", p.Domain)
	} else {
		logf("✓ Custom domain %s đã gắn", p.Domain)
	}

	target := proj.Subdomain
	if target == "" {
		target = p.Project + ".pages.dev"
	}
	recs, err := api.DNSRecords(a.ctx, zone.ID, p.Domain)
	if err != nil {
		// token OAuth của wrangler không có quyền đọc DNS → kiểm tra qua DNS công khai
		logf("• Không đọc được bản ghi DNS qua API (%v) — kiểm tra bằng DNS công khai", err)
		dnsHint(a, zone.Name, target)
		logf("\nTiếp theo: fa publish   (build + deploy). Kiểm tra domain: fa publish status")
		return nil
	}
	switch {
	case len(recs) == 0:
		if err := api.CreateCNAME(a.ctx, zone.ID, p.Domain, target); err != nil {
			logf("⚠ Không tự tạo được bản ghi DNS (token đăng nhập wrangler không có quyền sửa DNS): %v", err)
			logf(`  → Làm 1 lần trên dashboard: Cloudflare → %s → DNS → Records → Add record
       Type: CNAME · Name: %s · Target: %s · Proxy: bật (mây cam)
     (hoặc: Workers & Pages → %s → Custom domains → bấm "Activate domain")`, zone.Name, strings.TrimSuffix(p.Domain, "."+zone.Name), target, p.Project)
		} else {
			logf("✓ Tạo CNAME %s → %s (proxied)", p.Domain, target)
		}
	default:
		r := recs[0]
		if r.Type == "CNAME" && strings.EqualFold(r.Content, target) {
			logf("✓ DNS: %s CNAME → %s", p.Domain, target)
		} else {
			logf("⚠ DNS: %s đang trỏ %s %s — cần CNAME → %s (sửa trên dashboard)", p.Domain, r.Type, r.Content, target)
		}
	}
	logf("\nTiếp theo: fa publish   (build + deploy). Kiểm tra domain: fa publish status")
	return nil
}

func pagesStatus(a *app) error {
	p := a.cfg.Publish
	if p.Target == "workers" {
		u := publicURL(a)
		c := &http.Client{Timeout: 15 * time.Second}
		resp, err := c.Get(u + "/")
		if err != nil {
			fmt.Printf("✗ %s: %v\n", u, err)
			return nil
		}
		resp.Body.Close()
		fmt.Printf("Target:    Workers Static Assets (worker %s)\nSite:      %s → HTTP %d\n", p.Project, u, resp.StatusCode)
		if p.Domain == "" {
			fmt.Println("           (chưa đặt publish.domain — site ở <worker>.<subdomain>.workers.dev)")
		}
		return nil
	}
	api, account, zone, err := cfAPI(a)
	if err != nil {
		return err
	}
	proj, err := api.Project(a.ctx, account, p.Project)
	if err != nil {
		return err
	}
	if proj == nil {
		fmt.Printf("✗ Chưa có Pages project %q — chạy `fa publish setup`\n", p.Project)
		return nil
	}
	fmt.Printf("Project:   %s → https://%s\n", proj.Name, proj.Subdomain)
	ds, err := api.Domains(a.ctx, account, p.Project)
	if err != nil {
		return err
	}
	for _, d := range ds {
		fmt.Printf("Domain:    %s — %s (xác minh: %s · SSL: %s)\n", d.Name, d.Status, orDash(d.VerificationData.Status), orDash(d.ValidationData.Status))
		if e := d.VerificationData.Error + d.ValidationData.Error; e != "" {
			fmt.Printf("           lỗi: %s\n", e)
		}
	}
	if zone != nil && p.Domain != "" {
		recs, err := api.DNSRecords(a.ctx, zone.ID, p.Domain)
		if err != nil {
			dnsHint(a, zone.Name, proj.Subdomain)
		}
		for _, r := range recs {
			fmt.Printf("DNS:       %s %s → %s (proxied=%v)\n", r.Name, r.Type, r.Content, r.Proxied)
		}
		if len(recs) == 0 {
			fmt.Printf("DNS:       chưa có bản ghi cho %s\n", p.Domain)
		}
	}
	fmt.Printf("Site:      %s\n", publicURL(a))
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// dnsHint: phân giải domain qua DNS công khai; chưa có thì in hướng dẫn thêm CNAME.
func dnsHint(a *app, zoneName, target string) {
	d := a.cfg.Publish.Domain
	if ips, err := net.LookupHost(d); err == nil && len(ips) > 0 {
		logf("✓ DNS: %s đã phân giải (%s)", d, strings.Join(ips[:min(2, len(ips))], ", "))
		return
	}
	logf(`⚠ DNS: %s chưa phân giải. Thêm 1 lần trên dashboard Cloudflare:
     %s → DNS → Records → Add record · Type CNAME · Name %s · Target %s · Proxy bật (mây cam)
     (hoặc Workers & Pages → %s → Custom domains → "Activate domain")`,
		d, zoneName, strings.TrimSuffix(d, "."+zoneName), target, a.cfg.Publish.Project)
}
