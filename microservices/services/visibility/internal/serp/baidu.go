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

// baiduCollector 抓取 www.baidu.com 的结果页。
// 结果块：div#content_left 下的 .result / .c-container；真实 URL 还原三级：
// mu 属性优先 → .c-showurl 文本取 host 兜底 → 都没有则跳过该条（不做 HEAD 跟跳）。
type baiduCollector struct {
	client *http.Client
}

func (b *baiduCollector) Name() string { return "baidu" }

func (b *baiduCollector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	var items []Item
	pos := 0
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://www.baidu.com/s?wd=%s&pn=%d&ie=utf-8",
			url.QueryEscape(keyword), page*10)
		res, err := fetchPage(ctx, b.client, u)
		if err != nil {
			return items, fmt.Errorf("baidu page %d: %w", page+1, err)
		}
		finalURL := ""
		if res.Request != nil && res.Request.URL != nil {
			finalURL = res.Request.URL.String()
		}
		pageItems, err := parseBaidu(res.Body, finalURL, &pos)
		res.Body.Close()
		if err != nil {
			return items, fmt.Errorf("baidu page %d: %w", page+1, err)
		}
		items = append(items, pageItems...)
	}
	return items, nil
}

func parseBaidu(body io.Reader, finalURL string, pos *int) ([]Item, error) {
	if strings.Contains(finalURL, "wappass.baidu.com") {
		return nil, fmt.Errorf("触发百度安全验证（wappass 重定向）")
	}
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(doc.Find("title").Text(), "百度安全验证") {
		return nil, fmt.Errorf("触发百度安全验证")
	}
	nodes := doc.Find("div#content_left div.result, div#content_left div.c-container")
	if nodes.Length() == 0 {
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	nodes.Each(func(_ int, s *goquery.Selection) {
		// 跳过广告块
		if _, isAd := s.Attr("data-tuiguang"); isAd {
			return
		}
		if cls, _ := s.Attr("class"); strings.Contains(cls, "ec_") {
			return
		}
		title := strings.TrimSpace(s.Find("h3").First().Text())
		realURL := ""
		if mu, ok := s.Attr("mu"); ok && strings.HasPrefix(mu, "http") {
			realURL = mu
		} else {
			// showurl 可能被截断，只用于 host 匹配
			show := strings.TrimSpace(s.Find(".c-showurl, .c-showurl-color").First().Text())
			show = trimEllipsis(show)
			if show != "" {
				realURL = show
			}
		}
		if realURL == "" {
			return
		}
		*pos++
		items = append(items, Item{Position: *pos, URL: realURL, Title: title})
	})
	if len(items) == 0 {
		// 容器存在但一条真实 URL 都还原不出来，视为解析失败而非未收录。
		return nil, fmt.Errorf("results found but no real url recoverable")
	}
	return items, nil
}
