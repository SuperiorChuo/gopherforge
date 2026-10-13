package store

import "context"

// identity helpers 经共享 identityclient（Phase 3：gRPC 优先 + HTTP 回退）。
// identity 不可达时各方法按既有软读惯例降级（放行/空/self）。

// displayNames 批量取用户显示名（COALESCE(nickname, username)）。
func (s *Store) displayNames(ctx context.Context, ids []uint64) map[uint64]string {
	if s.idClient == nil || len(ids) == 0 {
		return map[uint64]string{}
	}
	names, err := s.idClient.BatchUserDisplayNames(ctx, ids)
	if err != nil {
		return map[uint64]string{}
	}
	return names
}

// userInTenant 校验用户属于该租户（分配目标校验）。
func (s *Store) userInTenant(ctx context.Context, userID, tenantID uint64) bool {
	if s.idClient == nil {
		return true // 未配置 → 放行（软读惯例）
	}
	contacts, err := s.idClient.BatchUserContacts(ctx, tenantID, []uint64{userID})
	if err != nil {
		return true
	}
	_, ok := contacts[userID]
	return ok
}

// userDataScope 返回用户最宽数据权限（all/dept/self）+ 部门。
func (s *Store) userDataScope(ctx context.Context, userID, tenantID uint64) (string, uint64) {
	if s.idClient == nil {
		return "self", 0
	}
	scope, deptID, err := s.idClient.UserDataScope(ctx, userID, tenantID)
	if err != nil {
		return "self", 0
	}
	return scope, deptID
}

// deptMembers 返回部门树（含子部门）内全部启用成员。
func (s *Store) deptMembers(ctx context.Context, deptID, tenantID uint64) []uint64 {
	if s.idClient == nil {
		return nil
	}
	ids, err := s.idClient.DepartmentMembers(ctx, deptID, tenantID)
	if err != nil {
		return nil
	}
	return ids
}

// orgData 返回租户组织架构（分配选择器）。
func (s *Store) orgData(ctx context.Context, tenantID uint64) ([]OrgDept, []OrgUser) {
	if s.idClient == nil {
		return nil, nil
	}
	org, err := s.idClient.Org(ctx, tenantID)
	if err != nil {
		return nil, nil
	}
	depts := make([]OrgDept, 0, len(org.GetDepts()))
	for _, d := range org.GetDepts() {
		depts = append(depts, OrgDept{ID: d.GetId(), Name: d.GetName()})
	}
	users := make([]OrgUser, 0, len(org.GetUsers()))
	for _, u := range org.GetUsers() {
		users = append(users, OrgUser{ID: u.GetId(), Name: u.GetName(), DepartmentID: u.GetDepartmentId()})
	}
	return depts, users
}

// deptExists 校验部门属于该租户。
func (s *Store) deptExists(ctx context.Context, deptID, tenantID uint64) bool {
	if s.idClient == nil {
		return true
	}
	_, err := s.idClient.DepartmentInfo(ctx, deptID, tenantID)
	return err == nil
}

// userInTenantActive 校验用户启用 + 属于租户（bpm 同口径）。
func (s *Store) userInTenantActive(ctx context.Context, userID, tenantID uint64) bool {
	return s.userInTenant(ctx, userID, tenantID)
}
