package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-admin-kit/services/visibility/internal/engine"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"log"
	"strings"
	"time"
)

// aiProviderSettingKey 与 ai-service 共用的控制台热配置行（system_settings）。
const aiProviderSettingKey = "ai.provider"

// ResolveAI 返回当前生效的 AI 配置：控制台热配置（ai.provider 行）非空字段
// 覆盖环境变量兜底值。提交跑题时读取，控制台改配置即时生效。
func (r *Runner) ResolveAI() AIConfig {
	cfg := r.ai
	v, err := r.store.GetSettingJSON(aiProviderSettingKey)
	if err != nil || v == nil {
		return cfg
	}
	pick := func(key, cur string) string {
		if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		return cur
	}
	cfg.BaseURL = pick("base_url", cfg.BaseURL)
	cfg.APIKey = pick("api_key", cfg.APIKey)
	cfg.Model = pick("chat_model", cfg.Model)
	return cfg
}

// SetExtraEngines 注入 Perplexity/Gemini 真实引擎配置（未配 key 则对应引擎不可用）。
func (r *Runner) SetExtraEngines(perplexity, gemini AIConfig) {
	r.perplexity = perplexity
	r.gemini = gemini
}

// resolveEngineCfg 返回引擎的调用配置：chatgpt/selfhost 用中转快照（relay），
// perplexity/gemini 用注入配置。key 未配时返回错误（调用方落 llm_fallback_stub）。
func (r *Runner) resolveEngineCfg(name string, relay AIConfig) (engine.Config, error) {
	switch name {
	case "chatgpt", "selfhost":
		return engine.Config{BaseURL: relay.BaseURL, APIKey: relay.APIKey, Model: relay.Model}, nil
	case "perplexity":
		if strings.TrimSpace(r.perplexity.APIKey) == "" {
			return engine.Config{}, fmt.Errorf("未配置 Perplexity API 密钥（PERPLEXITY_API_KEY）")
		}
		return engine.Config{BaseURL: r.perplexity.BaseURL, APIKey: r.perplexity.APIKey, Model: r.perplexity.Model}, nil
	case "gemini":
		if strings.TrimSpace(r.gemini.APIKey) == "" {
			return engine.Config{}, fmt.Errorf("未配置 Gemini API 密钥（GEMINI_API_KEY）")
		}
		return engine.Config{BaseURL: r.gemini.BaseURL, APIKey: r.gemini.APIKey, Model: r.gemini.Model}, nil
	}
	return engine.Config{}, fmt.Errorf("未知引擎 %q", name)
}

// EngineConfigured 判断某真实引擎是否已配置（供 API 校验创建 run 时用）。
func (r *Runner) EngineConfigured(name string) bool {
	switch name {
	case "chatgpt", "selfhost":
		return r.ResolveAI().APIKey != ""
	case "perplexity":
		return strings.TrimSpace(r.perplexity.APIKey) != ""
	case "gemini":
		return strings.TrimSpace(r.gemini.APIKey) != ""
	}
	return false
}

// GeneratePrompts 用当前生效的 AI 配置，根据租户的品牌/竞品生成候选检测问题（同步，不落库）。
// 供 handler 直接调用返回给前端预览；llm 未配置时由 handler 提前拦截。
func (r *Runner) GeneratePrompts(ctx context.Context, tenantID uint64, hint string, count int) ([]engine.GeneratedPrompt, error) {
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, err
	}
	ai := r.ResolveAI()
	return engine.GenerateGeoPrompts(ctx, ai.BaseURL, ai.APIKey, ai.Model, brands, hint, count)
}

// fetchSiteText 抓取站点文本，失败重试一次（GitHub 等站点在内网环境常见间歇可达）；
// 仍失败则降级为"只有网址"的占位素材（网址里的 owner/repo 对大模型已是强信息），
// fetched=false 告知调用方本次是降级生成，应提示用户。
func (r *Runner) fetchSiteText(ctx context.Context, siteURL string) (text string, fetched bool) {
	text, err := engine.FetchSiteText(ctx, siteURL, 6000)
	if err == nil {
		return text, true
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		if text, err = engine.FetchSiteText(ctx, siteURL, 6000); err == nil {
			return text, true
		}
	}
	log.Printf("visibility: fetch %s failed, fallback to url-only material: %v", siteURL, err)
	return "（注意：网页内容抓取失败，只有网址可用：" + siteURL +
		"。若是 GitHub/Gitee 仓库，路径中的 owner/repo 即项目名；请结合你对该公开项目/域名的了解作答，不确定的宁可不给。）", false
}

// GeneratePromptsFromSite 抓取 siteURL 的可读文本后生成候选问题，并顺带识别站点品牌。
// 抓取失败会降级为按网址生成（fetched=false）；品牌识别失败不报错（返回 nil brand）。
func (r *Runner) GeneratePromptsFromSite(ctx context.Context, tenantID uint64, siteURL, hint string, count int) ([]engine.GeneratedPrompt, *engine.SuggestedBrand, bool, error) {
	siteText, fetched := r.fetchSiteText(ctx, siteURL)
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, nil, fetched, err
	}
	ai := r.ResolveAI()
	items, brand, err := engine.GenerateGeoPromptsFromSite(ctx, ai.BaseURL, ai.APIKey, ai.Model, brands, siteText, hint, count)
	return items, brand, fetched, err
}

// GenerateBrands 用 AI 识别品牌/竞品候选（同步，不落库）。siteURL 非空时先抓站，
// 抓取失败降级为按网址生成（fetched=false，调用方据此提示用户）。
func (r *Runner) GenerateBrands(ctx context.Context, tenantID uint64, siteURL, hint string) ([]engine.BrandCandidate, bool, error) {
	siteText, fetched := "", true
	if strings.TrimSpace(siteURL) != "" {
		siteText, fetched = r.fetchSiteText(ctx, siteURL)
	}
	known, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, fetched, err
	}
	ai := r.ResolveAI()
	items, err := engine.GenerateBrands(ctx, ai.BaseURL, ai.APIKey, ai.Model, known, siteText, hint)
	return items, fetched, err
}

// GenerateKeywords 用 AI 生成 SEO 关键词候选（同步，不落库）。siteURL 非空时先抓站，
// 抓取失败降级为按网址生成（fetched=false）。
func (r *Runner) GenerateKeywords(ctx context.Context, tenantID uint64, siteURL, hint string, count int) ([]engine.KeywordCandidate, bool, error) {
	siteText, fetched := "", true
	if strings.TrimSpace(siteURL) != "" {
		siteText, fetched = r.fetchSiteText(ctx, siteURL)
	}
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, fetched, err
	}
	ai := r.ResolveAI()
	items, err := engine.GenerateKeywords(ctx, ai.BaseURL, ai.APIKey, ai.Model, brands, siteText, hint, count)
	return items, fetched, err
}

// GenerateNamingAnalysis 品牌命名体检（相似撞名 + 改名候选 + 结论）。
// statsText 由调用方提供（最近检测排行榜摘要，可为空）。
func (r *Runner) GenerateNamingAnalysis(ctx context.Context, tenantID uint64, statsText string) (*engine.NamingAnalysis, error) {
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, err
	}
	ai := r.ResolveAI()
	return engine.AnalyzeBrandNaming(ctx, ai.BaseURL, ai.APIKey, ai.Model, brands, statsText)
}

// GenerateGeoAdvice 对一次已完成的检测生成优化建议 + 可执行清单并写回（幂等：重复调用覆盖）。
func (r *Runner) GenerateGeoAdvice(ctx context.Context, tenantID, runID uint64) (string, []model.AdviceItem, error) {
	run, err := r.store.GetGeoRun(tenantID, runID)
	if err != nil {
		return "", nil, fmt.Errorf("检测记录不存在")
	}
	if run.Status != "done" {
		return "", nil, fmt.Errorf("检测尚未完成，完成后才能生成建议")
	}
	results, err := r.store.ListGeoResults(tenantID, runID)
	if err != nil {
		return "", nil, err
	}
	if len(results) == 0 {
		return "", nil, fmt.Errorf("该检测没有结果数据")
	}
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return "", nil, err
	}
	ai := r.ResolveAI()
	bundle, err := engine.GenerateGeoAdvice(ctx, ai.BaseURL, ai.APIKey, ai.Model, run, results, brands)
	if err != nil {
		return "", nil, err
	}
	itemsJSON, _ := json.Marshal(bundle.Actions)
	if err := r.store.UpdateGeoRun(tenantID, runID, map[string]any{
		"advice":            bundle.Markdown,
		"advice_items_json": string(itemsJSON),
	}); err != nil {
		return "", nil, err
	}
	return bundle.Markdown, bundle.Actions, nil
}
