package runner

import (
	"fmt"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"log"
)

// TenantLister 是 cron 需要的额外能力。
type TenantLister interface {
	ListTenantIDsWithTrackingKeywords() ([]uint64, error)
	ListTenantIDsWithActiveGeoPrompts() ([]uint64, error)
}

// RunWeeklyGeo 由 cron 触发：逐租户提交真实 llm 提及检测（异步执行）。
// 未配置 AI 密钥时整轮跳过；某租户已有 running 检测则复用不重复提交。
func (r *Runner) RunWeeklyGeo(tl TenantLister) {
	if r.ResolveAI().APIKey == "" {
		log.Printf("geo cron: AI key not configured, skip")
		return
	}
	tenants, err := tl.ListTenantIDsWithActiveGeoPrompts()
	if err != nil {
		log.Printf("geo cron: list tenants: %v", err)
		return
	}
	log.Printf("geo cron: start, %d tenant(s)", len(tenants))
	for _, tid := range tenants {
		select {
		case <-r.ctx.Done():
			return
		default:
		}
		run, reused, err := r.SubmitGeoRun(tid, 0, "llm", "chatgpt", "定时自动检测")
		if err != nil {
			log.Printf("geo cron: tenant %d: %v", tid, err)
			continue
		}
		if reused {
			log.Printf("geo cron: tenant %d already running run %d, skip", tid, run.ID)
		}
	}
	log.Printf("geo cron: submitted")
}

// RunDailySerp 由 cron 触发：逐租户【串行】采集，保证对搜索引擎的全局请求速率恒定。
// 某租户已有 running 任务则跳过。
func (r *Runner) RunDailySerp(tl TenantLister) {
	tenants, err := tl.ListTenantIDsWithTrackingKeywords()
	if err != nil {
		log.Printf("serp cron: list tenants: %v", err)
		return
	}
	log.Printf("serp cron: start, %d tenant(s)", len(tenants))
	for _, tid := range tenants {
		select {
		case <-r.ctx.Done():
			return
		default:
		}
		task, reused, err := r.submitSerpSync(tid)
		if err != nil {
			log.Printf("serp cron: tenant %d: %v", tid, err)
			continue
		}
		if reused {
			log.Printf("serp cron: tenant %d already running task %d, skip", tid, task.ID)
		}
	}
	log.Printf("serp cron: finished")
}

// submitSerpSync 与 SubmitSerpTask 相同的去重/建任务逻辑，但在当前 goroutine 同步执行。
func (r *Runner) submitSerpSync(tenantID uint64) (*model.SerpTask, bool, error) {
	r.mu.Lock()
	if r.ctx.Err() != nil {
		r.mu.Unlock()
		return nil, false, fmt.Errorf("服务关闭中")
	}
	if id, ok := r.serpActive[tenantID]; ok {
		r.mu.Unlock()
		return &model.SerpTask{ID: id, TenantID: tenantID, Status: "running"}, true, nil
	}
	existing, err := r.store.FindRunningSerpTask(tenantID)
	if err != nil {
		r.mu.Unlock()
		return nil, false, err
	}
	if existing != nil {
		r.mu.Unlock()
		return existing, true, nil
	}
	keywords, err := r.store.ListTrackingKeywords(tenantID, 0)
	if err != nil || len(keywords) == 0 {
		r.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("no tracking keywords")
		}
		return nil, false, err
	}
	task := &model.SerpTask{
		TenantID: tenantID, Trigger: "cron", Status: "running", Total: len(keywords),
	}
	if !r.registerTask() {
		r.mu.Unlock()
		return nil, false, fmt.Errorf("服务关闭中")
	}
	if err := r.store.CreateSerpTask(task); err != nil {
		r.wg.Done()
		r.mu.Unlock()
		return nil, false, err
	}
	r.serpActive[tenantID] = task.ID
	r.mu.Unlock()

	defer r.wg.Done()
	r.execSerpTask(task.ID, tenantID, keywords)
	return task, false, nil
}
