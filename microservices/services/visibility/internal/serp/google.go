package serp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// googleCollector 抓取 www.google.com 的结果页。
// Google class 名是混淆哈希不可依赖，用"div#search 内、父链上有 h3 的外链"做选择器。
// 国内服务器大概率不通：每请求独立 10s 超时，第一页失败立即放弃整个引擎（快速失败）。
type googleCollector struct {
	client *http.Client
}

func (g *googleCollector) Name() string { return "google" }

func (g *googleCollector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	var items []Item
	pos := 0
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://www.google.com/search?q=%s&start=%d&hl=zh-CN&num=10",
			url.QueryEscape(keyword), page*10)
		// cancel 必须在 body 消费完之后：提前 cancel 会让 body 读取报 context canceled
		pageItems, err := func() ([]Item, error) {
			reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			res, err := fetchPage(reqCtx, g.client, u)
			if err != nil {
				return nil, err
			}
			defer res.Body.Close()
			if res.Request != nil && res.Request.URL != nil {
				final := res.Request.URL.String()
				if strings.Contains(final, "consent.google") || strings.Contains(final, "/sorry/") {
					return nil, fmt.Errorf("google blocked (consent/sorry page)")
				}
			}
			return parseGoogle(res.Body, &pos)
		}()
		if err != nil {
			// 首页即失败 → 整个引擎放弃；后续页失败 → 返回已拿到的。
			return items, fmt.Errorf("google page %d: %w", page+1, err)
		}
		items = append(items, pageItems...)
	}
	return items, nil
}

func parseGoogle(body io.Reader, pos *int) ([]Item, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	root := doc.Find("div#search")
	if root.Length() == 0 {
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	seen := map[string]bool{}
	root.Find("a").Each(func(_ int, a *goquery.Selection) {
		if a.Find("h3").Length() == 0 {
			return
		}
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		real := extractGoogleURL(href)
		if real == "" || seen[real] {
			return
		}
		seen[real] = true
		*pos++
		items = append(items, Item{Position: *pos, URL: real, Title: strings.TrimSpace(a.Find("h3").First().Text())})
	})
	if len(items) == 0 {
		return nil, fmt.Errorf("search container present but no organic links parsed")
	}
	return items, nil
}

// extractGoogleURL 处理 /url?q=<real> 形式与直链。
func extractGoogleURL(href string) string {
	if strings.HasPrefix(href, "/url?") {
		u, err := url.Parse(href)
		if err != nil {
			return ""
		}
		q := u.Query().Get("q")
		if strings.HasPrefix(q, "http") {
			return q
		}
		return ""
	}
	if strings.HasPrefix(href, "http") && !strings.Contains(href, "google.com/") {
		return href
	}
	return ""
}
