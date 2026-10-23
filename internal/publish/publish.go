// Package publish: build site tĩnh và đưa lên Cloudflare Pages (gói free) — deploy qua wrangler,
// cấu hình project/custom domain qua Cloudflare API (dùng token đăng nhập của wrangler).
package publish

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Wrangler chạy Cloudflare wrangler CLI: dùng bản cài sẵn nếu có, không thì `npx --yes wrangler@4`.
type Wrangler struct {
	Dir string
	Out io.Writer // nơi in log của wrangler (stderr)
}

func (w *Wrangler) cmd(ctx context.Context, args ...string) *exec.Cmd {
	var c *exec.Cmd
	if p, err := exec.LookPath("wrangler"); err == nil {
		c = exec.CommandContext(ctx, p, args...)
	} else {
		c = exec.CommandContext(ctx, "npx", append([]string{"--yes", "wrangler@4"}, args...)...)
	}
	c.Dir = w.Dir
	c.Env = append(os.Environ(), "WRANGLER_SEND_METRICS=false", "CI=1")
	return c
}

func (w *Wrangler) output(ctx context.Context, args ...string) ([]byte, error) {
	c := w.cmd(ctx, args...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return out, fmt.Errorf("wrangler %s: %v %s", strings.Join(args, " "), err, lastLines(stderr.String()+string(out), 6))
	}
	return out, nil
}

var ErrNotLoggedIn = errors.New("chưa đăng nhập Cloudflare — chạy: npx wrangler login (mở trình duyệt để cấp quyền), rồi chạy lại")

type Whoami struct {
	Email    string `json:"email"`
	Accounts []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"accounts"`
}

func (w *Wrangler) Whoami(ctx context.Context) (*Whoami, error) {
	out, err := w.output(ctx, "whoami", "--json")
	if err != nil {
		if bytes.Contains(out, []byte("not authenticated")) || strings.Contains(err.Error(), "not authenticated") || strings.Contains(err.Error(), "exit status") {
			return nil, ErrNotLoggedIn
		}
		return nil, err
	}
	var who Whoami
	if err := json.Unmarshal(jsonPart(out), &who); err != nil {
		return nil, fmt.Errorf("whoami: %w", err)
	}
	return &who, nil
}

// Token lấy token API hiện hành của wrangler (OAuth sau `wrangler login`, hoặc CLOUDFLARE_API_TOKEN).
func (w *Wrangler) Token(ctx context.Context) (string, error) {
	if t := os.Getenv("CLOUDFLARE_API_TOKEN"); t != "" {
		return t, nil
	}
	out, err := w.output(ctx, "auth", "token", "--json")
	if err != nil {
		return "", ErrNotLoggedIn
	}
	var m map[string]any
	if err := json.Unmarshal(jsonPart(out), &m); err != nil {
		return "", fmt.Errorf("auth token: %w", err)
	}
	for _, k := range []string{"token", "apiToken", "oauthToken", "value"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", errors.New("không đọc được token từ `wrangler auth token --json`")
}

var deployURLRe = regexp.MustCompile(`https://[a-z0-9.-]+\.pages\.dev\S*`)

// Deploy đẩy thư mục site lên Cloudflare Pages; trả về URL deployment.
func (w *Wrangler) Deploy(ctx context.Context, dir, project, branch, message string) (string, error) {
	return w.stream(ctx, deployURLRe, "pages", "deploy", dir, "--project-name", project, "--branch", branch,
		"--commit-message", message, "--commit-dirty=true")
}

// WorkersConfig sinh wrangler.jsonc cho Worker chỉ phục vụ static assets (không có script → không tốn request Worker).
func WorkersConfig(name, assetsDir, domain string) []byte {
	cfg := map[string]any{
		"name":               name,
		"compatibility_date": "2025-09-01",
		"assets": map[string]any{
			"directory":          assetsDir,
			"not_found_handling": "404-page",
			"html_handling":      "auto-trailing-slash",
		},
		"observability": map[string]any{"enabled": false},
	}
	if domain != "" {
		cfg["routes"] = []map[string]any{{"pattern": domain, "custom_domain": true}}
		cfg["workers_dev"] = false
	} else {
		cfg["workers_dev"] = true
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

var workersDevRe = regexp.MustCompile(`https://[a-z0-9.-]+\.workers\.dev`)

// DeployWorkers chạy `wrangler deploy --config <file>`; trả về URL workers.dev nếu có.
func (w *Wrangler) DeployWorkers(ctx context.Context, configPath string) (string, error) {
	return w.stream(ctx, workersDevRe, "deploy", "--config", configPath)
}

// stream chạy wrangler, in log (bỏ danh sách file upload), bắt URL khớp re.
func (w *Wrangler) stream(ctx context.Context, re *regexp.Regexp, args ...string) (string, error) {
	c := w.cmd(ctx, args...)
	pr, pw := io.Pipe()
	c.Stdout, c.Stderr = pw, pw
	if err := c.Start(); err != nil {
		return "", err
	}
	var url string
	var tail []string
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "+ /") {
				continue
			}
			if w.Out != nil {
				fmt.Fprintln(w.Out, "  │ "+line)
			}
			if m := re.FindString(line); m != "" {
				url = m
			}
			tail = append(tail, line)
			if len(tail) > 10 {
				tail = tail[1:]
			}
		}
		close(done)
	}()
	err := c.Wait()
	pw.Close()
	<-done
	if err != nil {
		return "", fmt.Errorf("wrangler %s lỗi: %v\n%s", args[0], err, strings.Join(tail, "\n"))
	}
	return url, nil
}

// ---------- Cloudflare API ----------

type API struct {
	Token  string
	Client *http.Client
}

type apiResp struct {
	Success bool            `json:"success"`
	Errors  []apiErr        `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type apiErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type APIError struct {
	Status int
	Errs   []apiErr
}

func (e *APIError) Error() string {
	var parts []string
	for _, x := range e.Errs {
		parts = append(parts, fmt.Sprintf("%d %s", x.Code, x.Message))
	}
	return fmt.Sprintf("Cloudflare API HTTP %d: %s", e.Status, strings.Join(parts, "; "))
}

func (a *API) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Content-Type", "application/json")
	c := a.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("Cloudflare API %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if !r.Success {
		return &APIError{Status: resp.StatusCode, Errs: r.Errors}
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

type Zone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Account struct {
		ID string `json:"id"`
	} `json:"account"`
}

// FindZone tìm zone chứa domain (thử dần các hậu tố: a.b.example.org → b.example.org → example.org).
func (a *API) FindZone(ctx context.Context, domain string) (*Zone, error) {
	labels := strings.Split(strings.Trim(domain, "."), ".")
	for i := 0; i+2 <= len(labels); i++ {
		name := strings.Join(labels[i:], ".")
		var zs []Zone
		if err := a.do(ctx, http.MethodGet, "/zones?name="+name, nil, &zs); err != nil {
			return nil, err
		}
		if len(zs) > 0 {
			return &zs[0], nil
		}
	}
	return nil, fmt.Errorf("không thấy zone nào trên tài khoản Cloudflare chứa %s", domain)
}

type Project struct {
	Name      string   `json:"name"`
	Subdomain string   `json:"subdomain"`
	Domains   []string `json:"domains"`
}

func (a *API) Project(ctx context.Context, account, name string) (*Project, error) {
	var p Project
	err := a.do(ctx, http.MethodGet, "/accounts/"+account+"/pages/projects/"+name, nil, &p)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *API) CreateProject(ctx context.Context, account, name, branch string) (*Project, error) {
	var p Project
	err := a.do(ctx, http.MethodPost, "/accounts/"+account+"/pages/projects",
		map[string]any{"name": name, "production_branch": branch}, &p)
	return &p, err
}

type Domain struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	VerificationData struct {
		Status string `json:"status"`
		Error  string `json:"error_message"`
	} `json:"verification_data"`
	ValidationData struct {
		Status string `json:"status"`
		Method string `json:"method"`
		Error  string `json:"error_message"`
	} `json:"validation_data"`
}

func (a *API) Domains(ctx context.Context, account, project string) ([]Domain, error) {
	var ds []Domain
	err := a.do(ctx, http.MethodGet, "/accounts/"+account+"/pages/projects/"+project+"/domains", nil, &ds)
	return ds, err
}

func (a *API) AddDomain(ctx context.Context, account, project, domain string) (*Domain, error) {
	var d Domain
	err := a.do(ctx, http.MethodPost, "/accounts/"+account+"/pages/projects/"+project+"/domains", map[string]any{"name": domain}, &d)
	return &d, err
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

func (a *API) DNSRecords(ctx context.Context, zone, name string) ([]DNSRecord, error) {
	var rs []DNSRecord
	err := a.do(ctx, http.MethodGet, "/zones/"+zone+"/dns_records?name="+name, nil, &rs)
	return rs, err
}

func (a *API) CreateCNAME(ctx context.Context, zone, name, target string) error {
	return a.do(ctx, http.MethodPost, "/zones/"+zone+"/dns_records",
		map[string]any{"type": "CNAME", "name": name, "content": target, "proxied": true, "comment": "fa publish → Cloudflare Pages"}, nil)
}

// ---------- helpers ----------

// jsonPart bỏ phần log trước JSON (wrangler đôi khi in banner).
func jsonPart(b []byte) []byte {
	if i := bytes.IndexAny(b, "{["); i > 0 {
		return b[i:]
	}
	return b
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
