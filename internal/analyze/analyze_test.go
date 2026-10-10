package analyze

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Fed Rate Path":                  "fed-rate-path",
		"Lãi suất NHNN – tháng 10":       "lai-suat-nhnn-thang-10",
		"  Đường sắt cao tốc Bắc–Nam!! ": "duong-sat-cao-toc-bac-nam",
		"already-ok-slug":                "already-ok-slug",
		"":                               "",
		strings.Repeat("abc ", 30):       "abc-abc-abc-abc-abc-abc-abc-abc-abc-abc-abc-abc-abc-abc-abc",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q)=%q want %q", in, got, want)
		}
	}
}

func TestChainString(t *testing.T) {
	got := ChainString([]ChainStep{
		{Level: "micro", Step: "Nvidia bị hạn chế xuất khẩu"},
		{Level: "meso", Step: "chi phí GPU tăng", Channel: "thương mại"},
		{Level: "macro", Step: "FDI bán dẫn dịch chuyển"},
	})
	want := "[vi mô] Nvidia bị hạn chế xuất khẩu —(thương mại)→ [trung mô] chi phí GPU tăng → [vĩ mô] FDI bán dẫn dịch chuyển"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestSchemasAreValidJSON(t *testing.T) {
	for name, s := range map[string]any{"topic": topicSchema, "synthesis": synthesisSchema} {
		b, err := json.Marshal(s)
		if err != nil || len(b) < 100 {
			t.Fatalf("%s schema: %v", name, err)
		}
	}
}
