// Package config nạp cấu hình YAML: chủ đề quan tâm, nguồn RSS, model Claude.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var DefaultYAML []byte

type Topic struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Emoji       string   `yaml:"emoji"`
	Description string   `yaml:"description"`
	Keywords    []string `yaml:"keywords"`
}

type Feed struct {
	Name   string   `yaml:"name"`
	URL    string   `yaml:"url"`
	Region string   `yaml:"region"` // vn | intl
	Lang   string   `yaml:"lang"`   // vi | en
	Topics []string `yaml:"topics"` // gợi ý chủ đề (không bắt buộc)
	Off    bool     `yaml:"disabled"`
}

type Claude struct {
	Bin             string  `yaml:"bin"`
	ClassifyModel   string  `yaml:"classify_model"`
	AnalyzeModel    string  `yaml:"analyze_model"`
	SynthesisModel  string  `yaml:"synthesis_model"`
	DeepModel       string  `yaml:"deep_model"`
	BatchSize       int     `yaml:"batch_size"`
	Concurrency     int     `yaml:"concurrency"`
	TimeoutMinutes  int     `yaml:"timeout_minutes"`
	MaxBudgetPerRun float64 `yaml:"max_budget_usd_per_call"`
}

type Fetch struct {
	MaxAgeHours     int `yaml:"max_age_hours"`
	MaxItemsPerFeed int `yaml:"max_items_per_feed"`
	Concurrency     int `yaml:"concurrency"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
}

type Analyze struct {
	WindowHours      int     `yaml:"window_hours"`
	MinRelevance     float64 `yaml:"min_relevance"`
	MaxArticles      int     `yaml:"max_articles_per_topic"`
	StorylineMemDays int     `yaml:"storyline_memory_days"`
}

type Report struct {
	Dir    string   `yaml:"dir"`
	CopyTo []string `yaml:"copy_to"`
}

type Web struct {
	Addr string `yaml:"addr"`
}

type Config struct {
	Profile  string  `yaml:"profile"`
	Timezone string  `yaml:"timezone"`
	Topics   []Topic `yaml:"topics"`
	Feeds    []Feed  `yaml:"feeds"`
	Claude   Claude  `yaml:"claude"`
	Fetch    Fetch   `yaml:"fetch"`
	Analyze  Analyze `yaml:"analyze"`
	Report   Report  `yaml:"report"`
	Web      Web     `yaml:"web"`

	Home string         `yaml:"-"`
	Loc  *time.Location `yaml:"-"`
}

// HomeDir trả về thư mục dữ liệu: --home > FA_HOME > ~/.feed-analyze.
func HomeDir(flagHome string) string {
	if flagHome != "" {
		return flagHome
	}
	if h := os.Getenv("FA_HOME"); h != "" {
		return h
	}
	uh, err := os.UserHomeDir()
	if err != nil {
		return ".feed-analyze"
	}
	return filepath.Join(uh, ".feed-analyze")
}

func Path(home string) string { return filepath.Join(home, "config.yaml") }

var ErrNotInit = errors.New("chưa khởi tạo — chạy `fa init` trước")

func Load(home string) (*Config, error) {
	b, err := os.ReadFile(Path(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotInit
	}
	if err != nil {
		return nil, err
	}
	return Parse(b, home)
}

func Parse(b []byte, home string) (*Config, error) {
	c := &Config{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("config.yaml lỗi cú pháp: %w", err)
	}
	c.Home = home
	c.applyDefaults()
	return c, c.validate()
}

func (c *Config) applyDefaults() {
	if c.Timezone == "" {
		c.Timezone = "Asia/Ho_Chi_Minh"
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	c.Loc = loc
	def := func(p *int, v int) {
		if *p <= 0 {
			*p = v
		}
	}
	defs := func(p *string, v string) {
		if *p == "" {
			*p = v
		}
	}
	defs(&c.Claude.Bin, "claude")
	defs(&c.Claude.ClassifyModel, "haiku")
	defs(&c.Claude.AnalyzeModel, "sonnet")
	defs(&c.Claude.SynthesisModel, "opus")
	defs(&c.Claude.DeepModel, "opus")
	def(&c.Claude.BatchSize, 30)
	def(&c.Claude.Concurrency, 4)
	def(&c.Claude.TimeoutMinutes, 10)
	def(&c.Fetch.MaxAgeHours, 48)
	def(&c.Fetch.MaxItemsPerFeed, 60)
	def(&c.Fetch.Concurrency, 8)
	def(&c.Fetch.TimeoutSeconds, 25)
	def(&c.Analyze.WindowHours, 30)
	def(&c.Analyze.MaxArticles, 100)
	def(&c.Analyze.StorylineMemDays, 10)
	if c.Analyze.MinRelevance <= 0 {
		c.Analyze.MinRelevance = 0.5
	}
	defs(&c.Report.Dir, "reports")
	defs(&c.Web.Addr, "127.0.0.1:8787")
	if !filepath.IsAbs(c.Report.Dir) {
		c.Report.Dir = filepath.Join(c.Home, c.Report.Dir)
	}
	for i := range c.Feeds {
		f := &c.Feeds[i]
		defs(&f.Region, "intl")
		if f.Lang == "" {
			if f.Region == "vn" {
				f.Lang = "vi"
			} else {
				f.Lang = "en"
			}
		}
		defs(&f.Name, f.URL)
	}
}

func (c *Config) validate() error {
	if len(c.Topics) == 0 {
		return errors.New("config: cần ít nhất 1 topic")
	}
	seen := map[string]bool{}
	for _, t := range c.Topics {
		if t.ID == "" || strings.ContainsAny(t.ID, " \t") {
			return fmt.Errorf("config: topic id %q không hợp lệ (không rỗng, không khoảng trắng)", t.ID)
		}
		if seen[t.ID] {
			return fmt.Errorf("config: topic id %q bị trùng", t.ID)
		}
		seen[t.ID] = true
	}
	names := map[string]bool{}
	for _, f := range c.Feeds {
		if f.URL == "" {
			return fmt.Errorf("config: feed %q thiếu url", f.Name)
		}
		if names[f.Name] {
			return fmt.Errorf("config: feed name %q bị trùng", f.Name)
		}
		names[f.Name] = true
	}
	return nil
}

func (c *Config) Topic(id string) (Topic, bool) {
	for _, t := range c.Topics {
		if t.ID == id {
			return t, true
		}
	}
	return Topic{}, false
}

func (c *Config) TopicIDs() []string {
	ids := make([]string, len(c.Topics))
	for i, t := range c.Topics {
		ids[i] = t.ID
	}
	return ids
}

func (c *Config) ActiveFeeds() []Feed {
	var out []Feed
	for _, f := range c.Feeds {
		if !f.Off {
			out = append(out, f)
		}
	}
	return out
}

// Today trả về ngày hiện tại theo timezone cấu hình, dạng YYYY-MM-DD.
func (c *Config) Today() string { return time.Now().In(c.Loc).Format("2006-01-02") }

func (c *Config) DBPath() string { return filepath.Join(c.Home, "feed.db") }
