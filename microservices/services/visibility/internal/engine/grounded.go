package engine

// Grounded (RAG 式) answer generation: the model answers based on injected
// retrieved sources and cites them as [n], so mention / citation signals map to
// real web retrieval instead of parametric memory. Extraction of the [n] marks
// back onto sources lives here too.

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

// GroundedLLMAnswer 让模型基于【检索来源】回答（接地），要求句末用 [n] 标注引用。
// sources 由调用方注入（含抓取正文），本函数只拼 prompt 并复用 chatOnce 调用。
// 返回模型回答原文；引用解析由 ExtractCitations 负责。
func GroundedLLMAnswer(ctx context.Context, baseURL, apiKey, model, question string, sources []model.GeoSource) (string, error) {
	grounding, err := buildGroundedContext(sources)
	if err != nil {
		return "", err
	}
	system := "你是一个中立的产品顾问。基于提供的【检索来源】回答用户关于软件/SaaS 选型的问题。" +
		"引用某个来源时，在对应句子末尾标注它的编号 [n]；只能引用提供的来源，" +
		"不要编造来源里没有的信息，也不要编造或猜测链接。"
	user := grounding + "\n\n问题：" + strings.TrimSpace(question)
	return chatOnce(ctx, baseURL, apiKey, model, system, user, 0.3)
}

// buildGroundedContext 把注入的来源拼成编号上下文块，如：
// [1] <title>
// 链接：<url>
// 内容：<text>
func buildGroundedContext(sources []model.GeoSource) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("无检索来源")
	}
	var b strings.Builder
	b.WriteString("【检索来源】")
	for i, s := range sources {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			text = "（页面无可读文本，仅有链接）"
		}
		fmt.Fprintf(&b, "\n[%d] %s\n链接：%s\n内容：%s", i+1, strings.TrimSpace(s.Title), strings.TrimSpace(s.URL), text)
	}
	return b.String(), nil
}

var (
	// searchQuerySplitRE 按中文/英文标点切分问句成子句。
	searchQuerySplitRE = regexp.MustCompile(`[，,、；;。！!]+`)
	// searchQueryQWordRE 命中即视为"客套提问句"（可整句丢弃）。
	searchQueryQWordRE = regexp.MustCompile(`(有哪些|有什么|有没有|什么|哪种|哪套|哪个|哪一个|怎么|如何|为什么|推荐|选择|合适|比较|吗|呢)`)
	// searchQueryFraming 开头提问框架，最长优先剥离。
	searchQueryFraming = []string{
		"我想", "想找一套", "想找一个", "想找", "想", "请问", "求推荐", "推荐一个",
		"有没有", "有哪些", "有什么", "帮我推荐", "帮我", "麻烦推荐", "问一下",
	}
	// searchQueryFillers 全局语气/提问填充词，替换成空格后折叠（保留实词）。
	searchQueryFillers = []string{
		"有没有", "有哪些", "有什么", "什么", "哪些", "哪种", "哪套", "哪个", "哪一个",
		"怎么", "如何", "为什么", "推荐", "比较", "选择", "合适", "一下", "的话",
		"这种", "那种", "一种", "一个", "一套", "请问", "吗", "呢", "吧",
	}
)

// BuildSearchQuery 把自然语言问题转成检索用关键词串。中文检索（尤其 bing）会
// 锚定"想/哪些/什么"这类短单字，返回字典/门户垃圾页；实词串能显著提升相关性。
// 规则偏保守：宁可多留实词，也不误删产品/品类词；清理后过短则回退原文。
func BuildSearchQuery(question string) string {
	orig := strings.TrimSpace(question)
	q := strings.TrimRight(orig, "？?")
	for _, pre := range searchQueryFraming {
		if strings.HasPrefix(q, pre) {
			q = strings.TrimSpace(q[len(pre):])
			break
		}
	}
	// 按标点切句；末句剥掉提问词后若无实词（"有哪些比较成熟的选择"这类纯客套尾句）
	// 且前面已有实句 → 丢弃；有实词（"可扩展开源方案"）则保留，交给全局清理。
	clauses := searchQuerySplitRE.Split(q, -1)
	kept := make([]string, 0, len(clauses))
	for i, cl := range clauses {
		c := strings.TrimSpace(cl)
		if c == "" {
			continue
		}
		last := i == len(clauses)-1
		if last && len(kept) > 0 {
			withoutQ := strings.TrimSpace(searchQueryQWordRE.ReplaceAllString(c, ""))
			if len([]rune(withoutQ)) < 4 {
				continue
			}
		}
		kept = append(kept, c)
	}
	q = strings.Join(kept, " ")
	for _, w := range searchQueryFillers {
		q = strings.ReplaceAll(q, w, " ")
	}
	cleaned := strings.Join(strings.Fields(q), " ")
	if len([]rune(cleaned)) < 4 {
		return strings.TrimRight(orig, "？?") // 清过头就回退原文，别把查询清空
	}
	return cleaned
}

var reCitation = regexp.MustCompile(`\[(\d{1,3})\]`)

// ExtractCitations 解析回答里的 [n] 引用标记（越界忽略），并对每个来源 URL 做
// 字面命中兜底（模型偶尔直接贴 URL 而不是用 [n]）。返回带 Cited 标志的完整来源切片，
// 顺序与输入一致，方便调用方直接落库。
func ExtractCitations(answer string, sources []model.GeoSource) []model.GeoSource {
	if len(sources) == 0 || strings.TrimSpace(answer) == "" {
		return sources
	}
	cited := map[int]bool{}
	for _, m := range reCitation.FindAllStringSubmatch(answer, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= len(sources) {
			cited[n] = true
		}
	}
	lower := strings.ToLower(answer)
	out := make([]model.GeoSource, len(sources))
	for i, s := range sources {
		c := s
		if cited[i+1] || urlLiterallyIn(lower, s.URL) {
			c.Cited = true
		}
		out[i] = c
	}
	return out
}

// urlLiterallyIn 判断 URL 是否字面出现在回答里：忽略 scheme 与结尾斜杠
// （回答常写 "github.com/x/y" 而来源是 "https://github.com/x/y/"）。
func urlLiterallyIn(lowerAnswer, raw string) bool {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.TrimPrefix(key, "https://")
	key = strings.TrimPrefix(key, "http://")
	key = strings.TrimSuffix(key, "/")
	if key == "" {
		return false
	}
	return strings.Contains(lowerAnswer, key)
}
