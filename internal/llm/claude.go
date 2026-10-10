// Package llm gọi Claude Code CLI (`claude -p`) cho phân loại, phân tích và hỏi đáp.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner struct {
	Bin     string
	Dir     string // working dir cho claude (FA_HOME)
	Timeout time.Duration
	// Log được gọi sau mỗi lần gọi (để ghi chi phí). Có thể nil.
	Log func(purpose, model string, cost float64, dur time.Duration, err error)
}

type Request struct {
	Purpose      string // nhãn ghi log: classify, analyze:economy, synthesis, deep...
	Model        string
	System       string // thay system prompt mặc định (rẻ, gọn). Rỗng = dùng mặc định của Claude Code.
	AppendSystem string // nối vào system prompt mặc định (dùng khi cần tool như WebSearch)
	Prompt       string // gửi qua stdin
	Schema       any    // JSON schema → structured output
	Tools        []string
	Effort       string
	// Isolated=true: không nạp settings/hook/MCP/CLAUDE.md của người dùng — ổn định cho batch.
	Isolated bool
}

type Meta struct {
	CostUSD  float64
	Duration time.Duration
	Turns    int
}

type cliResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	NumTurns         int             `json:"num_turns"`
	DurationMS       int64           `json:"duration_ms"`
}

func (r *Runner) args(req Request, format string) ([]string, error) {
	a := []string{"-p", "--output-format", format, "--no-session-persistence"}
	if req.Model != "" {
		a = append(a, "--model", req.Model)
	}
	if req.Isolated {
		a = append(a, "--setting-sources", "", "--strict-mcp-config")
	}
	if req.System != "" {
		a = append(a, "--system-prompt", req.System)
	}
	if req.AppendSystem != "" {
		a = append(a, "--append-system-prompt", req.AppendSystem)
	}
	if req.Effort != "" {
		a = append(a, "--effort", req.Effort)
	}
	if len(req.Tools) == 0 {
		a = append(a, "--tools", "")
	} else {
		a = append(a, "--tools", strings.Join(req.Tools, ","), "--allowedTools", strings.Join(req.Tools, ","))
	}
	if req.Schema != nil {
		b, err := json.Marshal(req.Schema)
		if err != nil {
			return nil, err
		}
		a = append(a, "--json-schema", string(b))
	}
	return a, nil
}

func (r *Runner) command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = r.Dir
	// Tránh claude con kế thừa trạng thái "đang chạy trong Claude Code" của tiến trình cha.
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDECODE=") || strings.HasPrefix(e, "CLAUDE_CODE_ENTRYPOINT=") {
			continue
		}
		filtered = append(filtered, e)
	}
	cmd.Env = filtered
	return cmd
}

// JSON chạy claude với structured output và giải mã vào out. Thử lại 1 lần nếu lỗi.
func (r *Runner) JSON(ctx context.Context, req Request, out any) (Meta, error) {
	var lastErr error
	var total Meta
	for attempt := 0; attempt < 2; attempt++ {
		m, err := r.jsonOnce(ctx, req, out)
		total.CostUSD += m.CostUSD
		total.Duration += m.Duration
		total.Turns += m.Turns
		if err == nil {
			return total, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return total, lastErr
}

func (r *Runner) jsonOnce(ctx context.Context, req Request, out any) (Meta, error) {
	args, err := r.args(req, "json")
	if err != nil {
		return Meta{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := r.command(ctx, args)
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	meta := Meta{Duration: time.Since(start)}

	var res cliResult
	decErr := json.Unmarshal(lastJSONLine(stdout.Bytes()), &res)
	meta.CostUSD, meta.Turns = res.TotalCostUSD, res.NumTurns

	err = func() error {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("claude timeout sau %s", r.Timeout)
		}
		if decErr != nil {
			return fmt.Errorf("claude: không đọc được output (%v): %s %s", runErr, trim(stdout.String(), 300), trim(stderr.String(), 300))
		}
		if res.IsError || res.Subtype != "success" {
			return fmt.Errorf("claude lỗi (%s): %s %s", res.Subtype, trim(res.Result, 300), trim(stderr.String(), 200))
		}
		payload := res.StructuredOutput
		if len(payload) == 0 || string(payload) == "null" {
			payload = []byte(extractJSON(res.Result))
		}
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("claude: JSON không hợp lệ: %w — %s", err, trim(string(payload), 300))
		}
		return nil
	}()
	if r.Log != nil {
		r.Log(req.Purpose, req.Model, meta.CostUSD, meta.Duration, err)
	}
	return meta, err
}

// Stream chạy claude dạng text, ghi kết quả ra w theo thời gian thực và trả về toàn văn.
func (r *Runner) Stream(ctx context.Context, req Request, w io.Writer) (string, error) {
	args, err := r.args(req, "text")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := r.command(ctx, args)
	cmd.Stdin = strings.NewReader(req.Prompt)
	var buf, stderr bytes.Buffer
	if w != nil {
		cmd.Stdout = io.MultiWriter(&buf, w)
	} else {
		cmd.Stdout = &buf
	}
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("claude: %w %s", err, trim(stderr.String(), 400))
	}
	if r.Log != nil {
		r.Log(req.Purpose, req.Model, 0, time.Since(start), err)
	}
	return buf.String(), err
}

// Event là sự kiện tiến trình khi chạy agent: tool đang dùng, đoạn text trung gian.
type Event struct {
	Kind string // tool | text
	Name string // tên tool
	Text string // input tóm tắt hoặc text
}

// Agent chạy claude ở chế độ stream-json (có tool), gọi onEvent cho từng bước, trả về kết quả cuối.
func (r *Runner) Agent(ctx context.Context, req Request, onEvent func(Event)) (string, Meta, error) {
	args, err := r.args(req, "stream-json")
	if err != nil {
		return "", Meta{}, err
	}
	args = append(args, "--verbose")
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := r.command(ctx, args)
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", Meta{}, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return "", Meta{}, err
	}
	var final cliResult
	gotFinal := false
	dec := json.NewDecoder(pipe)
	for {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
			cliResult
		}
		if err := dec.Decode(&ev); err != nil {
			break
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				if onEvent == nil {
					continue
				}
				switch c.Type {
				case "tool_use":
					onEvent(Event{Kind: "tool", Name: c.Name, Text: toolInputSummary(c.Input)})
				case "text":
					onEvent(Event{Kind: "text", Text: c.Text})
				}
			}
		case "result":
			final, gotFinal = ev.cliResult, true
		}
	}
	waitErr := cmd.Wait()
	meta := Meta{Duration: time.Since(start), CostUSD: final.TotalCostUSD, Turns: final.NumTurns}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		err = fmt.Errorf("claude timeout sau %s", r.Timeout)
	case !gotFinal:
		err = fmt.Errorf("claude không trả kết quả (%v): %s", waitErr, trim(stderr.String(), 400))
	case final.IsError || final.Subtype != "success":
		err = fmt.Errorf("claude lỗi (%s): %s", final.Subtype, trim(final.Result, 300))
	}
	if r.Log != nil {
		r.Log(req.Purpose, req.Model, meta.CostUSD, meta.Duration, err)
	}
	return final.Result, meta, err
}

func toolInputSummary(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, k := range []string{"query", "url", "command", "file_path", "pattern", "prompt"} {
		if v, ok := m[k].(string); ok {
			return trim(v, 120)
		}
	}
	return ""
}

// Interactive mở phiên claude tương tác (kế thừa terminal).
func (r *Runner) Interactive(ctx context.Context, args ...string) error {
	cmd := r.command(ctx, args)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (r *Runner) Check(ctx context.Context) (string, error) {
	path, err := exec.LookPath(r.Bin)
	if err != nil {
		return "", errors.New("không tìm thấy Claude CLI — cài: npm i -g @anthropic-ai/claude-code, hoặc đặt claude.bin trong config")
	}
	out, err := r.command(ctx, []string{"--version"}).Output()
	return path + " " + strings.TrimSpace(string(out)), err
}

func lastJSONLine(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 && !bytes.HasPrefix(b, []byte("{")) {
		return b[i+1:]
	}
	return b
}

// extractJSON lấy khối JSON đầu tiên trong text (phòng khi model bọc trong ```json).
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	if i := strings.IndexAny(s, "{["); i >= 0 {
		return s[i:]
	}
	return s
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
