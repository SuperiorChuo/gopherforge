package runner

import (
	"context"
	"github.com/go-admin-kit/services/visibility/internal/engine"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// geoGroundingResults 单题接地最多注入的结果数。
	geoGroundingResults = 3
	// geoGroundingCandidates 接地检索先取的结果数（过滤垃圾域名后再截取）。
	geoGroundingCandidates = 10
	// geoGroundingRunes 每个来源正文抓取后的截断长度（控制 prompt 体积）。
	geoGroundingRunes = 2500
)

// groundSources 检索问题并并发抓取结果页正文，返回可用于接地注入的来源。
// 先过滤垃圾域名（字典/视频/门户），搜索失败、被过滤为空或全部抓取失败时
// 返回空切片，调用方据此降级为无来源回答——"接地失败"不是"品牌没被提到"
// 的证据，两者必须分开。
func (r *Runner) groundSources(ctx context.Context, question string, brands []model.Brand) []model.GeoSource {
	items := r.search(ctx, question, geoGroundingCandidates)
	ownSites := ownBrandSites(brands)
	picked := make([]serp.Item, 0, geoGroundingResults)
	for _, it := range items {
		if isJunkSource(it.URL) {
			continue
		}
		picked = append(picked, it)
		if len(picked) >= geoGroundingResults {
			break
		}
	}
	if len(picked) == 0 {
		return nil
	}
	sources := make([]model.GeoSource, 0, len(picked))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, it := range picked {
		wg.Add(1)
		go func(it serp.Item) {
			defer wg.Done()
			fctx, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			text, err := r.fetchText(fctx, it.URL, geoGroundingRunes)
			if err != nil {
				log.Printf("geo grounding fetch %s: %v", it.URL, err)
				return
			}
			mu.Lock()
			sources = append(sources, model.GeoSource{
				Position:  it.Position,
				Title:     it.Title,
				URL:       it.URL,
				OwnDomain: matchesOwnSite(it.URL, ownSites),
				Text:      text,
			})
			mu.Unlock()
		}(it)
	}
	wg.Wait()
	return sources
}

// search 单引擎轻量搜索（供接地）：优先 bing/baidu/duckduckgo（内网可达、中文
// 友好），失败按序换下一个，最多试 2 个；google 内网不可达不参与（serp 注释已载明）。
func (r *Runner) search(ctx context.Context, query string, maxResults int) []serp.Item {
	// 自然语言问题 → 检索关键词：中文长问句直接搜会被 bing 拆成单字锚定
	// （"想"→字典页、门户页），实词串显著提升检索相关性。
	query = engine.BuildSearchQuery(query)
	ordered := r.orderedCollectors()
	if len(ordered) == 0 {
		return nil
	}
	attempts := 0
	for _, c := range ordered {
		if attempts >= 2 {
			break
		}
		attempts++
		sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		items, err := serp.GroundingSearch(sctx, c, query, maxResults)
		cancel()
		if err == nil && len(items) > 0 {
			return items
		}
		if err != nil {
			log.Printf("geo grounding search %s: %v", c.Name(), err)
		}
	}
	return nil
}

// orderedCollectors 把 r.collectors 重排为 [bing, baidu, duckduckgo, 其余]，
// 让接地搜索优先命中内网可达、中文友好的引擎。
func (r *Runner) orderedCollectors() []serp.Collector {
	var prefs, rest []serp.Collector
	for _, c := range r.collectors {
		if containsStr([]string{"bing", "baidu", "duckduckgo"}, c.Name()) {
			prefs = append(prefs, c)
		} else {
			rest = append(rest, c)
		}
	}
	return append(prefs, rest...)
}

// ownBrandSites 收集自有品牌（kind=brand）站点，供来源归因判定"是否自有域名"。
func ownBrandSites(brands []model.Brand) []string {
	var sites []string
	for _, b := range brands {
		if b.Kind == "brand" {
			if s := strings.TrimSpace(b.Website); s != "" {
				sites = append(sites, s)
			}
		}
	}
	return sites
}

// matchesOwnSite 复用 serp 的 SERP 排名命中规则（host+path，GitHub/Gitee 感知）。
func matchesOwnSite(raw string, ownSites []string) bool {
	for _, site := range ownSites {
		if serp.URLMatchesTarget(raw, site) {
			return true
		}
	}
	return false
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// isJunkSource 判定接地检索来源是否垃圾：字典/词典（已知站点或域名含
// zidian/cidian 等拼音关键词）、视频站、游戏门户。这类页面锚定单字或与软件
// 选型无关，注入后只会让模型诚实拒绝，浪费接地机会。
func isJunkSource(raw string) bool {
	h := hostOfURL(raw)
	if h == "" {
		return false
	}
	if junkHostExact[h] {
		return true
	}
	for _, hint := range junkHostHints {
		if strings.Contains(h, hint) {
			return true
		}
	}
	return false
}

func hostOfURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

var (
	// junkHostExact 精确匹配的垃圾来源域名（字典/词典/视频/游戏门户）。
	junkHostExact = map[string]bool{
		"hanyuguoxue.com": true,
		"iciba.com":       true,
		"zdic.net":        true,
		"ufanv.cn":        true,
		"xuewuyou.com":    true,
		"iqiyi.com":       true,
		"bilibili.com":    true,
		"youku.com":       true,
		"youtube.com":     true,
		"douyin.com":      true,
		"mc.163.com":      true,
	}
	// junkHostHints 域名含这些关键词也视为字典/词典站（拼音命名无法穷举）。
	junkHostHints = []string{"zidian", "cidian", "hanzi", "pinyin", "dictionary"}
)
