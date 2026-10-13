package agentcore

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// 幂等键只属于当前租户/调用人。先读取已提交结果，再检查定义是否允许新建，
// 避免发布版本或禁用定义使网络重试变成另一笔请求。
func (r *Repository) findRunByIdempotencyKey(ctx context.Context, actorID uint64, key string) (*AgentRun, error) {
	tenantID, err := tenantIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var run AgentRun
	err = r.db.WithContext(ctx).Where("tenant_id = ? AND requested_by = ? AND idempotency_key = ?", tenantID, actorID, key).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// 比較不可变的客户端请求字段，而非隐式选择的版本hash。存量Run保留原hash，
// 无需回写账本；事务内竞争与预查命中采用同一判据。
func sameRunRequest(existing, incoming AgentRun) bool {
	return existing.AgentDefinitionID == incoming.AgentDefinitionID &&
		existing.RequestedBy == incoming.RequestedBy && existing.Input == incoming.Input
}
