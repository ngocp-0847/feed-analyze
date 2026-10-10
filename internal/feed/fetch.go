// Package feed quét RSS/Atom song song, chuẩn hoá và lưu bài mới.
package feed

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/gofeed"

	"github.com/ngocp-0847/feed-analyze/internal/config"
	"github.com/ngocp-0847/feed-analyze/internal/store"
)

const userAgent = "Mozilla/5.0 (compatible; feed-analyze/1.0; +https://github.com/ngocp-0847/feed-analyze)"

type Result struct {
	Feed  config.Feed
	Items int // số item trong feed
	Fresh int // item còn trong hạn max_age
	New   int // item mới lưu
	Err   error
	Took  time.Duration
}

// FetchAll quét mọi feed đang bật. progress (có thể nil) được gọi sau mỗi feed.
func FetchAll(ctx context.Context, cfg *config.Config, st *store.Store, feeds []config.Feed, progress func(Result)) []Result {
	client := &http.Client{Timeout: time.Duration(cfg.Fetch.TimeoutSeconds) * time.Second}
	sem := make(chan struct{}, cfg.Fetch.Concurrency)
	results := make([]Result, len(feeds))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, f := range feeds {
		wg.Add(1)
		go func(i int, f config.Feed) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := fetchOne(ctx, client, cfg, st, f)
			results[i] = r
			status := "ok"
			if r.Err != nil {
				status = r.Err.Error()
			}
			_ = st.SaveFeedStatus(store.FeedStatus{Name: f.Name, URL: f.URL, Status: status, LastFetched: time.Now(), Items: r.Items, New: r.New})
			if progress != nil {
				mu.Lock()
				progress(r)
				mu.Unlock()
			}
		}(i, f)
	}
	wg.Wait()
	return results
}

func fetchOne(ctx context.Context, client *http.Client, cfg *config.Config, st *store.Store, f config.Feed) Result {
	start := time.Now()
	r := Result{Feed: f}
	parsed, err := Parse(ctx, client, f.URL)
	if err != nil {
		r.Err, r.Took = err, time.Since(start)
		return r
	}
	r.Items = len(parsed.Items)
	now := time.Now()
	cutoff := now.Add(-time.Duration(cfg.Fetch.MaxAgeHours) * time.Hour)

	items := parsed.Items
	sort.SliceStable(items, func(i, j int) bool { return pubTime(items[i], now).After(pubTime(items[j], now)) })
	for _, it := range items {
		if r.Fresh >= cfg.Fetch.MaxItemsPerFeed {
			break
		}
		pub := pubTime(it, now)
		if pub.Before(cutoff) {
			continue
		}
		title := clean(it.Title, 300)
		if title == "" {
			continue
		}
		r.Fresh++
		link := strings.TrimSpace(it.Link)
		summary := clean(firstNonEmpty(it.Description, it.Content), 1200)
		a := &store.Article{
			Hash: hashKey(link, title, f.Name), Feed: f.Name, Source: sourceName(f.Name, link),
			Region: f.Region, Lang: f.Lang, Title: title, Link: link, Summary: summary,
			Published: pub, Fetched: now,
		}
		// cùng tiêu đề đã có trong 2 ngày (vd: feed khác của cùng báo) → bỏ
		if st.TitleExistsSince(title, now.Add(-48*time.Hour)) {
			continue
		}
		ok, err := st.InsertArticle(a)
		if err != nil {
			r.Err = err
			break
		}
		if ok {
			r.New++
		}
	}
	r.Took = time.Since(start)
	return r
}

// Parse tải và parse 1 feed.
func Parse(ctx context.Context, client *http.Client, u string) (*gofeed.Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	fp := gofeed.NewParser()
	feed, err := fp.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return feed, nil
}

func pubTime(it *gofeed.Item, now time.Time) time.Time {
	var t time.Time
	switch {
	case it.PublishedParsed != nil:
		t = *it.PublishedParsed
	case it.UpdatedParsed != nil:
		t = *it.UpdatedParsed
	default:
		return now
	}
	if t.After(now.Add(time.Hour)) || t.Year() < 2000 {
		return now
	}
	return t
}

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

func clean(s string, max int) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > max {
		rs := []rune(s)
		s = string(rs[:max]) + "…"
	}
	return s
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// hashKey: link chuẩn hoá (bỏ utm_*, fragment); không có link thì dùng tiêu đề + nguồn.
func hashKey(link, title, feed string) string {
	key := "t:" + strings.ToLower(title) + "|" + feed
	if u, err := url.Parse(link); err == nil && u.Host != "" {
		q := u.Query()
		for k := range q {
			if strings.HasPrefix(k, "utm_") || k == "ref" || k == "fbclid" {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
		u.Fragment = ""
		u.Scheme = "https"
		u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
		key = "u:" + u.String()
	}
	h := sha1.Sum([]byte(key))
	return hex.EncodeToString(h[:])
}

// sourceName: domain của bài viết (vd vnexpress.net); không có link thì dùng tên feed.
func sourceName(feedName, link string) string {
	if u, err := url.Parse(link); err == nil && u.Host != "" {
		return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	}
	return feedName
}
