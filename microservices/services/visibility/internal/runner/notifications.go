package runner

import (
	"context"
	"fmt"
	"github.com/go-admin-kit/services/shared/pkg/notifyclient"
	"github.com/go-admin-kit/services/visibility/internal/store/visibility"
	"log"
	"time"
)

// afterGeoRunDone 检测收尾闭环：自动刷新优化建议 + 结果站内信送达。
// 全程尽力而为——建议生成或通知失败只记日志，不影响 run 状态。
func (r *Runner) afterGeoRunDone(tenantID, runID uint64) {
	ctx, cancel := context.WithTimeout(r.ctx, 200*time.Second)
	defer cancel()
	if _, _, err := r.GenerateGeoAdvice(ctx, tenantID, runID); err != nil {
		log.Printf("geo run %d auto advice: %v", runID, err)
	}
	if !r.notify.Enabled() {
		return
	}
	run, err := r.store.GetGeoRun(tenantID, runID)
	if err != nil {
		return
	}
	results, err := r.store.ListGeoResults(tenantID, runID)
	if err != nil {
		return
	}
	total, mentions := 0, 0
	for _, res := range results {
		if res.Engine == "llm_fallback_stub" {
			continue
		}
		total++
		if res.MentionBrand {
			mentions++
		}
	}
	rate := 0
	if total > 0 {
		rate = mentions * 100 / total
	}
	delta := "首次基线"
	highlight := ""
	if prev, err := r.store.FindPreviousDoneGeoRun(tenantID, runID); err == nil && prev != nil {
		prevRes, err := r.store.ListGeoResults(tenantID, prev.ID)
		if err == nil {
			cmp := store.BuildGeoRunCompare(run, results, prev, prevRes)
			sign := "+"
			if cmp.RateDelta < 0 {
				sign = ""
			}
			delta = fmt.Sprintf("较上次 %s%.0fpp", sign, cmp.RateDelta)
			if n := len(cmp.NewlyMentioned); n > 0 {
				highlight = fmt.Sprintf("，新增 %d 题提及", n)
			}
			if n := len(cmp.NewlyMissed); n > 0 {
				highlight += fmt.Sprintf("，丢失 %d 题", n)
			}
		}
	}
	brand := ""
	if brands, err := r.store.ListAllBrands(tenantID); err == nil {
		for _, b := range brands {
			if b.Kind != "competitor" {
				brand = b.Name
				break
			}
		}
	}
	uids, err := r.store.ListAdminUserIDs(context.Background(), tenantID)
	if err != nil || len(uids) == 0 {
		log.Printf("geo run %d notify: no recipients (%v)", runID, err)
		return
	}
	if _, err := r.notify.Send(ctx, notifyclient.SendInput{
		TenantID:     tenantID,
		UserIDs:      uids,
		TemplateCode: "visibility.geo_weekly",
		Type:         "notice",
		RefType:      "geo_run",
		RefID:        fmt.Sprintf("%d", runID),
		Vars: map[string]string{
			"brand":     brand,
			"rate":      fmt.Sprintf("%d", rate),
			"delta":     delta,
			"mentions":  fmt.Sprintf("%d", mentions),
			"total":     fmt.Sprintf("%d", total),
			"highlight": highlight,
		},
	}); err != nil {
		log.Printf("geo run %d notify: %v", runID, err)
	}
}
