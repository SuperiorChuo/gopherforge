package serp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// bingCollector 抓取 cn.bing.com 的结果页。
// 选择器：#b_results > li.b_algo 内的 h2 > a；跳过广告(b_ad)与答案卡(b_ans)。
type bingCollector struct {
	client *http.Client
}

func (b *bingCollector) Name() string { return "bing" }

func (b *bingCollector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	var items []Item
	pos := 0
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://cn.bing.com/search?q=%s&first=%d&mkt=zh-CN&ensearch=0",
			url.QueryEscape(keyword), page*10+1)
		res, err := fetchPage(ctx, b.client, u)
		if err != nil {
			return items, fmt.Errorf("bing page %d: %w", page+1, err)
		}
		pageItems, err := parseBing(res.Body, &pos)
		res.Body.Close()
		if err != nil {
			return items, fmt.Errorf("bing page %d: %w", page+1, err)
		}
		items = append(items, pageItems...)
	}
	return items, nil
}

func parseBing(body io.Reader, pos *int) ([]Item, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	nodes := doc.Find("#b_results > li.b_algo")
	if nodes.Length() == 0 {
		// 容器为 0：被拦截或页面改版，不能当作"未收录"。
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	nodes.Each(func(_ int, s *goquery.Selection) {
		a := s.Find("h2 a").First()
		href, ok := a.Attr("href")
		if !ok || !strings.HasPrefix(href, "http") {
			return
		}
		*pos++
		items = append(items, Item{Position: *pos, URL: href, Title: strings.TrimSpace(a.Text())})
	})
	if len(items) == 0 {
		// 容器存在但一条真实链接都取不出（改版为跳转链等），视为解析失败而非未收录
		return nil, fmt.Errorf("results found but no real url recoverable")
	}
	return items, nil
}
