package engine

import "context"

// identity helpers 经共享 identityclient（Phase 3：gRPC 优先 + HTTP 回退）。
// identity 不可达时按软读惯例降级（见各方法注释）。

// userInTenantActive 校验用户启用且属于该租户（部门主管有效性校验）。
func (e *Engine) userInTenantActive(ctx context.Context, userID, tenantID uint64) bool {
	if e.idClient == nil {
		return true
	}
	contacts, err := e.idClient.BatchUserContacts(ctx, tenantID, []uint64{userID})
	if err != nil {
		return true
	}
	_, ok := contacts[userID]
	return ok
}

// deptLeader 返回部门主管 user_id（bpm RuleDeptLeader）。不存在/非本租户 → 0。
func (e *Engine) deptLeader(ctx context.Context, deptID, tenantID uint64) uint64 {
	if e.idClient == nil {
		return 0
	}
	info, err := e.idClient.DepartmentInfo(ctx, deptID, tenantID)
	if err != nil {
		return 0
	}
	return info.GetLeaderUserId()
}

// usersByRoles 返回租户内启用且拥有指定角色的用户 id（bpm RuleRoles 审批人解析）。
func (e *Engine) usersByRoles(ctx context.Context, roleIDs []uint64, tenantID uint64) []uint64 {
	if e.idClient == nil || len(roleIDs) == 0 {
		return nil
	}
	ids, err := e.idClient.UsersByRoles(ctx, tenantID, roleIDs)
	if err != nil {
		return nil
	}
	return ids
}

// userDepartment 返回用户所属部门（bpm 发起人部门兜底）。不存在 → 0。
func (e *Engine) userDepartment(ctx context.Context, userID, tenantID uint64) uint64 {
	if e.idClient == nil {
		return 0
	}
	deptID, err := e.idClient.UserDepartment(ctx, userID, tenantID)
	if err != nil {
		return 0
	}
	return deptID
}
