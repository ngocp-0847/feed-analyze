package analyze

// Các kiểu dữ liệu đầu ra của Claude (khớp JSON schema bên dưới).

type ChainStep struct {
	Level   string `json:"level"`   // micro | meso | macro
	Step    string `json:"step"`    // mô tả mắt xích
	Channel string `json:"channel"` // kênh truyền dẫn tới mắt xích này (rỗng với mắt xích đầu)
}

type StorylineOut struct {
	Slug         string      `json:"slug"`
	Title        string      `json:"title"`
	Status       string      `json:"status"` // new | developing | escalating | cooling | resolved
	Level        string      `json:"level"`
	Importance   int         `json:"importance"`
	Summary      string      `json:"summary"`
	WhatChanged  string      `json:"what_changed"`
	WhyItMatters string      `json:"why_it_matters"`
	CausalChain  []ChainStep `json:"causal_chain"`
	VNAngle      string      `json:"vn_angle"`
	Stakeholders []string    `json:"stakeholders"`
	WatchNext    []string    `json:"watch_next"`
	Confidence   string      `json:"confidence"`
	ArticleIDs   []int64     `json:"article_ids"`
}

type TopicAnalysis struct {
	Topic      string         `json:"topic"`
	Headline   string         `json:"headline"`
	Storylines []StorylineOut `json:"storylines"`
	Signals    []string       `json:"signals"`
	Questions  []string       `json:"questions"`
	// điền bởi Go
	ArticleCount int `json:"article_count"`
}

type CrossLink struct {
	FromTopic   string `json:"from_topic"`
	FromStory   string `json:"from_story"`
	ToTopic     string `json:"to_topic"`
	ToStory     string `json:"to_story"`
	Mechanism   string `json:"mechanism"`
	Implication string `json:"implication"`
}

type VNImpact struct {
	Area       string `json:"area"`
	Impact     string `json:"impact"`
	Direction  string `json:"direction"` // positive | negative | mixed | uncertain
	Horizon    string `json:"horizon"`   // short | medium | long
	Confidence string `json:"confidence"`
}

type WatchItem struct {
	Item string `json:"item"`
	Why  string `json:"why"`
	When string `json:"when"`
}

type DeepDive struct {
	Slug     string `json:"slug"`
	Question string `json:"question"`
}

type Synthesis struct {
	Title        string      `json:"title"`
	TLDR         []string    `json:"tldr"`
	MacroPicture string      `json:"macro_picture"`
	CrossLinks   []CrossLink `json:"cross_links"`
	VNImpact     []VNImpact  `json:"vn_impact"`
	ForYou       []string    `json:"for_you"`
	Watchlist    []WatchItem `json:"watchlist"`
	DeepDives    []DeepDive  `json:"deep_dives"`
	Contrarian   string      `json:"contrarian"`
}

// Daily gom toàn bộ bản tin 1 ngày (lưu JSON trong reports kind=daily).
type Daily struct {
	Date      string          `json:"date"`
	Synthesis *Synthesis      `json:"synthesis"`
	Topics    []TopicAnalysis `json:"topics"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func strEnum(vals ...string) map[string]any {
	return map[string]any{"type": "string", "enum": vals}
}
func arr(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
func obj(props map[string]any, req ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": req}
}

var levelEnum = strEnum("micro", "meso", "macro")

var topicSchema = obj(map[string]any{
	"headline": str("1 câu: điều quan trọng nhất của chủ đề hôm nay"),
	"storylines": arr(obj(map[string]any{
		"slug":           str("kebab-case không dấu, tiếng Anh, ổn định qua các ngày; dùng lại slug cũ nếu là cùng câu chuyện"),
		"title":          str("tiêu đề tiếng Việt ngắn gọn"),
		"status":         strEnum("new", "developing", "escalating", "cooling", "resolved"),
		"level":          levelEnum,
		"importance":     map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
		"summary":        str("2–4 câu: chuyện gì đang xảy ra, có số liệu"),
		"what_changed":   str("điều mới hôm nay so với trước (nếu là storyline cũ)"),
		"why_it_matters": str("vì sao quan trọng — tác động bậc 2"),
		"causal_chain": arr(obj(map[string]any{
			"level": levelEnum, "step": str("mắt xích"), "channel": str("kênh truyền dẫn dẫn tới mắt xích này"),
		}, "level", "step", "channel")),
		"vn_angle":     str("liên hệ/tác động tới Việt Nam; rỗng nếu không có"),
		"stakeholders": arr(str("bên liên quan")),
		"watch_next":   arr(str("chỉ báo/sự kiện cần theo dõi")),
		"confidence":   strEnum("low", "medium", "high"),
		"article_ids":  arr(map[string]any{"type": "integer"}),
	}, "slug", "title", "status", "level", "importance", "summary", "why_it_matters", "causal_chain", "confidence", "article_ids")),
	"signals":   arr(str("tín hiệu yếu đáng để mắt")),
	"questions": arr(str("câu hỏi đáng đào sâu")),
}, "headline", "storylines", "signals", "questions")

var synthesisSchema = obj(map[string]any{
	"title":         str("tiêu đề bản tin ngày, <= 15 từ"),
	"tldr":          arr(str("1 ý quan trọng, 1–2 câu")),
	"macro_picture": str("2–3 đoạn: bức tranh vĩ mô hôm nay, các lực chính và cách chúng tương tác"),
	"cross_links": arr(obj(map[string]any{
		"from_topic": str("topic id"), "from_story": str("slug"), "to_topic": str("topic id"), "to_story": str("slug hoặc khái niệm"),
		"mechanism": str("cơ chế/kênh truyền dẫn"), "implication": str("hệ quả"),
	}, "from_topic", "from_story", "to_topic", "to_story", "mechanism", "implication")),
	"vn_impact": arr(obj(map[string]any{
		"area": str("lĩnh vực ở VN"), "impact": str("tác động cụ thể"),
		"direction": strEnum("positive", "negative", "mixed", "uncertain"),
		"horizon":   strEnum("short", "medium", "long"), "confidence": strEnum("low", "medium", "high"),
	}, "area", "impact", "direction", "horizon", "confidence")),
	"for_you": arr(str("ý nghĩa với người dùng theo profile")),
	"watchlist": arr(obj(map[string]any{
		"item": str("sự kiện/chỉ báo"), "why": str("vì sao"), "when": str("khi nào, nếu biết"),
	}, "item", "why", "when")),
	"deep_dives": arr(obj(map[string]any{"slug": str("slug storyline"), "question": str("câu hỏi đào sâu")}, "slug", "question")),
	"contrarian": str("góc nhìn ngược: điều số đông/thị trường có thể đang bỏ qua"),
}, "title", "tldr", "macro_picture", "cross_links", "vn_impact", "for_you", "watchlist", "deep_dives", "contrarian")
