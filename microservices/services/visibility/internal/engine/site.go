package engine

// Site fetching for URL-based prompt generation: download a page, strip it
// to readable text and feed it to the LLM as context. Kept dependency-free
// (regex-based extraction) — good enough as LLM context, not for rendering.

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const siteFetchLimit = 1 << 20 // 1MB raw HTML cap

// isForbiddenHost 拦 loopback / link-local 的 URL 字面量（含 localhost 及尾点变体）。
// 内网段有意放行——这是鉴权后的管理工具，常指向内部预发站点。
func isForbiddenHost(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return true
	}
	return false
}

// safeTransport 在拨号处校验解析后的真实 IP——URL 字面量检查可被
// 「域名解析到 127.0.0.1」和「30x 重定向到内网」绕过，拨号层天然覆盖两者。
func safeTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if isForbiddenHost(host) {
				return nil, fmt.Errorf("目标地址不允许访问")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ia := range ips {
				if ia.IP.IsLoopback() || ia.IP.IsLinkLocalUnicast() {
					return nil, fmt.Errorf("目标地址不允许访问")
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
}

// siteHTTPClient 进程级共享：Transport 若每次请求新建，连接无法复用，且未设
// IdleConnTimeout 的废弃 Transport 会攒着 keep-alive 连接直到对端关闭。
var siteHTTPClient = &http.Client{Timeout: 20 * time.Second, Transport: safeTransport()}

var (
	// RE2 has no backreferences, so each container tag gets its own pattern.
	reStrip = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script\b.*?</script>`),
		regexp.MustCompile(`(?is)<style\b.*?</style>`),
		regexp.MustCompile(`(?is)<noscript\b.*?</noscript>`),
		regexp.MustCompile(`(?is)<svg\b.*?</svg>`),
		regexp.MustCompile(`(?is)<iframe\b.*?</iframe>`),
	}
	reTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reMeta  = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["']([^"']*)["']`)
	reMeta2 = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']*)["'][^>]+name=["']description["']`)
	reBlock = regexp.MustCompile(`(?i)</?(p|div|li|ul|ol|h[1-6]|tr|br|section|article)[^>]*>`)
	reTag   = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlank = regexp.MustCompile(`[ \t\r\f]+`)
	reLines = regexp.MustCompile(`\n{3,}`)
)

// FetchSiteText downloads rawURL and returns (title+description+body) plain
// text truncated to maxRunes. Only http/https; loopback and link-local
// targets are rejected (cloud-metadata style SSRF), intranet ranges are
// intentionally allowed — this is an authenticated admin tool often pointed
// at internal staging sites.
func FetchSiteText(ctx context.Context, rawURL string, maxRunes int) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("仅支持 http/https 网址")
	}
	if isForbiddenHost(u.Hostname()) {
		return "", fmt.Errorf("该地址不允许抓取")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; gopherforge-visibility/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := siteHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("抓取失败：%w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("抓取失败：HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, siteFetchLimit))
	if err != nil {
		return "", fmt.Errorf("读取页面失败：%w", err)
	}

	text := htmlToText(string(raw))
	if text == "" {
		return "", fmt.Errorf("页面没有可读文本（可能是纯前端渲染站点），换个页面或手动填写说明")
	}
	if maxRunes > 0 {
		if r := []rune(text); len(r) > maxRunes {
			text = string(r[:maxRunes])
		}
	}
	return text, nil
}

// htmlToText strips markup keeping title / meta description up front so the
// most descriptive copy survives truncation.
func htmlToText(h string) string {
	var head []string
	if m := reTitle.FindStringSubmatch(h); m != nil {
		if t := strings.TrimSpace(html.UnescapeString(m[1])); t != "" {
			head = append(head, "【标题】"+t)
		}
	}
	desc := reMeta.FindStringSubmatch(h)
	if desc == nil {
		desc = reMeta2.FindStringSubmatch(h)
	}
	if desc != nil {
		if d := strings.TrimSpace(html.UnescapeString(desc[1])); d != "" {
			head = append(head, "【描述】"+d)
		}
	}

	body := h
	for _, re := range reStrip {
		body = re.ReplaceAllString(body, " ")
	}
	body = reBlock.ReplaceAllString(body, "\n")
	body = reTag.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)
	body = reBlank.ReplaceAllString(body, " ")
	var lines []string
	for _, ln := range strings.Split(body, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	out := strings.TrimSpace(strings.Join(head, "\n") + "\n" + reLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	return strings.TrimSpace(out)
}
