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

// Publish: xuất bản site tĩnh công khai lên Cloudflare Pages (fa publish).
type Publish struct {
	Target      string   `yaml:"target"`      // workers (mặc định: Workers Static Assets, tự gắn domain + DNS) | pages
	Project     string   `yaml:"project"`     // tên Worker / Pages project
	Domain      string   `yaml:"domain"`      // custom domain, vd blog.example.com
	SiteTitle   string   `yaml:"site_title"`  //
	Description string   `yaml:"description"` //
	Days        int      `yaml:"days"`        // xuất bản bản tin N ngày gần nhất (0 = tất cả)
	OutDir      string   `yaml:"out_dir"`     // thư mục build, tương đối với home
	Branch      string   `yaml:"branch"`      // nhánh production của Pages project
	AfterDaily  bool     `yaml:"after_daily"` // fa daily tự publish khi xong
	Exclude     []string `yaml:"exclude"`     // slug báo cáo deep / storyline không xuất bản
	// IncludeSecurity: đưa bản đồ mối đe doạ (/security) lên site công khai. Mặc định không — trang có mục
	// "Stack của bạn" + playbook theo công nghệ dự án, công khai là lộ bề mặt tấn công của chính mình.
	IncludeSecurity bool `yaml:"include_security"`
}

type Web struct {
	Addr string `yaml:"addr"`
}

// Security: bản đồ mối đe doạ an ninh mạng (fa sec) — trích xuất tình báo từ tin security,
// CISA KEV, EPSS, hoạt động nhóm ransomware.
type Security struct {
	Enabled      bool   `yaml:"enabled"`
	Topic        string `yaml:"topic"`         // topic id dùng cho tin an ninh mạng
	ExtractModel string `yaml:"extract_model"` // trích xuất sự cố/kỹ thuật/CVE từ bài
	BriefModel   string `yaml:"brief_model"`   // tổng hợp xu hướng + playbook phòng thủ
	BatchSize    int    `yaml:"batch_size"`    // số bài mỗi lần gọi trích xuất
	FullText     bool   `yaml:"fulltext"`      // tải nội dung bài gốc để trích xuất chi tiết kỹ thuật
	TextChars    int    `yaml:"text_chars"`    // độ dài tối đa nội dung bài gửi cho Claude
	KEVURL       string `yaml:"kev_url"`       // CISA Known Exploited Vulnerabilities (JSON)
	EPSS         bool   `yaml:"epss"`          // lấy điểm EPSS (FIRST) cho CVE gần đây
	RansomURL    string `yaml:"ransom_url"`    // RansomLook: bài đăng mới trên leak site; rỗng = tắt
	BackfillDays int    `yaml:"backfill_days"` // fa sec backfill: lấy tin security cũ tới N ngày
	// Stack: công nghệ trong các dự án của người dùng — đánh dấu sự cố/CVE chạm tới stack, cá nhân hoá playbook.
	Stack []StackItem `yaml:"stack"`
}

// StackItem: 1 công nghệ + các từ khoá nhận diện trong tin/CVE (không phân biệt hoa thường, khớp theo từ).
type StackItem struct {
	Name  string   `yaml:"name"`
	Match []string `yaml:"match"`
}

type Config struct {
	Profile  string   `yaml:"profile"`
	Timezone string   `yaml:"timezone"`
	Topics   []Topic  `yaml:"topics"`
	Feeds    []Feed   `yaml:"feeds"`
	Claude   Claude   `yaml:"claude"`
	Fetch    Fetch    `yaml:"fetch"`
	Analyze  Analyze  `yaml:"analyze"`
	Report   Report   `yaml:"report"`
	Web      Web      `yaml:"web"`
	Publish  Publish  `yaml:"publish"`
	Security Security `yaml:"security"`

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
	defs(&c.Publish.Target, "workers")
	defs(&c.Publish.Project, "fa-blog")
	defs(&c.Publish.SiteTitle, "Bản tin vi mô ↔ vĩ mô")
	defs(&c.Publish.Description, "Bản tin AI agent, kinh tế, chính trị — Việt Nam & quốc tế — nối sự kiện vi mô với xu hướng vĩ mô, tạo tự động mỗi ngày.")
	defs(&c.Publish.OutDir, "site")
	defs(&c.Publish.Branch, "main")
	defs(&c.Security.Topic, "security")
	defs(&c.Security.ExtractModel, "sonnet")
	defs(&c.Security.BriefModel, c.Claude.SynthesisModel)
	def(&c.Security.BatchSize, 8)
	def(&c.Security.TextChars, 3500)
	def(&c.Security.BackfillDays, 30)
	defs(&c.Security.KEVURL, "https://raw.githubusercontent.com/cisagov/kev-data/develop/known_exploited_vulnerabilities.json")
	if !filepath.IsAbs(c.Publish.OutDir) {
		c.Publish.OutDir = filepath.Join(c.Home, c.Publish.OutDir)
	}
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

// SecurityOn: bật bản đồ mối đe doạ và topic security có trong config.
func (c *Config) SecurityOn() bool {
	_, ok := c.Topic(c.Security.Topic)
	return c.Security.Enabled && ok
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
