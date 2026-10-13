// Package serp fetches search engine result pages and locates a target
// domain's rank for a keyword. Collectors: bing / baidu / google.
//
// 可信度约定：解析后"结果容器节点数为 0"（被拦截或页面改版）必须返回 error，
// 而不是 rank=0；只有"容器存在但没匹配到目标域名"才是真正的未收录（rank=0）。
package serp

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Item is one organic search result with its global position (1-based, across pages).
type Item struct {
	Position int
	URL      string
	Title    string
}

// Collector fetches up to maxPages result pages for a keyword.
// 单页失败即返回已拿到的结果 + error（快速失败）。
type Collector interface {
	Name() string // "bing" | "baidu" | "google"
	Fetch(ctx context.Context, keyword string, maxPages int) ([]Item, error)
}

// EngineResult is one engine's outcome for a keyword check.
type EngineResult struct {
	Rank    int    // 0 = 前 N 未收录（仅当 Err == nil 时有意义）
	SerpURL string // 命中的真实结果 URL
	Err     error  // 非 nil = 该引擎本次采集失败
}

// DefaultCollectors 返回默认三采集器（共享一个带 cookiejar 的 http client）。
func DefaultCollectors() []Collector {
	return CollectorsByNames([]string{"bing", "baidu", "google"})
}

// CollectorsByNames 按名字装配采集器，未知名字忽略。
// 可选：bing / baidu / google / sogou / so360 / duckduckgo / github。
func CollectorsByNames(names []string) []Collector {
	c := newHTTPClient()
	var out []Collector
	for _, n := range names {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "bing":
			out = append(out, &bingCollector{client: c})
		case "baidu":
			out = append(out, &baiduCollector{client: c})
		case "google":
			out = append(out, &googleCollector{client: c})
		case "sogou":
			out = append(out, &sogouCollector{client: c})
		case "so360", "360":
			out = append(out, &so360Collector{client: c})
		case "duckduckgo", "ddg":
			out = append(out, &ddgCollector{client: c})
		case "github":
			out = append(out, &githubCollector{client: c})
		}
	}
	return out
}

// CheckKeyword 对单个关键词串行跑所有引擎，单引擎失败只记 error 不中断。
func CheckKeyword(ctx context.Context, cols []Collector, keyword, targetURL string, maxPages int) map[string]EngineResult {
	out := make(map[string]EngineResult, len(cols))
	for i, col := range cols {
		if i > 0 {
			if err := sleepCtx(ctx, jitter(3*time.Second, 5*time.Second)); err != nil {
				out[col.Name()] = EngineResult{Err: err}
				continue
			}
		}
		items, err := col.Fetch(ctx, keyword, maxPages)
		if err != nil && len(items) == 0 {
			out[col.Name()] = EngineResult{Err: err}
			continue
		}
		rank, serpURL := findTarget(items, targetURL)
		if rank == 0 && err != nil {
			// 翻页中途失败且未命中：目标可能在没抓到的页里，"未收录"不可信，按失败记
			out[col.Name()] = EngineResult{Err: err}
			continue
		}
		out[col.Name()] = EngineResult{Rank: rank, SerpURL: serpURL}
	}
	return out
}

func findTarget(items []Item, targetURL string) (int, string) {
	for _, it := range items {
		if matchTarget(it.URL, targetURL) {
			return it.Position, it.URL
		}
	}
	return 0, ""
}

// URLMatchesTarget 暴露 matchTarget 的判定给 runner：判断结果 URL 是否命中
// 目标站点（域名相等/子域 + 路径前缀，GitHub/Gitee 共享域名感知）。GEO 来源
// 归因用它判断检索来源是否命中自有品牌站点，规则与 SERP 排名匹配完全一致。
func URLMatchesTarget(itemURL, targetURL string) bool {
	return matchTarget(itemURL, targetURL)
}

// GroundingSearch 做一次轻量检索：单引擎单页，供 GEO 检测接地用。
// 与 CheckKeyword 不同：不跑多引擎串行、不加引擎间延时——调用方负责短超时。
// 返回最多 maxResults 个 http/https 结果（按 URL 去重），不含目标域名匹配逻辑。
func GroundingSearch(ctx context.Context, col Collector, query string, maxResults int) ([]Item, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	items, err := col.Fetch(ctx, query, 1)
	if err != nil && len(items) == 0 {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Item, 0, maxResults)
	for _, it := range items {
		if !isHTTPScheme(it.URL) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(it.URL))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
		if len(out) >= maxResults {
			break
		}
	}
	return out, nil
}

// isHTTPScheme 只认带 http/https scheme 且 host 非空的结果（过滤广告/站内跳转壳）。
func isHTTPScheme(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// matchTarget 判断结果 URL 是否命中目标：域名相等或为其子域（去 www. 前缀）；
// 目标带路径时还要求路径前缀按段命中——GitHub/Gitee 这类共享域名平台上，
// 只比域名会把别人的仓库当成自己的排名。
// 例外：结果 URL 无路径（如百度 showurl 只有 host）时放行域名命中，宁偏松不丢真命中。
func matchTarget(itemURL, targetURL string) bool {
	ih, ip := hostPathOf(itemURL)
	th, tp := hostPathOf(targetURL)
	if ih == "" || th == "" {
		return false
	}
	if ih != th && !strings.HasSuffix(ih, "."+th) {
		return false
	}
	if tp == "" || ip == "" {
		return true
	}
	if ip == tp || strings.HasPrefix(ip, tp+"/") {
		return true
	}
	// 百度/搜狗 showurl 可能把路径截断（github.com/SuperiorChuo/gopherfo…）：
	// 截断串是目标路径前缀也算命中。仅对无 scheme 的展示网址启用——带完整
	// scheme 的真实 URL 不存在截断，放行会让 github.com/superior 这类
	// 恰为前缀的别人主页冒领排名。最小长度防 /s 之类过短前缀误配。
	if strings.Contains(itemURL, "://") {
		return false
	}
	return len(ip) >= 8 && strings.HasPrefix(tp, ip)
}

func hostPathOf(raw string) (host, path string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	h := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	p := strings.TrimSuffix(strings.ToLower(u.Path), "/")
	return h, p
}

// ---------- HTTP 公共件 ----------

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
}

func newHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 12 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

func fetchPage(ctx context.Context, client *http.Client, pageURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}
	return res, nil
}

// jitter 返回 base + rand(0..spread)。
func jitter(base, spread time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(spread)))
}

// KeywordGap 关键词之间的建议延迟。
func KeywordGap() time.Duration { return jitter(5*time.Second, 5*time.Second) }

// PageGap 同引擎翻页之间的建议延迟。
func pageGap() time.Duration { return jitter(2*time.Second, 3*time.Second) }

// trimEllipsis 剥掉展示网址末尾的截断省略号（百度/搜狗既有 ASCII "..." 也有全角 "…"）。
func trimEllipsis(s string) string {
	s = strings.TrimSuffix(s, "...")
	return strings.TrimSuffix(s, "…")
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
