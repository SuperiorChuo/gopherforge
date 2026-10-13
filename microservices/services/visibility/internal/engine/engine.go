// Package engine provides GEO answer generation (stub/LLM) and mention analysis
// (heuristic + LLM structured) for visibility-service.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

// splitAliases 是 model.SplitAliases 的包级别名：本包多个函数用 model 作参数名
// （模型 id），函数体内包名被遮蔽，统一经此引用。
var splitAliases = model.SplitAliases

// llmHTTPClient 进程级共享（复用连接池）。超时由调用方 ctx 控制（各接口
// 40-90 秒不等）；Timeout 只兜底防止无 deadline 的 ctx 挂死。
var llmHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// MentionAnalysis is structured mention flags from an answer text.
type MentionAnalysis struct {
	MentionBrand      bool
	MentionCompetitor bool
	Recommended       bool
	HasLink           bool
	MatchedBrands     []string
	// Sentiment: positive | neutral | negative | ""（启发式分析不填）
	Sentiment string
	// ListPosition: 品牌在回答推荐列表中的位次，0 = 不在列表或无列表
	ListPosition int
}

// AnalyzeMentions scans answer for brand/competitor names and simple recommendation signals.
func AnalyzeMentions(answer string, brands []model.Brand) MentionAnalysis {
	a := MentionAnalysis{}
	if strings.TrimSpace(answer) == "" {
		return a
	}
	lower := strings.ToLower(answer)
	a.HasLink = strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.")

	for _, b := range brands {
		names := append([]string{b.Name}, splitAliases(b.Aliases)...)
		hit := false
		for _, n := range names {
			if n == "" {
				continue
			}
			if MentionedLiterally(answer, n) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		a.MatchedBrands = append(a.MatchedBrands, b.Name)
		if b.Kind == "competitor" {
			a.MentionCompetitor = true
		} else {
			a.MentionBrand = true
		}
	}

	// crude recommendation signals near brand names
	recHints := []string{"推荐", "首选", "建议使用", "更适合", "首选方案", "强烈建议", "recommend", "best choice"}
	if a.MentionBrand {
		for _, h := range recHints {
			hl := strings.ToLower(h)
			idx := 0
			for {
				i := strings.Index(lower[idx:], hl)
				if i < 0 {
					break
				}
				p := idx + i
				// 否定前缀（不推荐/别推荐/避免使用/not recommend）不算正面推荐
				prefix := lower[:p]
				if !strings.HasSuffix(prefix, "不") && !strings.HasSuffix(prefix, "别") &&
					!strings.HasSuffix(prefix, "避免") && !strings.HasSuffix(prefix, "not ") && !strings.HasSuffix(prefix, "n't ") {
					a.Recommended = true
					break
				}
				idx = p + len(hl)
			}
			if a.Recommended {
				break
			}
		}
	}
	return a
}

// MentionedLiterally 判断品牌名是否在回答中【字面】出现：允许大小写、
// 连字符/下划线/空格写法差异（Go Admin Kit ≈ go-admin-kit ≈ GoAdminKit），
// 但要求命中处两侧是词边界——防止 "go-admin" 命中 "go-admin-kit"、
// "django admin" 归一后误命中 "GoAdmin" 这类张冠李戴。
func MentionedLiterally(answer, name string) bool {
	an := strings.ToLower(answer)
	base := strings.ToLower(strings.TrimSpace(name))
	if base == "" {
		return false
	}
	fields := strings.FieldsFunc(base, func(r rune) bool { return r == ' ' || r == '-' || r == '_' })
	variants := map[string]bool{base: true}
	for _, sep := range []string{"", "-", "_", " "} {
		variants[strings.Join(fields, sep)] = true
	}
	for v := range variants {
		if v == "" {
			continue
		}
		for idx := 0; ; {
			i := strings.Index(an[idx:], v)
			if i < 0 {
				break
			}
			s := idx + i
			e := s + len(v)
			beforeOK := s == 0 || !isWordByte(an[s-1])
			afterOK := e >= len(an) || !isWordByte(an[e])
			if beforeOK && afterOK {
				return true
			}
			idx = s + 1
		}
	}
	return false
}

// isWordByte：ASCII 字母/数字/连字符/下划线算词内字符（go-admin-kit 是一个
// 完整 token，"go-admin" 停在 '-' 上属于部分命中）；多字节（CJK 等）与其余符号算边界。
func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
}

// stubAnswerPrefix 演示回答的自述标记。StubAnswer 拼出来的文本必然带上自有
// 品牌名，一旦被当成真实 AI 回答就是直接的误导——所以正文里就写清它是什么，
// 不依赖调用方、前端或某个 mode 字段去免责。
const stubAnswerPrefix = "【演示模拟数据 · 非真实 AI 回答】"

// StubAnswer fabricates a demo engine answer for offline/demo environments.
// 只有演示模式（VISIBILITY_DEMO_MODE）下创建的 stub 检测会走到这里；
// 生产环境不会产生 stub 结果行（见 api.CreateGeoRun）。
func StubAnswer(question string, brands []model.Brand) string {
	return stubAnswerPrefix + stubAnswerBody(question, brands)
}

func stubAnswerBody(question string, brands []model.Brand) string {
	var own, comps []string
	for _, b := range brands {
		if b.Kind == "competitor" {
			comps = append(comps, b.Name)
		} else {
			own = append(own, b.Name)
		}
	}
	ownName := "该产品"
	if len(own) > 0 {
		ownName = own[0]
	}
	comp := "同类方案"
	if len(comps) > 0 {
		comp = comps[0]
	}
	// Deterministic-ish variety from question length
	n := 0
	for _, r := range question {
		if unicode.IsLetter(r) || unicode.Is(unicode.Han, r) {
			n++
		}
	}
	switch n % 3 {
	case 0:
		return fmt.Sprintf("针对「%s」，市场上常见选择包括 %s 与 %s。其中 %s 在中小团队后台场景里被较多提及，可结合官网文档评估是否满足需求。",
			trimQ(question), ownName, comp, ownName)
	case 1:
		return fmt.Sprintf("关于「%s」：更推荐先看开源脚手架与云厂商方案。%s 是可选之一；也有团队对比过 %s。请以实际场景与预算为准。",
			trimQ(question), ownName, comp)
	default:
		return fmt.Sprintf("回答「%s」时：当前公开信息里 %s 出现频率一般，%s 有时也会被提到。建议查阅官方站点与近期评测再做决策。",
			trimQ(question), comp, ownName)
	}
}

func trimQ(q string) string {
	q = strings.TrimSpace(q)
	if len([]rune(q)) > 40 {
		return string([]rune(q)[:40]) + "…"
	}
	return q
}

// GeneratedPrompt is one AI-suggested GEO question (candidate, not yet saved).
type GeneratedPrompt struct {
	Category string `json:"category"`
	Question string `json:"question"`
}

var geoPromptCategories = map[string]bool{"category": true, "comparison": true, "scenario": true, "other": true}

// GenerateGeoPrompts 让大模型根据自有品牌/竞品/行业描述，生成用户真实可能会问 AI 的检测问题。
// count 是期望条数（1-30，越界会夹取）；hint 是可选的行业/受众补充描述。
// 返回的候选已做分类归一、去空、按问题文本去重，但【不落库】——由调用方保存用户勾选的部分。
func GenerateGeoPrompts(ctx context.Context, baseURL, apiKey, model string, brands []model.Brand, hint string, count int) ([]GeneratedPrompt, error) {
	if count <= 0 {
		count = 8
	}
	if count > 30 {
		count = 30
	}
	var own, comps []string
	for _, b := range brands {
		label := b.Name
		if als := strings.TrimSpace(b.Aliases); als != "" {
			label += "（别名: " + als + "）"
		}
		if b.Kind == "competitor" {
			comps = append(comps, label)
		} else {
			own = append(own, label)
		}
	}
	ownStr := strings.Join(own, "；")
	if ownStr == "" {
		ownStr = "（未填写，请围绕行业描述生成）"
	}
	compStr := strings.Join(comps, "；")
	if compStr == "" {
		compStr = "（未填写竞品）"
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "（无额外说明）"
	}

	user := fmt.Sprintf(`【自有品牌】%s
【竞品】%s
【行业/受众补充说明】%s

请站在【潜在用户】的角度，设想他们在向 AI 助手（如 ChatGPT）咨询同类产品/服务时，真实会问出口的问题，用于检测这些品牌在 AI 回答中的出现情况。
要求：
- 生成 %d 个问题，覆盖不同角度，尽量口语化、贴近真实提问，不要直接点名自有品牌（那样测不出自然提及）。
- 每个问题标注分类，只能是：category（品类词，问某类产品有哪些好选择）、comparison（对比题，把几个产品/方案放一起比较）、scenario（场景题，描述一个具体使用场景求推荐）、other（其他）。
- 只输出一个 JSON 数组，每个元素形如 {"category":"category","question":"..."}；不要输出 markdown 代码块、编号或任何其它文字。`,
		ownStr, compStr, hint, count)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的 SEO/GEO 营销顾问，擅长揣摩真实用户会如何向 AI 提问。只输出 JSON 数组，不要输出解释或 markdown。",
		user, 0.7)
	if err != nil {
		return nil, err
	}
	items, err := parseGeneratedPrompts(out)
	if err != nil {
		return nil, err
	}
	result := normalizeGeneratedPrompts(items, count)
	if len(result) == 0 {
		return nil, fmt.Errorf("模型未返回有效问题，请重试")
	}
	return result, nil
}

// SuggestedBrand 是从网站内容中识别出的品牌候选（未落库，由用户确认后写入品牌库）。
type SuggestedBrand struct {
	Name    string `json:"name"`
	Aliases string `json:"aliases"` // 逗号分隔
}

// GenerateGeoPromptsFromSite 基于抓取到的网站文本生成检测问题，并顺带识别站点品牌。
// siteText 由 FetchSiteText 提供；brands/hint/count 语义同 GenerateGeoPrompts。
// 品牌识别失败不视为错误（返回 nil brand），问题为空才报错。
func GenerateGeoPromptsFromSite(ctx context.Context, baseURL, apiKey, model string, brands []model.Brand, siteText, hint string, count int) ([]GeneratedPrompt, *SuggestedBrand, error) {
	if count <= 0 {
		count = 8
	}
	if count > 30 {
		count = 30
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		hint = "（无额外说明）"
	}
	var known []string
	for _, b := range brands {
		known = append(known, b.Name)
	}
	knownStr := strings.Join(known, "；")
	if knownStr == "" {
		knownStr = "（品牌库为空）"
	}

	user := fmt.Sprintf(`【网站内容摘录】
%s

【已录入品牌库】%s
【行业/受众补充说明】%s

任务一：从网站内容判断这个网站属于什么产品/服务，识别它的品牌名（和常见别名，逗号分隔；识别不出就留空）。
任务二：站在【潜在用户】的角度，设想他们在向 AI 助手（如 ChatGPT）咨询这类产品/服务时，真实会问出口的问题，生成 %d 个，用于检测品牌在 AI 回答中的出现情况。
要求：
- 问题覆盖不同角度，口语化、贴近真实提问，不要直接点名该品牌（那样测不出自然提及）。
- 每个问题标注分类，只能是：category（品类词）、comparison（对比题）、scenario（场景题）、other（其他）。
- 只输出一个 JSON 对象：{"brand":{"name":"...","aliases":"别名1,别名2"},"questions":[{"category":"category","question":"..."}]}；不要输出 markdown 代码块或任何其它文字。`,
		siteText, knownStr, hint, count)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个资深的 SEO/GEO 营销顾问，擅长从网站内容提炼定位并揣摩真实用户会如何向 AI 提问。只输出 JSON 对象，不要输出解释或 markdown。",
		user, 0.7)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := parseGeneratedSite(out)
	if err != nil {
		return nil, nil, err
	}
	result := normalizeGeneratedPrompts(parsed.Questions, count)
	if len(result) == 0 {
		return nil, nil, fmt.Errorf("模型未返回有效问题，请重试")
	}
	var brand *SuggestedBrand
	if parsed.Brand != nil && strings.TrimSpace(parsed.Brand.Name) != "" {
		brand = &SuggestedBrand{
			Name:    strings.TrimSpace(parsed.Brand.Name),
			Aliases: strings.TrimSpace(parsed.Brand.Aliases),
		}
	}
	return result, brand, nil
}

// normalizeGeneratedPrompts 去空、按问题文本（小写）去重、分类归一并截断到 count。
func normalizeGeneratedPrompts(items []GeneratedPrompt, count int) []GeneratedPrompt {
	seen := map[string]bool{}
	var result []GeneratedPrompt
	for _, it := range items {
		q := strings.TrimSpace(it.Question)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if seen[key] {
			continue
		}
		seen[key] = true
		cat := strings.TrimSpace(strings.ToLower(it.Category))
		if !geoPromptCategories[cat] {
			cat = "category"
		}
		result = append(result, GeneratedPrompt{Category: cat, Question: q})
		if len(result) >= count {
			break
		}
	}
	return result
}

// parseGeneratedPrompts 宽容解析：剥 markdown 围栏，取首个 [ 到末个 ] 的子串再反序列化。
func parseGeneratedPrompts(out string) ([]GeneratedPrompt, error) {
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json array in llm output")
	}
	var items []GeneratedPrompt
	if err := json.Unmarshal([]byte(s[start:end+1]), &items); err != nil {
		return nil, fmt.Errorf("bad llm json: %w", err)
	}
	return items, nil
}

type generatedSite struct {
	Brand     *SuggestedBrand   `json:"brand"`
	Questions []GeneratedPrompt `json:"questions"`
}

// parseGeneratedSite 宽容解析站点模式输出：取首个 { 到末个 } 的子串反序列化。
func parseGeneratedSite(out string) (*generatedSite, error) {
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json object in llm output")
	}
	var parsed generatedSite
	if err := json.Unmarshal([]byte(s[start:end+1]), &parsed); err != nil {
		return nil, fmt.Errorf("bad llm json: %w", err)
	}
	return &parsed, nil
}

// LLMAnswer calls an OpenAI-compatible chat completions endpoint.
func LLMAnswer(ctx context.Context, baseURL, apiKey, model, question string) (string, error) {
	return chatOnce(ctx, baseURL, apiKey, model,
		"你是一个中立的产品顾问。用中文简洁回答用户关于软件/SaaS选型的问题，可提及真实存在的品牌与链接。",
		question, 0.4)
}

// chatOnce 发起一次非流式 chat completions 调用，返回首个 choice 文本。
func chatOnce(ctx context.Context, baseURL, apiKey, model, system, user string, temperature float64) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("AI_API_KEY empty")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 容忍 OpenAI SDK 惯例的 /v1 结尾（下面自己拼 /v1/...，保留会变成 /v1/v1 打 404）
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	url := baseURL + "/v1/chat/completions"
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": temperature,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := llmHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("llm status %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty llm choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

type llmAnalysisPayload struct {
	MentionBrand      bool     `json:"mention_brand"`
	MentionCompetitor bool     `json:"mention_competitor"`
	MatchedBrands     []string `json:"matched_brands"`
	Recommended       bool     `json:"recommended"`
	Sentiment         string   `json:"sentiment"`
	ListPosition      int      `json:"list_position"`
}

// AnalyzeMentionsLLM 用大模型对回答做结构化提及分析（能理解否定表述与语气）。
// 任何调用/解析失败都返回 error，调用方应降级到启发式 AnalyzeMentions。
func AnalyzeMentionsLLM(ctx context.Context, baseURL, apiKey, model, answer string, brands []model.Brand) (MentionAnalysis, error) {
	var own, comps []string
	for _, b := range brands {
		label := b.Name
		if als := strings.TrimSpace(b.Aliases); als != "" {
			label += "（别名: " + als + "）"
		}
		if b.Kind == "competitor" {
			comps = append(comps, label)
		} else {
			own = append(own, label)
		}
	}
	if len(own) == 0 && len(comps) == 0 {
		return MentionAnalysis{}, fmt.Errorf("no brands to analyze")
	}
	truncated := answer
	if r := []rune(truncated); len(r) > 4000 {
		truncated = string(r[:4000])
	}
	user := fmt.Sprintf(`【自有品牌】%s
【竞品】%s
【待分析回答】
%s

请判断并输出 JSON，字段：
mention_brand: 布尔，回答是否提到任一自有品牌（含别名、大小写变体）
mention_competitor: 布尔，是否提到任一竞品
matched_brands: 字符串数组，实际提到的品牌标准名（用上面给出的名字，不要用别名）
recommended: 布尔，回答是否【正面推荐】自有品牌。注意："不推荐X""避免使用X"等否定表述必须为 false
sentiment: 对自有品牌的整体态度，只能是 "positive" / "neutral" / "negative"；未提及时填 "neutral"
list_position: 整数，若回答给出推荐列表/排序，自有品牌排第几（1 起）；不在列表或无列表填 0`,
		strings.Join(own, "；"), strings.Join(comps, "；"), truncated)

	out, err := chatOnce(ctx, baseURL, apiKey, model,
		"你是一个文本分析器。只输出一个 JSON 对象，不要输出 markdown 代码块、解释或任何其它文字。", user, 0)
	if err != nil {
		return MentionAnalysis{}, err
	}
	payload, err := parseLLMAnalysis(out)
	if err != nil {
		return MentionAnalysis{}, err
	}

	// 词库过滤防幻觉：matched_brands 只保留已知品牌标准名（别名归一到标准名），
	// 且必须通过字面核验——品牌名或其别名真实出现在回答里才算。防止模型把
	// "go-admin" 的提及记到 "go-admin-kit" 头上（相似名 ≠ 同一品牌）。
	std := map[string]string{}
	kind := map[string]string{}
	namesOf := map[string][]string{}
	for _, b := range brands {
		std[strings.ToLower(strings.TrimSpace(b.Name))] = b.Name
		kind[b.Name] = b.Kind
		namesOf[b.Name] = append(namesOf[b.Name], b.Name)
		for _, al := range splitAliases(b.Aliases) {
			std[strings.ToLower(al)] = b.Name
			namesOf[b.Name] = append(namesOf[b.Name], al)
		}
	}
	literallyIn := func(name string) bool {
		for _, n := range namesOf[name] {
			if MentionedLiterally(answer, n) {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	var matched []string
	for _, m := range payload.MatchedBrands {
		name, ok := std[strings.ToLower(strings.TrimSpace(m))]
		if !ok || seen[name] || !literallyIn(name) {
			continue
		}
		seen[name] = true
		matched = append(matched, name)
	}
	a := MentionAnalysis{
		Recommended:   payload.Recommended,
		MatchedBrands: matched,
		Sentiment:     payload.Sentiment,
		ListPosition:  payload.ListPosition,
	}
	// 布尔位只从字面核验通过的名单推导，不采信模型自报的 mention_* 布尔
	for _, name := range matched {
		if kind[name] == "competitor" {
			a.MentionCompetitor = true
		} else {
			a.MentionBrand = true
		}
	}
	if a.Sentiment != "positive" && a.Sentiment != "negative" {
		a.Sentiment = "neutral"
	}
	if a.ListPosition < 0 {
		a.ListPosition = 0
	}
	if !a.MentionBrand {
		a.Recommended = false
		a.ListPosition = 0
		// 与 prompt 约定一致：未提及时态度必须是 neutral（防"未提及但态度正面"的矛盾数据）
		a.Sentiment = "neutral"
	}
	lower := strings.ToLower(answer)
	a.HasLink = strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.")
	return a, nil
}

// parseLLMAnalysis 宽容解析：剥 markdown 围栏，取首个 { 到末个 } 的子串再反序列化。
func parseLLMAnalysis(out string) (llmAnalysisPayload, error) {
	var p llmAnalysisPayload
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return p, fmt.Errorf("no json object in llm output")
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &p); err != nil {
		return p, fmt.Errorf("bad llm json: %w", err)
	}
	return p, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
