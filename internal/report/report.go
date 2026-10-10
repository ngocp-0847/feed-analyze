// Package report dựng bản tin markdown từ kết quả phân tích và ghi ra file.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

type Input struct {
	Daily      *analyze.Daily
	Articles   map[int64]store.ArticleView
	Storylines map[string]store.Storyline
}

type Options struct {
	// StoryLink trả về URL cho slug (web UI). nil = hiển thị slug dạng `code`.
	StoryLink func(slug string) string
	// ChainHTML vẽ chuỗi tác động thành khối HTML (web UI). nil = 1 dòng chữ.
	ChainHTML func([]analyze.ChainStep) string
}

// Collect nạp bài viết & storyline được tham chiếu trong bản tin.
func Collect(st *store.Store, d *analyze.Daily) (Input, error) {
	in := Input{Daily: d, Articles: map[int64]store.ArticleView{}, Storylines: map[string]store.Storyline{}}
	var ids []int64
	for _, t := range d.Topics {
		for _, sl := range t.Storylines {
			ids = append(ids, sl.ArticleIDs...)
			if s, err := st.Storyline(sl.Slug); err == nil && s != nil {
				in.Storylines[sl.Slug] = *s
			}
		}
	}
	vs, err := st.ArticlesByIDs(ids)
	if err != nil {
		return in, err
	}
	for _, v := range vs {
		in.Articles[v.ID] = v
	}
	return in, nil
}

var weekdayVI = [...]string{"Chủ nhật", "Thứ Hai", "Thứ Ba", "Thứ Tư", "Thứ Năm", "Thứ Sáu", "Thứ Bảy"}

var statusIcon = map[string]string{"new": "🆕", "developing": "🔄", "escalating": "🔺", "cooling": "🔻", "resolved": "✅"}
var statusVI = map[string]string{"new": "mới", "developing": "tiếp diễn", "escalating": "leo thang", "cooling": "hạ nhiệt", "resolved": "kết thúc"}
var dirVI = map[string]string{"positive": "🟢 tích cực", "negative": "🔴 tiêu cực", "mixed": "🟡 đan xen", "uncertain": "⚪ chưa rõ"}
var horizonVI = map[string]string{"short": "ngắn hạn", "medium": "trung hạn", "long": "dài hạn"}
var confVI = map[string]string{"low": "thấp", "medium": "vừa", "high": "cao"}

func tr(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return k
}

func stars(n int) string {
	n = max(0, min(5, n))
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

// DateTitle: "Thứ Bảy, 10/10/2026".
func DateTitle(date string, loc *time.Location) string {
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%s, %s", weekdayVI[t.Weekday()], t.Format("02/01/2006"))
}

func Markdown(cfg *config.Config, in Input, opt Options) string {
	d := in.Daily
	var b strings.Builder
	slug := func(s string) string {
		if opt.StoryLink != nil {
			return fmt.Sprintf("[`%s`](%s)", s, opt.StoryLink(s))
		}
		return "`" + s + "`"
	}
	topicName := func(id string) string {
		if t, ok := cfg.Topic(id); ok {
			return t.Emoji + " " + t.Name
		}
		return id
	}

	title := "Bản tin"
	if d.Synthesis != nil && d.Synthesis.Title != "" {
		title = d.Synthesis.Title
	}
	fmt.Fprintf(&b, "# 📰 %s\n\n", title)
	total := 0
	var counts []string
	for _, t := range d.Topics {
		total += t.ArticleCount
		counts = append(counts, fmt.Sprintf("%s %d", topicName(t.Topic), t.ArticleCount))
	}
	fmt.Fprintf(&b, "> **%s** · %d bài liên quan · %s\n\n", DateTitle(d.Date, cfg.Loc), total, strings.Join(counts, " · "))

	if s := d.Synthesis; s != nil {
		if len(s.TLDR) > 0 {
			b.WriteString("## ⚡ TL;DR\n\n")
			for _, x := range s.TLDR {
				fmt.Fprintf(&b, "- %s\n", x)
			}
			b.WriteString("\n")
		}
		if s.MacroPicture != "" {
			fmt.Fprintf(&b, "## 🌐 Bức tranh vĩ mô\n\n%s\n\n", strings.TrimSpace(s.MacroPicture))
		}
		if len(s.CrossLinks) > 0 {
			b.WriteString("## 🔗 Liên kết chéo\n\n| Từ | Tới | Cơ chế | Hệ quả |\n|---|---|---|---|\n")
			for _, l := range s.CrossLinks {
				fmt.Fprintf(&b, "| %s %s | %s %s | %s | %s |\n", emoji(cfg, l.FromTopic), slugOrText(in, l.FromStory, slug),
					emoji(cfg, l.ToTopic), slugOrText(in, l.ToStory, slug), cell(l.Mechanism), cell(l.Implication))
			}
			b.WriteString("\n")
		}
		if len(s.VNImpact) > 0 {
			b.WriteString("## 🇻🇳 Tác động tới Việt Nam\n\n| Lĩnh vực | Tác động | Chiều | Kỳ hạn | Tin cậy |\n|---|---|---|---|---|\n")
			for _, v := range s.VNImpact {
				fmt.Fprintf(&b, "| **%s** | %s | %s | %s | %s |\n", cell(v.Area), cell(v.Impact), tr(dirVI, v.Direction), tr(horizonVI, v.Horizon), tr(confVI, v.Confidence))
			}
			b.WriteString("\n")
		}
		if len(s.ForYou) > 0 {
			b.WriteString("## 👤 Với bạn\n\n")
			for _, x := range s.ForYou {
				fmt.Fprintf(&b, "- %s\n", x)
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("> ⚠️ Phần tổng hợp liên chủ đề chưa có (lỗi hoặc chưa chạy). Chạy lại: `fa analyze`.\n\n")
	}

	for _, ta := range d.Topics {
		fmt.Fprintf(&b, "## %s\n\n", topicName(ta.Topic))
		if ta.Headline != "" {
			fmt.Fprintf(&b, "_%s_\n\n", ta.Headline)
		}
		for _, sl := range ta.Storylines {
			fmt.Fprintf(&b, "### %s %s\n\n", tr(statusIcon, sl.Status), sl.Title)
			meta := []string{slug(sl.Slug), tr(statusVI, sl.Status), analyze.LevelVI(sl.Level), stars(sl.Importance)}
			if s, ok := in.Storylines[sl.Slug]; ok && s.DaysActive > 1 {
				meta = append(meta, fmt.Sprintf("ngày thứ %d (từ %s)", s.DaysActive, s.FirstDate))
			}
			meta = append(meta, "tin cậy "+tr(confVI, sl.Confidence))
			fmt.Fprintf(&b, "%s\n\n%s\n\n", strings.Join(meta, " · "), strings.TrimSpace(sl.Summary))
			if sl.WhatChanged != "" && sl.Status != "new" {
				fmt.Fprintf(&b, "- **Mới hôm nay:** %s\n", sl.WhatChanged)
			}
			if sl.WhyItMatters != "" {
				fmt.Fprintf(&b, "- **Vì sao quan trọng:** %s\n", sl.WhyItMatters)
			}
			if len(sl.CausalChain) > 0 && opt.ChainHTML == nil {
				fmt.Fprintf(&b, "- **Chuỗi tác động:** %s\n", analyze.ChainString(sl.CausalChain))
			}
			if sl.VNAngle != "" {
				fmt.Fprintf(&b, "- **Việt Nam:** %s\n", sl.VNAngle)
			}
			if len(sl.Stakeholders) > 0 {
				fmt.Fprintf(&b, "- **Các bên:** %s\n", strings.Join(sl.Stakeholders, ", "))
			}
			if len(sl.WatchNext) > 0 {
				fmt.Fprintf(&b, "- **Theo dõi:** %s\n", strings.Join(sl.WatchNext, "; "))
			}
			if src := sources(in, sl.ArticleIDs, 5); src != "" {
				fmt.Fprintf(&b, "- **Nguồn:** %s\n", src)
			}
			if len(sl.CausalChain) > 0 && opt.ChainHTML != nil {
				fmt.Fprintf(&b, "\n%s\n", opt.ChainHTML(sl.CausalChain))
			}
			b.WriteString("\n")
		}
		if len(ta.Signals) > 0 {
			b.WriteString("**📡 Tín hiệu yếu:**\n")
			for _, x := range ta.Signals {
				fmt.Fprintf(&b, "- %s\n", x)
			}
			b.WriteString("\n")
		}
		if len(ta.Questions) > 0 {
			b.WriteString("**❓ Câu hỏi đáng đào sâu:**\n")
			for _, x := range ta.Questions {
				fmt.Fprintf(&b, "- %s\n", x)
			}
			b.WriteString("\n")
		}
	}

	if s := d.Synthesis; s != nil {
		if len(s.Watchlist) > 0 {
			b.WriteString("## 👀 Theo dõi tiếp\n\n")
			for _, w := range s.Watchlist {
				when := ""
				if w.When != "" {
					when = " _(" + w.When + ")_"
				}
				fmt.Fprintf(&b, "- **%s** — %s%s\n", w.Item, w.Why, when)
			}
			b.WriteString("\n")
		}
		if len(s.DeepDives) > 0 {
			b.WriteString("## 🔍 Gợi ý đào sâu\n\n")
			for _, x := range s.DeepDives {
				fmt.Fprintf(&b, "- %s — %s  \n  `fa deep %s`\n", slug(x.Slug), x.Question, x.Slug)
			}
			b.WriteString("\n")
		}
		if s.Contrarian != "" {
			fmt.Fprintf(&b, "## 🤔 Góc nhìn ngược\n\n%s\n\n", strings.TrimSpace(s.Contrarian))
		}
	}
	b.WriteString("---\n_Tạo bởi feed-analyze + Claude. Tóm tắt dựa trên tiêu đề/mô tả RSS — kiểm chứng ở nguồn gốc trước khi ra quyết định._\n")
	return b.String()
}

func emoji(cfg *config.Config, topic string) string {
	if t, ok := cfg.Topic(topic); ok {
		return t.Emoji
	}
	return ""
}

// slugOrText: nếu là slug storyline đã biết → link/code, ngược lại giữ text.
func slugOrText(in Input, s string, slug func(string) string) string {
	if _, ok := in.Storylines[s]; ok {
		title := in.Storylines[s].Title
		return cell(title) + " " + slug(s)
	}
	return cell(s)
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "|", "/"), "\n", " ")
}

func sources(in Input, ids []int64, n int) string {
	var parts []string
	seen := map[string]bool{}
	for _, id := range ids {
		a, ok := in.Articles[id]
		if !ok || a.Link == "" {
			continue
		}
		label := a.Source
		if seen[label] {
			label = fmt.Sprintf("%s #%d", a.Source, a.ID)
		}
		seen[a.Source] = true
		parts = append(parts, fmt.Sprintf("[%s](%s)", label, a.Link))
		if len(parts) >= n {
			break
		}
	}
	if extra := len(ids) - len(parts); extra > 0 && len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("+%d bài", extra))
	}
	return strings.Join(parts, " · ")
}

// WriteFiles ghi reports/<date>.md và chép sang các thư mục copy_to. Trả về đường dẫn chính.
func WriteFiles(cfg *config.Config, name, md string, sub ...string) (string, error) {
	dir := filepath.Join(append([]string{cfg.Report.Dir}, sub...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name+".md")
	if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
		return "", err
	}
	for _, c := range cfg.Report.CopyTo {
		cd := filepath.Join(append([]string{c}, sub...)...)
		if err := os.MkdirAll(cd, 0o755); err != nil {
			return p, fmt.Errorf("copy_to %s: %w", c, err)
		}
		if err := os.WriteFile(filepath.Join(cd, name+".md"), []byte(md), 0o644); err != nil {
			return p, fmt.Errorf("copy_to %s: %w", c, err)
		}
	}
	return p, nil
}
