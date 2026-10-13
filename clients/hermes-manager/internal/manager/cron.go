package manager

import (
	"context"
	"strconv"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

// dispatchCron 在每次 Sync 中评估租户下发的 cron 任务（M5），到点把提示词推给
// 本地 Hermes。
//
// 设计决策（与里程碑文档一致）：
//   - 不依赖未验证的上游 `hermes cron`；调度由 manager 在设备端自行评估。
//   - v1 门控 `config.APIServerKey != ""`（loopback API server 只在
//     company_cluster + active 时下发）：cron 仅在 company_cluster 设备执行；
//     员工 company 设备收到 Cron 元数据但 no-op。
//   - 按「分钟槽」幂等：同一 job 同一分钟只推一次（CronLastRuns 落盘，重启安全）。
//     分钟翻篇即跳过不补跑（分频错误/错过由下一周期自然校准）。
//   - 推送失败：记 cron_dispatch 事件、不记槽 → 本分钟内重试，分钟翻篇跳过。
func (m *Manager) dispatchCron(ctx context.Context, desired *protocol.DesiredState, config *state.Config, client *protocol.Client, identity *protocol.Identity) {
	if desired == nil || len(desired.Cron) == 0 || m.adapter == nil {
		return
	}
	// 决策 A 门控：仅在 API server（loopback 推送通道）可用时执行
	if config.ActualState != "active" || config.APIServerKey == "" {
		return
	}
	now := m.now()
	slot := now.Unix() / 60
	for _, job := range desired.Cron {
		if !job.Enabled {
			continue
		}
		spec, err := protocol.ParseCronExpression(job.CronExpression)
		if err != nil {
			// 签名 desired-state 已过 validateCron，这里仅是防御性重解析
			continue
		}
		if !spec.Matches(now) {
			continue
		}
		key := cronSlotKey(job.ID)
		if config.CronLastRuns[key] == slot {
			continue // 本分钟已触发
		}
		task := protocol.DesiredStateTask{
			TaskID:      job.ID,
			Description: job.Prompt,
			Model:       desired.Model.Model,
		}
		result, err := m.adapter.PushTask(ctx, task, config.APIServerKey)
		if err != nil {
			_ = client.SendEvent(ctx, identity, protocol.DeviceEvent{
				EventType:             "cron_dispatch",
				Result:                "failed",
				ErrorCode:             "CRON_PUSH_FAILED",
				DesiredPolicyRevision: desired.Revision,
				AppliedPolicyRevision: config.AppliedPolicyRevision,
				OccurredAt:            now.UTC(),
			})
			continue // 不记槽 → 本分钟内重试；分钟翻篇即跳过
		}
		if config.CronLastRuns == nil {
			config.CronLastRuns = map[string]int64{}
		}
		config.CronLastRuns[key] = slot
		if err := m.store.SaveConfig(config); err != nil {
			return
		}
		if reportErr := client.ReportCronRun(ctx, identity, job.ID, "completed", result); reportErr != nil {
			// 已记槽不再重推；结果丢失留可观测（同 dispatchTask）
			_ = reportErr
		}
	}
}

func cronSlotKey(jobID uint64) string {
	return strconv.FormatUint(jobID, 10)
}
