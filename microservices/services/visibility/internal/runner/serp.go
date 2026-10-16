package runner

import (
	"fmt"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
	"log"
	"strings"
	"time"
)

// SubmitSerpTask 创建采集任务并异步执行；同租户已有 running 任务时返回该任务且 reused=true。
// keywordID = 0 表示全部 tracking 关键词。
func (r *Runner) SubmitSerpTask(tenantID, userID, keywordID uint64, trigger string) (*model.SerpTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return nil, false, fmt.Errorf("服务关闭中，请稍后重试")
	}
	if id, ok := r.serpActive[tenantID]; ok {
		return &model.SerpTask{ID: id, TenantID: tenantID, Status: "running"}, true, nil
	}
	existing, err := r.store.FindRunningSerpTask(tenantID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}

	keywords, err := r.store.ListTrackingKeywords(tenantID, keywordID)
	if err != nil {
		return nil, false, err
	}
	if len(keywords) == 0 {
		return nil, false, fmt.Errorf("没有可检查的关键词（需为追踪中状态）")
	}

	task := &model.SerpTask{
		TenantID:  tenantID,
		Trigger:   trigger,
		KeywordID: keywordID,
		Status:    "running",
		Total:     len(keywords),
		CreatedBy: userID,
	}
	if !r.registerTask() {
		return nil, false, fmt.Errorf("服务关闭中，请稍后重试")
	}
	if err := r.store.CreateSerpTask(task); err != nil {
		r.wg.Done()
		return nil, false, err
	}
	r.serpActive[tenantID] = task.ID
	go func() {
		defer r.wg.Done()
		r.execSerpTask(task.ID, tenantID, keywords)
	}()
	return task, false, nil
}

// execSerpTask 串行逐关键词跑三引擎（内部含随机延迟，控制对搜索引擎的请求速率）。
func (r *Runner) execSerpTask(taskID, tenantID uint64, keywords []model.Keyword) {
	defer func() {
		r.mu.Lock()
		delete(r.serpActive, tenantID)
		r.mu.Unlock()
	}()
	defer func() {
		if p := recover(); p != nil {
			r.failSerpTask(tenantID, taskID, fmt.Sprintf("internal panic: %v", p))
		}
	}()

	okCount, failCount := 0, 0
	engineOK, engineFail := map[string]int{}, map[string]int{}
	for i, kw := range keywords {
		select {
		case <-r.ctx.Done():
			r.failSerpTask(tenantID, taskID, "服务关闭，任务中断")
			return
		default:
		}
		if i > 0 {
			t := time.NewTimer(serp.KeywordGap())
			select {
			case <-r.ctx.Done():
				// 与循环顶部同构地标 failed 退出：末位关键词若走 continue 会直接
				// 跳出循环、被收尾逻辑标成 done（实际最后一个词没采集）
				t.Stop()
				r.failSerpTask(tenantID, taskID, "服务关闭，任务中断")
				return
			case <-t.C:
			}
		}

		if strings.TrimSpace(kw.TargetURL) == "" {
			failCount++
			if err := r.store.CreateRankSnapshot(&model.RankSnapshot{
				TenantID: tenantID, KeywordID: kw.ID, Engine: "all",
				Error: "未填目标网页，无法匹配排名", TaskID: taskID,
			}); err != nil {
				r.failSerpTask(tenantID, taskID, fmt.Sprintf("create missing-target snapshot: %v", err))
				return
			}
			if err := r.store.UpdateSerpTask(tenantID, taskID, map[string]any{
				"done_count": i + 1, "ok_count": okCount, "fail_count": failCount,
			}); err != nil {
				r.failSerpTask(tenantID, taskID, fmt.Sprintf("update progress: %v", err))
				return
			}
			continue
		}

		results := serp.CheckKeyword(r.ctx, r.collectors, kw.Keyword, kw.TargetURL, r.maxPages)
		if r.failSerpIfClosing(tenantID, taskID) {
			return
		}
		var ranks []store.EngineRank
		for eng, res := range results {
			snap := &model.RankSnapshot{
				TenantID: tenantID, KeywordID: kw.ID, Engine: eng, TaskID: taskID,
			}
			if res.Err != nil {
				snap.Error = taskError(res.Err.Error())
				engineFail[eng]++
			} else {
				engineOK[eng]++
				snap.Rank = res.Rank
				snap.SerpURL = res.SerpURL
				ranks = append(ranks, store.EngineRank{Engine: eng, Rank: res.Rank})
			}
			if err := r.store.CreateRankSnapshot(snap); err != nil {
				r.failSerpTask(tenantID, taskID, fmt.Sprintf("create %s snapshot: %v", eng, err))
				return
			}
		}
		if len(ranks) > 0 {
			if err := r.store.ApplyEngineRanks(tenantID, kw.ID, ranks); err != nil {
				r.failSerpTask(tenantID, taskID, fmt.Sprintf("apply ranks for keyword %d: %v", kw.ID, err))
				return
			}
			okCount++
		} else {
			failCount++
		}
		if err := r.store.UpdateSerpTask(tenantID, taskID, map[string]any{
			"done_count": i + 1, "ok_count": okCount, "fail_count": failCount,
		}); err != nil {
			r.failSerpTask(tenantID, taskID, fmt.Sprintf("update progress: %v", err))
			return
		}
	}
	if r.failSerpIfClosing(tenantID, taskID) {
		return
	}
	if err := r.store.UpdateSerpTask(tenantID, taskID, map[string]any{
		"status": "done", "finished_at": time.Now(),
	}); err != nil {
		r.failSerpTask(tenantID, taskID, fmt.Sprintf("complete task: %v", err))
		return
	}
	// 引擎级成败汇总：任务级 ok_count 只要有一个引擎成功就计 ok，
	// 单引擎被反爬封死（如 so360 连续多日全挂）在任务层完全不可见
	for eng, n := range engineFail {
		if engineOK[eng] == 0 && n > 0 {
			log.Printf("serp task %d: engine %s ALL FAILED (%d/%d) — 可能被反爬封禁", taskID, eng, n, n)
		}
	}
}

func (r *Runner) failSerpIfClosing(tenantID, taskID uint64) bool {
	if r.ctx.Err() == nil {
		return false
	}
	r.failSerpTask(tenantID, taskID, "服务关闭，任务中断")
	return true
}
