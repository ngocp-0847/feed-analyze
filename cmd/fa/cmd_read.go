package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/feed"
	"github.com/ngocp-0847/feed-analyze/internal/research"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func cmdBrief(a *app, args []string) error {
	fs := newFlags("brief")
	date := fs.String("date", "", "YYYY-MM-DD (mặc định: bản tin mới nhất)")
	webView := fs.Bool("web", false, "mở bản tin + dashboard trên trình duyệt")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	d := *date
	if d == "" {
		dates, err := a.st.ReportDates("daily-md", 1)
		if err != nil {
			return err
		}
		if len(dates) == 0 {
			return errors.New("chưa có bản tin — chạy `fa daily`")
		}
		d = dates[0]
		if d != a.cfg.Today() {
			logf("⚠ chưa có bản tin hôm nay — hiển thị bản %s (chạy `fa daily` để cập nhật)", d)
		}
	}
	rep, err := a.st.GetReport(d, "daily-md")
	if err != nil {
		return err
	}
	if rep == nil {
		return fmt.Errorf("không có bản tin ngày %s", d)
	}
	if *webView {
		return openPath(a, "/r/"+d)
	}
	fmt.Print(rep.Markdown)
	return nil
}

func cmdContext(a *app, args []string) error {
	fs := newFlags("context")
	date := fs.String("date", "", "YYYY-MM-DD")
	days := fs.Int("days", 3, "storyline hoạt động trong N ngày")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := research.Context(a.cfg, a.st, *date, *days)
	if err != nil {
		return err
	}
	fmt.Print(s)
	return nil
}

func cmdStories(a *app, args []string) error {
	fs := newFlags("stories")
	topic := fs.String("topic", "", "lọc theo topic id")
	days := fs.Int("days", 7, "hoạt động trong N ngày")
	asJSON := fs.Bool("json", false, "xuất JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	since := time.Now().In(a.cfg.Loc).AddDate(0, 0, -*days+1).Format("2006-01-02")
	sls, err := a.st.Storylines(*topic, since, 300)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(sls)
	}
	if len(sls) == 0 {
		fmt.Println("Chưa có storyline. Chạy `fa daily`.")
		return nil
	}
	for _, t := range a.cfg.Topics {
		first := true
		for _, s := range sls {
			if s.Topic != t.ID {
				continue
			}
			if first {
				fmt.Printf("\n%s %s\n", t.Emoji, t.Name)
				first = false
			}
			fmt.Printf("  %-42s %-10s %-8s %2dd  %s  %s\n", s.Slug, s.Status, analyze.LevelVI(s.Level), s.DaysActive, s.LastDate, s.Title)
		}
	}
	return nil
}

func cmdStory(a *app, args []string) error {
	fs := newFlags("story")
	asJSON := fs.Bool("json", false, "xuất JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("cần slug: fa story <slug> (xem `fa stories`)")
	}
	sl, err := a.st.Storyline(pos[0])
	if err != nil {
		return err
	}
	if sl == nil {
		return fmt.Errorf("không có storyline %q", pos[0])
	}
	evs, _ := a.st.StorylineEvents(sl.Slug)
	ids, _ := a.st.StorylineArticleIDs(sl.Slug)
	arts, _ := a.st.ArticlesByIDs(ids)
	if *asJSON {
		type ev struct {
			store.StorylineEvent
			Detail *analyze.StorylineOut `json:"detail,omitempty"`
		}
		var out []ev
		for _, e := range evs {
			d := &analyze.StorylineOut{}
			if json.Unmarshal([]byte(e.Detail), d) != nil {
				d = nil
			}
			e.Detail = ""
			out = append(out, ev{e, d})
		}
		return printJSON(map[string]any{"storyline": sl, "timeline": out, "articles": arts})
	}
	fmt.Printf("# %s\n`%s` · %s · %s · %s · %d ngày (%s → %s)\n\n%s\n\nVì sao quan trọng: %s\n\n## Diễn biến\n",
		sl.Title, sl.Slug, sl.Topic, analyze.LevelVI(sl.Level), sl.Status, sl.DaysActive, sl.FirstDate, sl.LastDate, sl.Summary, sl.Why)
	for _, e := range evs {
		var d analyze.StorylineOut
		_ = json.Unmarshal([]byte(e.Detail), &d)
		fmt.Printf("\n### %s [%s]\n%s\n", e.Date, e.Status, e.Summary)
		if d.WhatChanged != "" {
			fmt.Printf("- Mới: %s\n", d.WhatChanged)
		}
		if len(d.CausalChain) > 0 {
			fmt.Printf("- Chuỗi tác động: %s\n", analyze.ChainString(d.CausalChain))
		}
		if d.VNAngle != "" {
			fmt.Printf("- Việt Nam: %s\n", d.VNAngle)
		}
		if len(d.WatchNext) > 0 {
			fmt.Printf("- Theo dõi: %s\n", strings.Join(d.WatchNext, "; "))
		}
	}
	fmt.Printf("\n## Bài viết (%d)\n", len(arts))
	printArticles(a, arts)
	return nil
}

func printArticles(a *app, vs []store.ArticleView) {
	for _, v := range vs {
		meta := ""
		if v.Classification != nil {
			var ts []string
			for _, t := range v.Topics {
				ts = append(ts, fmt.Sprintf("%s:%.1f", t.Topic, t.Relevance))
			}
			meta = fmt.Sprintf(" [imp %d · %s · %s]", v.Importance, analyze.LevelVI(v.Scale), strings.Join(ts, " "))
		}
		fmt.Printf("#%-6d %s %-22s %s%s\n", v.ID, v.Published.In(a.cfg.Loc).Format("01-02 15:04"), trunc(v.Source, 22), v.Title, meta)
		if v.Classification != nil && v.SummaryVI != "" {
			fmt.Printf("        %s\n", v.SummaryVI)
		}
		fmt.Printf("        %s\n", v.Link)
	}
}

func cmdSearch(a *app, args []string) error {
	fs := newFlags("search")
	topic := fs.String("topic", "", "lọc topic id")
	days := fs.Int("days", 7, "trong N ngày")
	limit := fs.Int("limit", 30, "số kết quả")
	asJSON := fs.Bool("json", false, "xuất JSON")
	anyWord := fs.Bool("any", false, "khớp BẤT KỲ từ nào (mặc định: tất cả các từ)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	o := store.SearchOpts{Query: strings.Join(pos, " "), Topic: *topic, Since: time.Now().AddDate(0, 0, -*days), Limit: *limit}
	if *anyWord {
		o.Raw = research.NaturalQuery(o.Query)
	}
	vs, err := a.st.Search(o)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(vs)
	}
	if len(vs) == 0 {
		fmt.Println("Không tìm thấy. Thử --days lớn hơn hoặc --any.")
		return nil
	}
	printArticles(a, vs)
	return nil
}

func cmdShow(a *app, args []string) error {
	fs := newFlags("show")
	asJSON := fs.Bool("json", false, "xuất JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New("cần id: fa show <id>")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(pos[0], "#"), 10, 64)
	if err != nil {
		return fmt.Errorf("id không hợp lệ: %s", pos[0])
	}
	vs, err := a.st.ArticlesByIDs([]int64{id})
	if err != nil {
		return err
	}
	if len(vs) == 0 {
		return fmt.Errorf("không có bài #%d", id)
	}
	v := vs[0]
	if *asJSON {
		return printJSON(v)
	}
	fmt.Printf("#%d %s\n%s · %s · %s/%s\n%s\n\n", v.ID, v.Title, v.Source, v.Published.In(a.cfg.Loc).Format("2006-01-02 15:04"), v.Region, v.Lang, v.Link)
	if v.Summary != "" {
		fmt.Printf("Mô tả RSS: %s\n\n", v.Summary)
	}
	if c := v.Classification; c != nil {
		fmt.Printf("Tóm tắt: %s\nChủ đề: %v\nQuan trọng: %d · tầng %s · %s\nThực thể: %s\n", c.SummaryVI, c.Topics, c.Importance, analyze.LevelVI(c.Scale), c.Geo, strings.Join(c.Entities, ", "))
	} else {
		fmt.Println("(chưa phân loại)")
	}
	return nil
}

func cmdTopics(a *app, _ []string) error {
	st, _ := a.st.Stats(time.Now().Add(-24 * time.Hour))
	for _, t := range a.cfg.Topics {
		fmt.Printf("%s %-12s %s  (%d bài/24h)\n   %s\n", t.Emoji, t.ID, t.Name, st.TopicCounts[t.ID], strings.TrimSpace(t.Description))
	}
	return nil
}

func cmdFeeds(a *app, args []string) error {
	fs := newFlags("feeds")
	u := fs.String("url", "", "kiểm tra 1 URL RSS bất kỳ")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Duration(a.cfg.Fetch.TimeoutSeconds) * time.Second}
	if *u != "" {
		return checkFeed(a.ctx, client, *u)
	}
	if len(pos) > 0 && pos[0] == "check" {
		fails := 0
		for _, f := range a.cfg.ActiveFeeds() {
			p, err := feed.Parse(a.ctx, client, f.URL)
			if err != nil {
				fails++
				fmt.Printf("✗ %-28s %v\n", f.Name, err)
				continue
			}
			fmt.Printf("✓ %-28s %4d item  %s\n", f.Name, len(p.Items), newest(p.Items))
		}
		fmt.Printf("\n%d/%d nguồn lỗi\n", fails, len(a.cfg.ActiveFeeds()))
		return nil
	}
	sts, err := a.st.FeedStatuses()
	if err != nil {
		return err
	}
	fmt.Printf("%-28s %-5s %-4s %-17s %s\n", "NGUỒN", "VÙNG", "MỚI", "LẦN QUÉT CUỐI", "TRẠNG THÁI")
	for _, f := range a.cfg.Feeds {
		s, ok := sts[f.Name]
		last, status := "-", "chưa quét"
		if ok {
			last, status = s.LastFetched.In(a.cfg.Loc).Format("2006-01-02 15:04"), s.Status
		}
		if f.Off {
			status = "tắt"
		}
		fmt.Printf("%-28s %-5s %-4d %-17s %s\n", trunc(f.Name, 28), f.Region, s.New, last, trunc(status, 60))
	}
	return nil
}

func checkFeed(ctx context.Context, client *http.Client, u string) error {
	p, err := feed.Parse(ctx, client, u)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s — %d item, mới nhất: %s\n", p.Title, len(p.Items), newest(p.Items))
	for i, it := range p.Items {
		if i >= 5 {
			break
		}
		fmt.Printf("  - %s\n", it.Title)
	}
	return nil
}

func newest(items []*gofeed.Item) string {
	var ts []time.Time
	for _, it := range items {
		if it.PublishedParsed != nil {
			ts = append(ts, *it.PublishedParsed)
		}
	}
	if len(ts) == 0 {
		return "(không rõ ngày)"
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].After(ts[j]) })
	return ts[0].Local().Format("2006-01-02 15:04")
}

func cmdStats(a *app, _ []string) error {
	st, err := a.st.Stats(time.Now().Add(-24 * time.Hour))
	if err != nil {
		return err
	}
	fmt.Printf("Home:        %s\nConfig:      %s\nDatabase:    %s\nBản tin:     %s\n\n", a.home, a.home+string(os.PathSeparator)+"config.yaml", a.cfg.DBPath(), a.cfg.Report.Dir)
	fmt.Printf("Bài viết:    %d (đã phân loại %d, liên quan %d, chờ %d)\nStoryline:   %d\nSố bản tin:  %d\n",
		st.Articles, st.Classified, st.Relevant, a.st.CountUnclassified(), st.Storylines, st.Reports)
	fmt.Printf("Claude 24h:  %d lần gọi, %d lỗi, ≈ $%.3f   ·  30 ngày ≈ $%.2f\n", st.Calls24h, st.Fail24h, st.Cost24h, st.Cost30d)
	fmt.Println("\nBài liên quan 24h:")
	for _, t := range a.cfg.Topics {
		fmt.Printf("  %s %-30s %d\n", t.Emoji, t.Name, st.TopicCounts[t.ID])
	}
	return nil
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
