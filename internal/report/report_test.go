package report

import (
	"strings"
	"testing"
	"time"

	"github.com/ngocp-0847/feed-analyze/internal/analyze"
	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

func TestMarkdown(t *testing.T) {
	cfg, err := config.Parse(config.DefaultYAML, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := &analyze.Daily{
		Date: "2026-10-10",
		Synthesis: &analyze.Synthesis{
			Title: "Ngày của AI agent", TLDR: []string{"ý 1"}, MacroPicture: "vĩ mô",
			CrossLinks: []analyze.CrossLink{{FromTopic: "ai-agent", FromStory: "agent-security", ToTopic: "politics", ToStory: "khái niệm | lạ", Mechanism: "an ninh", Implication: "luật"}},
			VNImpact:   []analyze.VNImpact{{Area: "IT", Impact: "x", Direction: "mixed", Horizon: "short", Confidence: "low"}},
			DeepDives:  []analyze.DeepDive{{Slug: "agent-security", Question: "?"}},
		},
		Topics: []analyze.TopicAnalysis{{Topic: "ai-agent", Headline: "h", ArticleCount: 2, Storylines: []analyze.StorylineOut{{
			Slug: "agent-security", Title: "Agent tấn công", Status: "developing", Level: "meso", Importance: 4, Confidence: "medium",
			Summary: "tóm tắt", WhatChanged: "mới", CausalChain: []analyze.ChainStep{{Level: "micro", Step: "a"}, {Level: "macro", Step: "b", Channel: "c"}},
			ArticleIDs: []int64{1, 2},
		}}}},
	}
	in := Input{Daily: d,
		Articles: map[int64]store.ArticleView{
			1: {Article: store.Article{ID: 1, Source: "vnexpress.net", Link: "https://vnexpress.net/1", Published: time.Now()}},
			2: {Article: store.Article{ID: 2, Source: "vnexpress.net", Link: "https://vnexpress.net/2", Published: time.Now()}},
		},
		Storylines: map[string]store.Storyline{"agent-security": {Slug: "agent-security", Title: "Agent tấn công", DaysActive: 3, FirstDate: "2026-10-08"}},
	}
	md := Markdown(cfg, in, Options{})
	for _, want := range []string{
		"# 📰 Ngày của AI agent", "Thứ Bảy, 10/10/2026", "## ⚡ TL;DR", "🔄 Agent tấn công", "★★★★☆",
		"ngày thứ 3 (từ 2026-10-08)", "—(c)→ [vĩ mô] b", "[vnexpress.net](https://vnexpress.net/1)", "[vnexpress.net #2]",
		"khái niệm / lạ", "🟡 đan xen", "`fa deep agent-security`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("thiếu %q", want)
		}
	}
	web := Markdown(cfg, in, Options{StoryLink: func(s string) string { return "/s/" + s }})
	if !strings.Contains(web, "[`agent-security`](/s/agent-security)") {
		t.Error("web mode phải link storyline")
	}
}
