package llm

import (
	"slices"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```":   `{"a":1}`,
		"Đây là kết quả: {\"a\":1}": `{"a":1}`,
		`{"a":1}`:                   `{"a":1}`,
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q)=%q", in, got)
		}
	}
}

func TestArgs(t *testing.T) {
	r := &Runner{}
	a, err := r.args(Request{Model: "haiku", System: "s", Schema: map[string]any{"type": "object"}, Isolated: true}, "json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-p", "--no-session-persistence", "--json-schema", "--strict-mcp-config", "--system-prompt"} {
		if !slices.Contains(a, want) {
			t.Errorf("thiếu %s trong %v", want, a)
		}
	}
	i := slices.Index(a, "--tools")
	if i < 0 || a[i+1] != "" {
		t.Errorf("không có tool phải truyền --tools \"\": %v", a)
	}
	a, _ = r.args(Request{Tools: []string{"WebSearch", "WebFetch"}}, "stream-json")
	i = slices.Index(a, "--allowedTools")
	if i < 0 || a[i+1] != "WebSearch,WebFetch" {
		t.Errorf("allowedTools sai: %v", a)
	}
}
