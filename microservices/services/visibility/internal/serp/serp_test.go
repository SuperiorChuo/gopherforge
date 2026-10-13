package serp

import (
	"strings"
	"testing"
)

func TestMatchTarget(t *testing.T) {
	cases := []struct {
		item, target string
		want         bool
	}{
		{"https://www.example.com/page", "https://example.com", true},
		{"https://example.com/page", "www.example.com", true},
		{"https://blog.example.com/post", "https://example.com", true}, // 子域命中
		{"https://notexample.com/", "https://example.com", false},      // 后缀不是子域
		{"https://example.com.evil.com/", "https://example.com", false},
		{"", "https://example.com", false},
		{"https://example.com", "", false},
		// 目标带路径：共享域名平台（GitHub 等）必须路径前缀命中
		{"https://github.com/SuperiorChuo/gopherforge", "https://github.com/SuperiorChuo/gopherforge", true},
		{"https://github.com/SuperiorChuo/gopherforge/issues", "https://github.com/SuperiorChuo/gopherforge", true},
		{"https://github.com/go-admin-team/go-admin", "https://github.com/SuperiorChuo/gopherforge", false},        // 别人的仓库
		{"https://github.com/SuperiorChuo/gopherforge-docs", "https://github.com/SuperiorChuo/gopherforge", false}, // 前缀但非路径段
		// 大小写不敏感
		{"https://github.com/superiorchuo/GopherForge", "https://github.com/SuperiorChuo/gopherforge", true},
		// 结果只有 host（百度 showurl）：目标带路径时放行域名命中
		{"github.com", "https://github.com/SuperiorChuo/gopherforge", true},
		// showurl 路径被截断：截断串是目标路径前缀 → 命中；过短前缀不放行
		{"github.com/SuperiorChuo/gopherfo", "https://github.com/SuperiorChuo/gopherforge", true},
		{"github.com/Superio", "https://github.com/SuperiorChuo/gopherforge", true},
		{"github.com/S", "https://github.com/SuperiorChuo/gopherforge", false},
		{"github.com/go-admin-te", "https://github.com/SuperiorChuo/gopherforge", false},
		// 截断兜底只对无 scheme 的展示网址生效：带 scheme 的完整 URL 恰为
		// 目标前缀（别人的主页/同前缀仓库）不得冒领
		{"https://github.com/superior", "https://github.com/SuperiorChuo/gopherforge", false},
		{"https://github.com/SuperiorChuo", "https://github.com/SuperiorChuo/gopherforge", false},
	}
	for _, c := range cases {
		if got := matchTarget(c.item, c.target); got != c.want {
			t.Fatalf("matchTarget(%q,%q) = %v, want %v", c.item, c.target, got, c.want)
		}
	}
}

func TestParseSogouEmptyCite(t *testing.T) {
	// 结果块存在但标题链接是跳转链、citeurl 为空：不能 panic，应跳过该条
	html := `<html><body><div class="results">
	<div class="vrwrap"><h3><a href="https://www.sogou.com/link?url=x">题</a></h3><div class="citeurl"></div></div>
	<div class="vrwrap"><h3><a href="https://real.example.com/page">题2</a></h3></div>
	</div></body></html>`
	pos := 0
	items, err := parseSogou(strings.NewReader(html), "https://www.sogou.com/web?query=q", &pos)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].URL != "https://real.example.com/page" {
		t.Fatalf("items = %+v", items)
	}
}

const bingFixture = `<html><body><ol id="b_results">
<li class="b_ad"><h2><a href="https://ad.example.org/x">广告</a></h2></li>
<li class="b_algo"><h2><a href="https://www.first.com/a">First 标题</a></h2></li>
<li class="b_ans">答案卡</li>
<li class="b_algo"><h2><a href="https://blog.example.com/post">Example 博客</a></h2></li>
<li class="b_algo"><h2><a href="https://third.cn/c">Third</a></h2></li>
</ol></body></html>`

func TestParseBing(t *testing.T) {
	pos := 0
	items, err := parseBing(strings.NewReader(bingFixture), &pos)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 organic items, got %d: %+v", len(items), items)
	}
	if items[1].URL != "https://blog.example.com/post" || items[1].Position != 2 {
		t.Fatalf("unexpected item[1]: %+v", items[1])
	}
	rank, serpURL := findTarget(items, "https://example.com")
	if rank != 2 || serpURL != "https://blog.example.com/post" {
		t.Fatalf("rank=%d url=%s", rank, serpURL)
	}
}

func TestParseBingBlocked(t *testing.T) {
	pos := 0
	_, err := parseBing(strings.NewReader(`<html><body><div>empty shell</div></body></html>`), &pos)
	if err == nil {
		t.Fatal("expected error for empty container (blocked), got nil")
	}
}

const baiduFixture = `<html><head><title>关键词_百度搜索</title></head><body><div id="content_left">
<div class="result c-container" mu="https://www.first.com/a"><h3>First</h3></div>
<div class="result" data-tuiguang="1" mu="https://ad.com/x"><h3>广告</h3></div>
<div class="c-container"><h3>ShowURL 兜底</h3><span class="c-showurl">blog.example.com/post...</span></div>
<div class="result c-container"><h3>无法还原</h3></div>
</div></body></html>`

func TestParseBaidu(t *testing.T) {
	pos := 0
	items, err := parseBaidu(strings.NewReader(baiduFixture), "https://www.baidu.com/s?wd=x", &pos)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items (mu + showurl, ad and unrecoverable skipped), got %d: %+v", len(items), items)
	}
	if items[0].URL != "https://www.first.com/a" {
		t.Fatalf("mu attr not used: %+v", items[0])
	}
	rank, _ := findTarget(items, "https://example.com")
	if rank != 2 {
		t.Fatalf("showurl fallback should match example.com at pos 2, got %d", rank)
	}
}

func TestParseBaiduCaptcha(t *testing.T) {
	pos := 0
	if _, err := parseBaidu(strings.NewReader(`<html><head><title>百度安全验证</title></head><body></body></html>`), "https://www.baidu.com/s", &pos); err == nil {
		t.Fatal("expected captcha error")
	}
	if _, err := parseBaidu(strings.NewReader(`<html></html>`), "https://wappass.baidu.com/static/captcha", &pos); err == nil {
		t.Fatal("expected wappass redirect error")
	}
}

const googleFixture = `<html><body><div id="search">
<a href="/url?q=https://www.first.com/a&sa=U"><h3>First</h3></a>
<a href="https://blog.example.com/post"><h3>Example Blog</h3></a>
<a href="https://accounts.google.com/signin">登录（应跳过）</a>
<a href="https://third.cn/c"><h3>Third</h3></a>
</div></body></html>`

func TestParseGoogle(t *testing.T) {
	pos := 0
	items, err := parseGoogle(strings.NewReader(googleFixture), &pos)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d: %+v", len(items), items)
	}
	if items[0].URL != "https://www.first.com/a" {
		t.Fatalf("/url?q= not extracted: %+v", items[0])
	}
	rank, _ := findTarget(items, "example.com")
	if rank != 2 {
		t.Fatalf("want rank 2, got %d", rank)
	}
}

func TestParseGoogleBlocked(t *testing.T) {
	pos := 0
	if _, err := parseGoogle(strings.NewReader(`<html><body>captcha</body></html>`), &pos); err == nil {
		t.Fatal("expected error for missing div#search")
	}
}

func TestExtractGoogleURL(t *testing.T) {
	if got := extractGoogleURL("/url?q=https://a.com/b&sa=U"); got != "https://a.com/b" {
		t.Fatalf("got %q", got)
	}
	if got := extractGoogleURL("https://www.google.com/maps"); got != "" {
		t.Fatalf("google internal link should be skipped, got %q", got)
	}
}
