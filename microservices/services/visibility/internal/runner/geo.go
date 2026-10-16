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

// SubmitGeoRun 创建跑题任务并异步执行；同租户已有 running 任务时返回该任务且 reused=true。
// 调用方需保证 mode 合法、llm 模式下 APIKey 非空。
func (r *Runner) SubmitGeoRun(tenantID, userID uint64, mode, engine, note string) (*model.GeoRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	engine = normalizeEngine(engine)
	// Shutdown 已 cancel 后不得再 wg.Add（WaitGroup 契约），也没意义
	if r.ctx.Err() != nil {
		return nil, false, fmt.Errorf("服务关闭中，请稍后重试")
	}
	if id, ok := r.geoActive[tenantID]; ok {
		return &model.GeoRun{ID: id, TenantID: tenantID, Status: "running"}, true, nil
	}
	// DB 双保险（防御：正常情况下遗留 running 已被启动恢复清掉）。
	// 查询失败时禁止继续创建，否则可能绕过去重产生第二条 running 任务。
	existing, err := r.store.FindRunningGeoRun(tenantID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}

	prompts, err := r.store.ListActiveGeoPrompts(tenantID)
	if err != nil {
		return nil, false, err
	}
	if len(prompts) == 0 {
		return nil, false, fmt.Errorf("请先添加检测问题")
	}
	brands, err := r.store.ListAllBrands(tenantID)
	if err != nil {
		return nil, false, err
	}

	run := &model.GeoRun{
		TenantID:    tenantID,
		Mode:        mode,
		Engine:      engine,
		Status:      "running",
		PromptCount: len(prompts),
		Note:        strings.TrimSpace(note),
		CreatedBy:   userID,
	}
	if !r.registerTask() {
		return nil, false, fmt.Errorf("服务关闭中，请稍后重试")
	}
	if err := r.store.CreateGeoRun(run); err != nil {
		r.wg.Done()
		return nil, false, err
	}
	r.geoActive[tenantID] = run.ID
	ai := r.ResolveAI() // 提交时快照热配置，整个 run 使用同一套
	go r.execGeoRun(run.ID, tenantID, mode, engine, ai, prompts, brands)
	return run, false, nil
}

func (r *Runner) execGeoRun(runID, tenantID uint64, mode, engineName string, ai AIConfig, prompts []model.GeoPrompt, brands []model.Brand) {
	defer r.wg.Done()
	defer func() {
		r.mu.Lock()
		delete(r.geoActive, tenantID)
		r.mu.Unlock()
	}()
	defer func() {
		if p := recover(); p != nil {
			r.failGeoRun(tenantID, runID, fmt.Sprintf("internal panic: %v", p))
		}
	}()

	mention := 0
	for i, p := range prompts {
		select {
		case <-r.ctx.Done():
			r.failGeoRun(tenantID, runID, "服务关闭，任务中断")
			return
		default:
		}

		answer := ""
		resEngine := mode
		var sources []model.GeoSource
		if mode == "llm" {
			eng := engine.NewEngine(engineName)
			cfg, cfgErr := r.resolveEngineCfg(engineName, ai)
			var ans engine.EngineAnswer
			var err error
			if cfgErr != nil {
				err = cfgErr
			} else if engineName == "chatgpt" {
				// 真实 ChatGPT 联网搜索：自备检索、自备引用。实测简单题 ~10s、
				// 复杂多约束题 ~3min（中转上游做多次 web_search 深度检索），
				// 预算放宽到 240s，否则复杂题全超时降级成假数据。
				ctx, cancel := context.WithTimeout(r.ctx, 240*time.Second)
				ans, err = eng.Ask(ctx, cfg, p.Question, nil)
				cancel()
			} else if engineName == "selfhost" {
				// 自建接地：bing 检索 → 抓页注入 → RAG 回答，预算 75s
				ctx, cancel := context.WithTimeout(r.ctx, 75*time.Second)
				injected := r.groundSources(ctx, p.Question, brands)
				ans, err = eng.Ask(ctx, cfg, p.Question, injected)
				cancel()
			} else {
				// perplexity / gemini：真实引擎自备检索、自备引用，预算 90s
				ctx, cancel := context.WithTimeout(r.ctx, 90*time.Second)
				ans, err = eng.Ask(ctx, cfg, p.Question, nil)
				cancel()
			}
			if err != nil {
				// 服务关闭导致的失败：直接标 failed 退出，别写"模型调用失败"的垃圾结果行
				if r.failGeoIfClosing(tenantID, runID) {
					return
				}
				// 只落错误本身。曾经这里会补一段 StubAnswer 假回答"让结果行有内容"，
				// 但那段模板必然带自有品牌名——失败的检测反而看起来像被 AI 提到了。
				// engine 值沿用 llm_fallback_stub：历史行是这个值，全站排除口径
				// （Overview / 排行榜 / 历史对比 / 建议）都按它过滤，改名会漏掉老数据。
				answer = "（模型调用失败：" + err.Error() + "）"
				resEngine = "llm_fallback_stub"
				sources = nil
			} else {
				answer = ans.Answer
				resEngine = engineName // chatgpt | selfhost
				sources = ans.Sources
				// 补自有域名判定（真实引擎的引用也归到自有/竞品站点）
				if own := ownBrandSites(brands); len(own) > 0 {
					for i := range sources {
						if sources[i].URL != "" {
							sources[i].OwnDomain = matchesOwnSite(sources[i].URL, own)
						}
					}
				}
				if len(ans.Queries) > 0 {
					log.Printf("geo run %d prompt %d [%s] search queries: %v", runID, p.ID, engineName, ans.Queries)
				}
			}
		} else {
			answer = engine.StubAnswer(p.Question, brands)
		}

		var an engine.MentionAnalysis
		analyzeBy := "heuristic"
		if resEngine == "llm_fallback_stub" {
			// 失败占位行不参与提及分析：不产生任何提及数据，也不进提及率与排行榜。
			analyzeBy = "skipped"
		} else if mode == "llm" && isRealEngine(resEngine) {
			ctx, cancel := context.WithTimeout(r.ctx, 20*time.Second)
			llmAn, err := engine.AnalyzeMentionsLLM(ctx, ai.BaseURL, ai.APIKey, ai.Model, answer, brands)
			cancel()
			if r.failGeoIfClosing(tenantID, runID) {
				return
			}
			if err != nil {
				log.Printf("geo run %d prompt %d llm analysis fallback: %v", runID, p.ID, err)
				an = engine.AnalyzeMentions(answer, brands)
			} else {
				an = llmAn
				analyzeBy = "llm"
			}
		} else {
			an = engine.AnalyzeMentions(answer, brands)
		}
		// 接地行的 has_link 重定义：只认"引用了真实检索来源"（Cited）。
		// 回答里编造的链接字符串不再计分；无来源降级行恒为 false，诚实呈现——
		// 裸模型回答里出现 URL 是幻觉，不算"被 AI 引用"。
		if isRealEngine(resEngine) {
			an.HasLink = false
			for _, s := range sources {
				if s.Cited {
					an.HasLink = true
					break
				}
			}
		}
		var sourcesJSON string
		if len(sources) > 0 {
			if raw, err := json.Marshal(sources); err == nil {
				sourcesJSON = string(raw)
			}
		}
		resultErr := r.store.CreateGeoResult(&model.GeoResult{
			TenantID:          tenantID,
			RunID:             runID,
			PromptID:          p.ID,
			Question:          p.Question,
			Answer:            answer,
			MentionBrand:      an.MentionBrand,
			MentionCompetitor: an.MentionCompetitor,
			Recommended:       an.Recommended,
			HasLink:           an.HasLink,
			MatchedBrands:     strings.Join(an.MatchedBrands, ","),
			Engine:            resEngine,
			Sentiment:         an.Sentiment,
			ListPosition:      an.ListPosition,
			AnalyzeBy:         analyzeBy,
			SourcesJSON:       sourcesJSON,
		})
		if r.failGeoIfClosing(tenantID, runID) {
			return
		}
		if resultErr != nil {
			r.failGeoRun(tenantID, runID, fmt.Sprintf("create result for prompt %d: %v", p.ID, resultErr))
			return
		}
		if an.MentionBrand {
			mention++
		}
		if err := r.store.UpdateGeoRun(tenantID, runID, map[string]any{
			"done_count": i + 1, "mention_count": mention,
		}); err != nil {
			r.failGeoRun(tenantID, runID, fmt.Sprintf("update progress: %v", err))
			return
		}
	}
	if r.failGeoIfClosing(tenantID, runID) {
		return
	}
	if err := r.store.UpdateGeoRun(tenantID, runID, map[string]any{
		"status": "done", "mention_count": mention, "finished_at": time.Now(),
	}); err != nil {
		r.failGeoRun(tenantID, runID, fmt.Sprintf("complete run: %v", err))
		return
	}
	if mode == "llm" {
		// 独立 goroutine：建议生成最长 200s，若同步执行会拖住 geoActive 占坑释放，
		// 期间新提交拿到"已在进行"的假 running 占位。关闭开始后不再派生
		// 新的 after-hook。
		if r.registerTask() {
			go func() {
				defer r.wg.Done()
				defer func() {
					if p := recover(); p != nil {
						log.Printf("geo run %d after-hook panic: %v", runID, p)
					}
				}()
				r.afterGeoRunDone(tenantID, runID)
			}()
		}
	}
}

func (r *Runner) failGeoIfClosing(tenantID, runID uint64) bool {
	if r.ctx.Err() == nil {
		return false
	}
	r.failGeoRun(tenantID, runID, "服务关闭，任务中断")
	return true
}

// normalizeEngine 把请求/老数据里的引擎值归一为当前枚举：chatgpt | selfhost |
// perplexity | gemini。空值或未知值都落到 selfhost（老 run 引擎列是空的，
// 等价于自建接地）。
func normalizeEngine(e string) string {
	switch strings.TrimSpace(e) {
	case "chatgpt", "perplexity", "gemini":
		return strings.TrimSpace(e)
	default:
		return "selfhost"
	}
}

// isRealEngine 判断结果行是否来自真实生成式引擎（参与提及分析 / has_link 计算）。
// "llm" 是历史值（老 selfhost run 的 engine 列），chatgpt/selfhost 是当前真实引擎值。
func isRealEngine(e string) bool {
	switch e {
	case "llm", "chatgpt", "selfhost", "perplexity", "gemini":
		return true
	}
	return false
}
