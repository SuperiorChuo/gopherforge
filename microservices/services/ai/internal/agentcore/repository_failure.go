package agentcore

import "gorm.io/gorm"

// 失败收尾不能只信任内存中的当前步骤：进程恢复时该指针可能为空。
// 调用方已锁定Run并核对owner；在同一事务内找出所有running子步骤。
func runningStepsForFailure(tx *gorm.DB, run AgentRun, expected *AgentStep) ([]AgentStep, error) {
	var steps []AgentStep
	if err := tx.Where("tenant_id = ? AND run_id = ? AND status = ?", run.TenantID, run.ID, StepRunning).
		Order("ordinal ASC").Limit(33).Find(&steps).Error; err != nil {
		return nil, err
	}
	// 发布约束最多32步；额外一行用于发现损坏账本，不静默留下部分running记录。
	if len(steps) > 32 {
		return nil, ErrConflict
	}
	if expected != nil {
		found := false
		for _, step := range steps {
			if step.ID == expected.ID {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrLeaseLost
		}
	}
	return steps, nil
}
