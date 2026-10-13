package serp

// 扩展采集器：搜狗 / 360 / DuckDuckGo（HTML 版）/ GitHub 仓库搜索。
// 与内置三引擎同一套约定：结果容器为 0 必须报错（拦截或改版），不能当"未收录"。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// sogouCollector 抓取 www.sogou.com 网页搜索。
// 结果块 .results > .vrwrap / .rb；链接多为 /link?url= 跳转，无法便宜还原真实域名时
// 用 .fz-mid.citeurl / .citeurl 的展示域名兜底（同百度 showurl 思路，只够 host 匹配）。
type sogouCollector struct {
	client *http.Client
}

func (s *sogouCollector) Name() string { return "sogou" }

func (s *sogouCollector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	var items []Item
	pos := 0
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://www.sogou.com/web?query=%s&page=%d", url.QueryEscape(keyword), page+1)
		res, err := fetchPage(ctx, s.client, u)
		if err != nil {
			return items, fmt.Errorf("sogou page %d: %w", page+1, err)
		}
		finalURL := ""
		if res.Request != nil && res.Request.URL != nil {
			finalURL = res.Request.URL.String()
		}
		pageItems, err := parseSogou(res.Body, finalURL, &pos)
		res.Body.Close()
		if err != nil {
			return items, fmt.Errorf("sogou page %d: %w", page+1, err)
		}
		items = append(items, pageItems...)
	}
	return items, nil
}

func parseSogou(body io.Reader, finalURL string, pos *int) ([]Item, error) {
	if strings.Contains(finalURL, "antispider") || strings.Contains(finalURL, "captcha") {
		return nil, fmt.Errorf("触发搜狗反爬验证")
	}
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	nodes := doc.Find(".results .vrwrap, .results .rb, .results .result")
	if nodes.Length() == 0 {
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	nodes.Each(func(_ int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find("h3").First().Text())
		realURL := ""
		if href, ok := sel.Find("h3 a").First().Attr("href"); ok && strings.HasPrefix(href, "http") && !strings.Contains(href, "sogou.com/link") {
			realURL = href
		} else {
			cite := strings.TrimSpace(sel.Find(".citeurl, .fz-mid.citeurl, .str-blue").First().Text())
			if fields := strings.Fields(cite); len(fields) > 0 {
				realURL = trimEllipsis(fields[0])
			}
		}
		if realURL == "" {
			return
		}
		*pos++
		items = append(items, Item{Position: *pos, URL: realURL, Title: title})
	})
	if len(items) == 0 {
		return nil, fmt.Errorf("results found but no real url recoverable")
	}
	return items, nil
}

// so360Collector 抓取 www.so.com（360 搜索）。
// 结果块 ul.result > li.res-list；真实 URL 优先 data-mdurl / lnk-url 属性，
// 兜底 .res-linkinfo cite 展示域名。
type so360Collector struct {
	client *http.Client
}

func (s *so360Collector) Name() string { return "so360" }

func (s *so360Collector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	var items []Item
	pos := 0
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://www.so.com/s?q=%s&pn=%d", url.QueryEscape(keyword), page+1)
		res, err := fetchPage(ctx, s.client, u)
		if err != nil {
			return items, fmt.Errorf("so360 page %d: %w", page+1, err)
		}
		// 360 对可疑 IP 会 302 到 qcaptcha 验证码页，落地页无结果容器；
		// 按安全验证明确报错，别让 "no result container" 误导成选择器改版
		if res.Request != nil && res.Request.URL != nil &&
			strings.Contains(res.Request.URL.Host, "captcha") {
			res.Body.Close()
			return items, fmt.Errorf("so360 page %d: 触发 360 安全验证（captcha 重定向）", page+1)
		}
		pageItems, err := parseSo360(res.Body, &pos)
		res.Body.Close()
		if err != nil {
			return items, fmt.Errorf("so360 page %d: %w", page+1, err)
		}
		items = append(items, pageItems...)
	}
	return items, nil
}

func parseSo360(body io.Reader, pos *int) ([]Item, error) {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(doc.Find("title").Text(), "安全验证") {
		return nil, fmt.Errorf("触发 360 安全验证")
	}
	nodes := doc.Find("ul.result li.res-list, ul#m-result li.res-list")
	if nodes.Length() == 0 {
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	nodes.Each(func(_ int, sel *goquery.Selection) {
		a := sel.Find("h3 a").First()
		title := strings.TrimSpace(a.Text())
		realURL := ""
		for _, attr := range []string{"data-mdurl", "lnk-url", "data-url"} {
			if v, ok := a.Attr(attr); ok && strings.HasPrefix(v, "http") && !strings.Contains(v, "so.com/link") {
				realURL = v
				break
			}
		}
		if realURL == "" {
			if href, ok := a.Attr("href"); ok && strings.HasPrefix(href, "http") && !strings.Contains(href, "so.com/link") {
				realURL = href
			}
		}
		if realURL == "" {
			cite := strings.TrimSpace(sel.Find(".res-linkinfo cite, cite").First().Text())
			if fields := strings.Fields(cite); len(fields) > 0 {
				realURL = trimEllipsis(fields[0])
			}
		}
		if realURL == "" {
			return
		}
		*pos++
		items = append(items, Item{Position: *pos, URL: realURL, Title: title})
	})
	if len(items) == 0 {
		return nil, fmt.Errorf("results found but no real url recoverable")
	}
	return items, nil
}

// ddgCollector 抓取 DuckDuckGo 的 html 版（html.duckduckgo.com/html，无 JS 依赖，
// 对自动化最宽容）。DDG 不分页语义与他家不同（POST s= 偏移），这里只取首屏 ~30 条，
// maxPages 仅用于对齐接口。国内网络若不可达则整引擎报错，由上层记 error。
type ddgCollector struct {
	client *http.Client
}

func (d *ddgCollector) Name() string { return "duckduckgo" }

func (d *ddgCollector) Fetch(ctx context.Context, keyword string, _ int) ([]Item, error) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(keyword)
	res, err := fetchPage(ctx, d.client, u)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: %w", err)
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}
	nodes := doc.Find(".result:not(.result--ad) .result__title a.result__a")
	if nodes.Length() == 0 {
		return nil, fmt.Errorf("no result container (blocked or layout changed)")
	}
	var items []Item
	pos := 0
	nodes.Each(func(_ int, a *goquery.Selection) {
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		// DDG html 版链接形如 //duckduckgo.com/l/?uddg=<encoded>；还原真实 URL
		if strings.Contains(href, "uddg=") {
			if u2, err := url.Parse(href); err == nil {
				if real := u2.Query().Get("uddg"); real != "" {
					href = real
				}
			}
		}
		if !strings.HasPrefix(href, "http") {
			return
		}
		pos++
		items = append(items, Item{Position: pos, URL: href, Title: strings.TrimSpace(a.Text())})
	})
	if len(items) == 0 {
		return nil, fmt.Errorf("results found but no real url recoverable")
	}
	return items, nil
}

// githubCollector 用 GitHub 仓库搜索 API（无凭证 10 次/分钟，每日一次 cron 足够）。
// 排名含义：关键词在 GitHub 仓库搜索（best match）中你的仓库排第几——
// 开发者找轮子的主战场，对开源项目它比通用引擎更贴近真实获客。
type githubCollector struct {
	client *http.Client
}

func (g *githubCollector) Name() string { return "github" }

func (g *githubCollector) Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error) {
	per := 30
	if maxPages <= 0 {
		maxPages = 1
	}
	if maxPages > 2 {
		maxPages = 2 // 60 条足够，别烧匿名配额
	}
	var items []Item
	pos := 0
	for page := 1; page <= maxPages; page++ {
		if page > 1 {
			if err := sleepCtx(ctx, pageGap()); err != nil {
				return items, err
			}
		}
		u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=%d&page=%d",
			url.QueryEscape(keyword), per, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return items, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "gopherforge-visibility/1.0")
		res, err := g.client.Do(req)
		if err != nil {
			return items, fmt.Errorf("github page %d: %w", page, err)
		}
		if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusTooManyRequests {
			res.Body.Close()
			return items, fmt.Errorf("github api rate limited (%d)", res.StatusCode)
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return items, fmt.Errorf("github page %d: status %d", page, res.StatusCode)
		}
		var parsed struct {
			Items []struct {
				HTMLURL  string `json:"html_url"`
				FullName string `json:"full_name"`
			} `json:"items"`
		}
		err = json.NewDecoder(res.Body).Decode(&parsed)
		res.Body.Close()
		if err != nil {
			return items, fmt.Errorf("github page %d: %w", page, err)
		}
		for _, it := range parsed.Items {
			pos++
			items = append(items, Item{Position: pos, URL: it.HTMLURL, Title: it.FullName})
		}
		if len(parsed.Items) < per {
			break
		}
	}
	if len(items) == 0 {
		// API 正常返回空列表是真·无结果；但对排名监测更可能是关键词全中文等场景，照实回空
		return items, nil
	}
	return items, nil
}
