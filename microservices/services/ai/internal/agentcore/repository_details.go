package agentcore

import "context"

const detailsEventPageSize = 200

// 默认详情兼容已有调用；事件改为首批，并明确告诉调用方是否还有后续。
func (r *Repository) GetRunDetails(ctx context.Context, id uint64) (*RunDetails, error) {
	return r.GetRunDetailsAfter(ctx, id, 0)
}

func (r *Repository) GetRunDetailsAfter(ctx context.Context, id, afterID uint64) (*RunDetails, error) {
	if afterID > maxDatabaseID {
		return nil, ErrValidation
	}
	run, err := r.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	tenantID := run.TenantID
	details := &RunDetails{Run: *run, EventsNextCursor: afterID}
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, run.AgentDefinitionID).First(&details.Definition).Error; err != nil {
		return nil, normalizeDBError(err)
	}
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, run.AgentVersionID).First(&details.Version).Error; err != nil {
		return nil, normalizeDBError(err)
	}
	// 步骤/工具各受发布上限32约束；异常额外记录必须报错，不能静默截掉损坏账本。
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, id).Order("ordinal ASC").Limit(33).Find(&details.Steps).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, id).Order("id ASC").Limit(33).Find(&details.ToolCalls).Error; err != nil {
		return nil, err
	}
	if len(details.Steps) > 32 || len(details.ToolCalls) > 32 {
		return nil, ErrConflict
	}
	// 恢复可重复生成事件，不能从max_steps推出数量上限。多取一条判断has_more，
	// 游标停在实际返回的末条，下一批不会跳过用于探测的那一条。
	events, _, err := r.ListEventsAfter(ctx, id, afterID, detailsEventPageSize+1)
	if err != nil {
		return nil, err
	}
	details.EventsHasMore = len(events) > detailsEventPageSize
	if details.EventsHasMore {
		events = events[:detailsEventPageSize]
	}
	details.Events = events
	if len(events) > 0 {
		details.EventsNextCursor = events[len(events)-1].ID
	}
	return details, nil
}
