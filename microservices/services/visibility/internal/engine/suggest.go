package engine

// AI suggestion helpers beyond prompt generation: brand/competitor discovery,
// SEO keyword generation, and post-run GEO advice. All reuse chatOnce and the
// tolerant JSON extraction style established in engine.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

// BrandCandidate 是 AI 识别出的品牌/竞品候选（未落库，由用户勾选后写入品牌库）。
type BrandCandidate struct {
	Name    string `json:"name"`
	Aliases string `json:"aliases"` // 逗号分隔
	// Kind: brand | competitor
	Kind    string `json:"kind"`
	Website string `json:"website"`
}

// GenerateBrands 让大模型识别自有品牌与真实存在的竞品。
// siteText 非空时以站点内容为主要素材（识别站点自身品牌 + 同类竞品）；
// 为空时以已录入品牌库中的自有品牌 + hint 为素材（只推荐竞品）。
// known 用于提示模型避开已录入的名字；返回值已去空、按名称去重、kind 归一。
func GenerateBrands(ctx context.Context, baseURL, apiKey, model string, known []model.Brand, siteText, hint string) ([]BrandCandidate, error) {
	var knownNames []string
	var ownNames []string
	for _, b := range known {
		knownNames = append(knownNames, b.Name)
		if b.Kind != "competitor" {
			ownNames = append(ownNames, b.Name)
		}
	}
	knownStr := strings.Join(knownNames, "；")
	if knownStr == "" {
		knownStr = "（空）"
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "（无额外说明）"
	}

	var material string
	if strings.TrimSpace(siteText) != "" {
		material = "【网站内容摘录】\n" + siteText + "\n\n任务：先从网站内容识别这个站点自身的品牌（kind 填 brand），再列出该产品/服务的真实竞品（kind 填 competitor）。"
	} else {
		ownStr := strings.Join(ownNames, "；")
		if ownStr == "" {
			return nil, fmt.Errorf("品牌库为空且未填网址：请先填一个网址，或手动录入自有品牌")
		}
		material = "【自有品牌】" + ownStr + "\n\n任务：围绕上述自有品牌，列出真实存在的竞品（kind 填 competitor）。"
	}

	user := fmt.Sprintf(`%s

【已录入品牌库（不要重复给出）】%s
【行业/受众补充说明】%s

要求：
- 竞品必须是真实存在、可查证的产品/项目，给 4-8 个；没把握的不要编造。
- 每项给出：name（标准名）、aliases（常见别名/简称/英文名，逗号分隔，没有留空）、kind（brand 或 competitor）、website（官网或代码仓库地址，不确定留空）。
- 只输出一个 JSON 数组，元素形如 {"name":"...","aliases":"...","kind":"competitor","website":"https://..."}；不要输出 markdown 代码块或任何其它文字。`,
		material, knownStr, hint)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的市场分析师，熟悉各行业产品与开源项目的竞争格局。只输出 JSON 数组，不要输出解释或 markdown。",
		user, 0.4)
	if err != nil {
		return nil, err
	}
	items, err := parseBrandCandidates(out)
	if err != nil {
		return nil, err
	}
	result := normalizeBrandCandidates(items, known)
	if len(result) == 0 {
		return nil, fmt.Errorf("模型未识别出新的品牌/竞品，请补充说明后重试")
	}
	return result, nil
}

func parseBrandCandidates(out string) ([]BrandCandidate, error) {
	s := stripFence(out)
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json array in llm output")
	}
	var items []BrandCandidate
	if err := json.Unmarshal([]byte(s[start:end+1]), &items); err != nil {
		return nil, fmt.Errorf("bad llm json: %w", err)
	}
	return items, nil
}

// normalizeBrandCandidates 去空、避开已录入名称（含别名）、按名称去重、kind 归一。
func normalizeBrandCandidates(items []BrandCandidate, known []model.Brand) []BrandCandidate {
	seen := map[string]bool{}
	for _, b := range known {
		seen[strings.ToLower(strings.TrimSpace(b.Name))] = true
		for _, al := range splitAliases(b.Aliases) {
			seen[strings.ToLower(al)] = true
		}
	}
	var result []BrandCandidate
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		kind := strings.ToLower(strings.TrimSpace(it.Kind))
		if kind != "brand" {
			kind = "competitor"
		}
		result = append(result, BrandCandidate{
			Name:    name,
			Aliases: strings.TrimSpace(it.Aliases),
			Kind:    kind,
			Website: strings.TrimSpace(it.Website),
		})
	}
	return result
}

// KeywordCandidate 是 AI 生成的 SEO 关键词候选（未落库）。
type KeywordCandidate struct {
	Keyword string `json:"keyword"`
	// Locale: zh-CN | en-US
	Locale string `json:"locale"`
}

// GenerateKeywords 生成用户真实会在搜索引擎输入的关键词。
// siteText 非空时以站点内容为素材，否则用品牌库 + hint。count 1-30 夹取。
func GenerateKeywords(ctx context.Context, baseURL, apiKey, model string, brands []model.Brand, siteText, hint string, count int) ([]KeywordCandidate, error) {
	if count <= 0 {
		count = 10
	}
	if count > 30 {
		count = 30
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "（无额外说明）"
	}

	var material string
	if strings.TrimSpace(siteText) != "" {
		material = "【网站内容摘录】\n" + siteText
	} else {
		var own []string
		for _, b := range brands {
			if b.Kind != "competitor" {
				own = append(own, b.Name)
			}
		}
		ownStr := strings.Join(own, "；")
		if ownStr == "" {
			return nil, fmt.Errorf("品牌库为空且未填网址：请先填一个网址，或手动录入自有品牌")
		}
		material = "【自有品牌】" + ownStr
	}

	user := fmt.Sprintf(`%s

【行业/受众补充说明】%s

请站在【潜在用户】的角度，生成 %d 个他们真实会在搜索引擎（百度/必应/Google）里输入、并且这个产品/服务有机会被搜到的关键词，用于 SEO 排名追踪。
要求：
- 覆盖不同意图：品类词（如"开源后台管理系统"）、长尾场景词、对比词；不要只给品牌名本身。
- 中文站点以中文词为主（locale 填 zh-CN），明显面向海外的补充英文词（locale 填 en-US）。
- 只输出一个 JSON 数组，元素形如 {"keyword":"...","locale":"zh-CN"}；不要输出 markdown 代码块或任何其它文字。`,
		material, hint, count)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的 SEO 顾问，擅长揣摩真实用户的搜索习惯。只输出 JSON 数组，不要输出解释或 markdown。",
		user, 0.6)
	if err != nil {
		return nil, err
	}
	items, err := parseKeywordCandidates(out)
	if err != nil {
		return nil, err
	}
	result := normalizeKeywordCandidates(items, count)
	if len(result) == 0 {
		return nil, fmt.Errorf("模型未返回有效关键词，请重试")
	}
	return result, nil
}

func parseKeywordCandidates(out string) ([]KeywordCandidate, error) {
	s := stripFence(out)
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json array in llm output")
	}
	var items []KeywordCandidate
	if err := json.Unmarshal([]byte(s[start:end+1]), &items); err != nil {
		return nil, fmt.Errorf("bad llm json: %w", err)
	}
	return items, nil
}

func normalizeKeywordCandidates(items []KeywordCandidate, count int) []KeywordCandidate {
	seen := map[string]bool{}
	var result []KeywordCandidate
	for _, it := range items {
		kw := strings.TrimSpace(it.Keyword)
		if kw == "" {
			continue
		}
		key := strings.ToLower(kw)
		if seen[key] {
			continue
		}
		seen[key] = true
		loc := strings.TrimSpace(it.Locale)
		if loc != "en-US" {
			loc = "zh-CN"
		}
		result = append(result, KeywordCandidate{Keyword: kw, Locale: loc})
		if len(result) >= count {
			break
		}
	}
	return result
}

// GeoAdviceBundle is markdown narrative + structured checklist (GEO 收口).
type GeoAdviceBundle struct {
	Markdown string             `json:"markdown"`
	Actions  []model.AdviceItem `json:"actions"`
}

// GenerateGeoAdvice 基于一次已完成检测的结果，生成可执行的 GEO 优化建议（markdown + 清单）。
// 结果逐条压缩成一行摘要喂给模型，控制 token；回答原文不传。
func GenerateGeoAdvice(ctx context.Context, baseURL, apiKey, model string, run *model.GeoRun, results []model.GeoResult, brands []model.Brand) (GeoAdviceBundle, error) {
	var own, comps []string
	var ownSites []string
	for _, b := range brands {
		if b.Kind == "competitor" {
			comps = append(comps, b.Name)
		} else {
			own = append(own, b.Name)
			if w := strings.TrimSpace(b.Website); w != "" {
				ownSites = append(ownSites, w)
			}
		}
	}
	ownStr := strings.Join(own, "；")
	if ownStr == "" {
		ownStr = "（未录入）"
	}
	compStr := strings.Join(comps, "；")
	if compStr == "" {
		compStr = "（未录入竞品）"
	}
	siteStr := strings.Join(ownSites, "；")
	if siteStr == "" {
		siteStr = "（未填官网）"
	}

	var lines []string
	mention := 0
	for _, r := range results {
		// 模型调用失败的兜底假回答没有做提及分析，喂给顾问会被当成"未提到品牌"的假事实
		if r.Engine == "llm_fallback_stub" {
			continue
		}
		if r.MentionBrand {
			mention++
		}
		mark := "未提到品牌"
		if r.MentionBrand {
			mark = "提到品牌"
			if r.ListPosition > 0 {
				mark += fmt.Sprintf("，榜单第%d位", r.ListPosition)
			}
			if r.Sentiment != "" && r.Sentiment != "neutral" {
				mark += "，态度" + map[string]string{"positive": "正面", "negative": "负面"}[r.Sentiment]
			}
		}
		comp := ""
		if r.MentionCompetitor {
			comp = "；提到竞品"
			if names := competitorNames(r.MatchedBrands, brands); names != "" {
				comp += "：" + names
			}
		}
		q := r.Question
		if rr := []rune(q); len(rr) > 60 {
			q = string(rr[:60]) + "…"
		}
		lines = append(lines, fmt.Sprintf("%d. %s → %s%s", len(lines)+1, q, mark, comp))
		if len(lines) >= 50 {
			break
		}
	}
	if len(lines) == 0 {
		return GeoAdviceBundle{}, fmt.Errorf("该检测没有有效结果（模型调用全部失败），请重跑检测")
	}

	rate := mention * 100 / len(lines)
	user := fmt.Sprintf(`【自有品牌】%s
【自有品牌官网/仓库】%s
【竞品】%s
【本次检测】共 %d 个有效问题，%d 个回答提到了自有品牌（提及率 %d%%）
【逐题结果】
%s

以上是"模拟用户向 AI 提问，检查 AI 回答是否提到该品牌"的检测结果。请作为 GEO（生成式引擎优化）顾问，只输出一个 JSON 对象（不要 markdown 围栏、不要其它文字），结构：
{
  "markdown": "给运营者看的完整建议，markdown 字符串。需含 ## 总体诊断 / ## 立即可做 / ## 持续监测 三段，具体可执行。",
  "actions": [
    {
      "title": "短标题（一句话行动）",
      "detail": "怎么做、做成什么样",
      "priority": "high|medium|low",
      "related": "针对哪类未提及/被竞品占据的问题"
    }
  ]
}
要求 actions 4-6 条，priority 以 high 居多；结合官网/仓库形态给建议（GitHub 项目侧重 README/topics/对比文档；产品站侧重 FAQ/对比页/内容页）。`,
		ownStr, siteStr, compStr, len(lines), mention, rate, strings.Join(lines, "\n"))

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的 GEO/内容营销顾问，建议务实、具体、可执行。只输出合法 JSON 对象。", user, 0.5)
	if err != nil {
		return GeoAdviceBundle{}, err
	}
	bundle, err := parseGeoAdviceBundle(out)
	if err != nil {
		// 兼容旧模型只吐 markdown 的情况：整段当 markdown，并从列表行抽 checklist
		md := strings.TrimSpace(stripFence(out))
		// A broken structured response is not a legacy markdown answer.
		// In particular, never persist truncated JSON or a wrong JSON root.
		if looksStructuredAdvice(out, md) {
			return GeoAdviceBundle{}, fmt.Errorf("invalid advice JSON: %w", err)
		}
		if md == "" {
			return GeoAdviceBundle{}, fmt.Errorf("模型未返回建议内容，请重试")
		}
		return GeoAdviceBundle{Markdown: md, Actions: checklistFromMarkdown(md)}, nil
	}
	if strings.TrimSpace(bundle.Markdown) == "" {
		return GeoAdviceBundle{}, fmt.Errorf("模型未返回建议正文，请重试")
	}
	if len(bundle.Actions) == 0 {
		bundle.Actions = checklistFromMarkdown(bundle.Markdown)
	}
	// 规范化 id
	for i := range bundle.Actions {
		if bundle.Actions[i].ID == "" {
			bundle.Actions[i].ID = fmt.Sprintf("a%d", i+1)
		}
		p := strings.ToLower(strings.TrimSpace(bundle.Actions[i].Priority))
		if p != "high" && p != "medium" && p != "low" {
			bundle.Actions[i].Priority = "medium"
		} else {
			bundle.Actions[i].Priority = p
		}
	}
	return bundle, nil
}

// Markdown links/reference definitions are not JSON arrays. Other JSON-looking
// lines (even after prose) must not fall back to persisted legacy Markdown.
func adviceMarkdownLink(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	depth := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++ // Escaped brackets do not delimit the label.
		case '[':
			depth++
		case ']':
			depth--
			if depth != 0 {
				continue
			}
			label := strings.TrimSpace(line[1:i])
			if label == "" {
				return false
			}
			tail := strings.TrimSpace(line[i+1:])
			if strings.HasPrefix(tail, "(") || strings.HasPrefix(tail, "[") || strings.HasPrefix(tail, ":") {
				return true
			}
			// A bare JSON-looking label is not a Markdown shortcut link.
			return tail == "" && label[0] != '{' && label[0] != '[' && label[0] != '"'
		}
	}
	return false
}

func looksStructuredAdvice(raw, md string) bool {
	if json.Valid([]byte(md)) {
		return true
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "```json") || strings.HasPrefix(line, "{") {
			return true
		}
		if start := strings.Index(line, "{"); start >= 0 {
			// JSON permits whitespace between the opening brace and its key.
			rest := strings.TrimSpace(line[start+1:])
			if rest == "" || strings.HasPrefix(rest, "\"") {
				return true
			}
		}
		if start := strings.Index(line, "["); start >= 0 {
			candidate := line[start:]
			if json.Valid([]byte(candidate)) || !adviceMarkdownLink(candidate) {
				return true
			}
		}
	}
	return false
}

func parseGeoAdviceBundle(out string) (GeoAdviceBundle, error) {
	s := stripFence(out)
	start := strings.Index(s, "{")
	// Do not extract an object nested inside an array, including after prose.
	if start >= 0 {
		prefix := strings.TrimSpace(s[:start])
		if strings.HasSuffix(prefix, "[") {
			return GeoAdviceBundle{}, fmt.Errorf("expected advice object, not array")
		}
	}
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return GeoAdviceBundle{}, fmt.Errorf("no json object")
	}
	var raw struct {
		Markdown string `json:"markdown"`
		Actions  []struct {
			Title    string `json:"title"`
			Detail   string `json:"detail"`
			Priority string `json:"priority"`
			Related  string `json:"related"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &raw); err != nil {
		return GeoAdviceBundle{}, err
	}
	var actions []model.AdviceItem
	for i, a := range raw.Actions {
		title := strings.TrimSpace(a.Title)
		if title == "" {
			continue
		}
		actions = append(actions, model.AdviceItem{
			ID:       fmt.Sprintf("a%d", i+1),
			Title:    title,
			Detail:   strings.TrimSpace(a.Detail),
			Priority: strings.TrimSpace(a.Priority),
			Related:  strings.TrimSpace(a.Related),
		})
	}
	return GeoAdviceBundle{Markdown: strings.TrimSpace(raw.Markdown), Actions: actions}, nil
}

// checklistFromMarkdown 从「立即可做」列表行粗抽 checklist（兼容纯 markdown）。
func checklistFromMarkdown(md string) []model.AdviceItem {
	var items []model.AdviceItem
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// - xxx / 1. xxx / * xxx
		for _, prefix := range []string{"- ", "* ", "• "} {
			if strings.HasPrefix(t, prefix) {
				t = strings.TrimSpace(t[len(prefix):])
				break
			}
		}
		if rr := []rune(t); len(rr) > 2 && rr[0] >= '1' && rr[0] <= '9' && (rr[1] == '.' || rr[1] == '、' || rr[1] == ')') {
			t = strings.TrimSpace(string(rr[2:]))
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		// 跳过段落标题行
		if strings.Contains(t, "总体诊断") || strings.Contains(t, "持续监测") || strings.Contains(t, "立即可做") {
			continue
		}
		if len([]rune(t)) < 6 {
			continue
		}
		items = append(items, model.AdviceItem{
			ID:       fmt.Sprintf("m%d", len(items)+1),
			Title:    truncateRunesAdvice(t, 80),
			Detail:   t,
			Priority: "medium",
		})
		if len(items) >= 6 {
			break
		}
	}
	return items
}

func truncateRunesAdvice(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// NamingCollision 一个与自有品牌名容易混淆的名字。
type NamingCollision struct {
	Name string `json:"name"`
	// Risk: high | medium | low
	Risk   string `json:"risk"`
	Reason string `json:"reason"`
}

// NamingSuggestion 一个改名候选。
type NamingSuggestion struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// NamingAnalysis 品牌命名分析结果。
type NamingAnalysis struct {
	// Summary: markdown，总体判断（现名竞争力 + 保留强化还是改名的明确倾向）
	Summary     string             `json:"summary"`
	Collisions  []NamingCollision  `json:"collisions"`
	Suggestions []NamingSuggestion `json:"suggestions"`
}

// AnalyzeBrandNaming 让大模型做品牌命名体检：相似撞名检测（不限于竞品清单，
// 含模型已知的同领域项目/通用词）+ 改名候选 + 保留/改名结论。
// statsText 是最近检测的排行榜摘要（可为空），用于让结论落在真实数据上。
func AnalyzeBrandNaming(ctx context.Context, baseURL, apiKey, model string, brands []model.Brand, statsText string) (*NamingAnalysis, error) {
	var own, comps []string
	for _, b := range brands {
		label := b.Name
		if als := strings.TrimSpace(b.Aliases); als != "" {
			label += "（别名: " + als + "）"
		}
		if w := strings.TrimSpace(b.Website); w != "" {
			label += "（" + w + "）"
		}
		if b.Kind == "competitor" {
			comps = append(comps, label)
		} else {
			own = append(own, label)
		}
	}
	if len(own) == 0 {
		return nil, fmt.Errorf("品牌库还没有自有品牌，请先录入")
	}
	compStr := strings.Join(comps, "；")
	if compStr == "" {
		compStr = "（未录入竞品）"
	}
	if strings.TrimSpace(statsText) == "" {
		statsText = "（暂无检测数据）"
	}

	user := fmt.Sprintf(`【自有品牌】%s
【已录入竞品】%s
【最近 AI 提及检测数据】%s

请给这个品牌做一次"命名体检"，从 GEO（AI 回答中的能见度）与 SEO 角度评估名字本身，输出 JSON 对象，字段：

collisions: 数组，相似撞名检测。找出与自有品牌名（含别名）容易混淆的名字——不限于上面的竞品清单，也包括你知识里同领域的其他项目/产品/通用词。判断依据：互为子串、编辑距离近、大小写或连字符变体、发音相近、同类命名套路。每项 {"name":"...","risk":"high|medium|low","reason":"..."}，reason 说清危害（AI 张冠李戴、提及被稀释、搜索被劫持等）。没有明显撞名就给空数组。
suggestions: 数组，改名候选 3-5 个，{"name":"...","reason":"..."}。要求候选名：全网检索唯一性好、保留领域语义、英文可读利于开源传播；reason 从 GEO/SEO 角度说明为什么这个名字更容易被 AI 和搜索引擎准确关联。若现名已足够独特，可只给 1-2 个备选并在 summary 说明不必改。
summary: 字符串（markdown，两三段）：①现名在 AI 语料中的辨识度判断（结合撞名情况与检测数据）；②明确结论——「保留现名并强化」还是「建议改名」，以及为什么；③若保留，给 1-2 条让 AI 把提及准确记到你头上的做法（如统一书写规范、全称+简称同现）。

只输出一个 JSON 对象，不要输出 markdown 代码块或任何其它文字。`,
		strings.Join(own, "；"), compStr, statsText)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的品牌命名与 SEO/GEO 顾问，熟悉开源项目生态的命名格局。只输出 JSON 对象，不要输出解释或 markdown。",
		user, 0.5)
	if err != nil {
		return nil, err
	}
	res, err := parseNamingAnalysis(out)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(res.Summary) == "" {
		return nil, fmt.Errorf("模型未返回有效分析，请重试")
	}
	return res, nil
}

func parseNamingAnalysis(out string) (*NamingAnalysis, error) {
	s := stripFence(out)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json object in llm output")
	}
	var res NamingAnalysis
	if err := json.Unmarshal([]byte(s[start:end+1]), &res); err != nil {
		return nil, fmt.Errorf("bad llm json: %w", err)
	}
	// risk 归一
	valid := map[string]bool{"high": true, "medium": true, "low": true}
	var cols []NamingCollision
	for _, c := range res.Collisions {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			continue
		}
		c.Risk = strings.ToLower(strings.TrimSpace(c.Risk))
		if !valid[c.Risk] {
			c.Risk = "medium"
		}
		cols = append(cols, c)
	}
	res.Collisions = cols
	var sugs []NamingSuggestion
	for _, sg := range res.Suggestions {
		sg.Name = strings.TrimSpace(sg.Name)
		if sg.Name == "" {
			continue
		}
		sugs = append(sugs, sg)
	}
	res.Suggestions = sugs
	return &res, nil
}

// competitorNames 从 matched_brands 逗号串里筛出竞品名。
func competitorNames(matched string, brands []model.Brand) string {
	isComp := map[string]bool{}
	for _, b := range brands {
		if b.Kind == "competitor" {
			isComp[b.Name] = true
		}
	}
	var names []string
	for _, m := range strings.Split(matched, ",") {
		if t := strings.TrimSpace(m); t != "" && isComp[t] {
			names = append(names, t)
		}
	}
	return strings.Join(names, "、")
}

// stripFence 剥掉可能包裹输出的 markdown 代码块围栏。
func stripFence(out string) string {
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```markdown")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return s
}
