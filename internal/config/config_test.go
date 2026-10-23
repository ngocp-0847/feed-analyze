package config

import (
	"strings"
	"testing"
)

func TestDefaultConfigParses(t *testing.T) {
	c, err := Parse(DefaultYAML, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Topics) != 4 || len(c.ActiveFeeds()) < 40 {
		t.Fatalf("topics=%d feeds=%d", len(c.Topics), len(c.ActiveFeeds()))
	}
	if !c.SecurityOn() || c.Security.ExtractModel == "" || c.Security.KEVURL == "" || c.Security.BatchSize <= 0 {
		t.Fatalf("security mặc định: %+v", c.Security)
	}
	vn, intl := 0, 0
	for _, f := range c.Feeds {
		switch f.Region {
		case "vn":
			vn++
		case "intl":
			intl++
		}
		for _, tp := range f.Topics {
			if _, ok := c.Topic(tp); !ok {
				t.Errorf("feed %s gợi ý topic không tồn tại: %s", f.Name, tp)
			}
		}
	}
	if vn < 10 || intl < 20 {
		t.Fatalf("vn=%d intl=%d", vn, intl)
	}
	if c.Loc == nil || c.Claude.ClassifyModel != "haiku" || !strings.HasSuffix(c.Report.Dir, "reports") {
		t.Fatalf("defaults sai: %+v", c.Claude)
	}
}

func TestValidate(t *testing.T) {
	bad := []string{
		"topics: []",
		"topics: [{id: 'a b'}]",
		"topics: [{id: a}, {id: a}]",
		"topics: [{id: a}]\nfeeds: [{name: x}]",
	}
	for _, y := range bad {
		if _, err := Parse([]byte(y), t.TempDir()); err == nil {
			t.Errorf("phải lỗi: %q", y)
		}
	}
}
