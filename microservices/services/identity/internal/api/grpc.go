package api

import (
	"context"

	"gorm.io/gorm"

	identityv1 "github.com/go-admin-kit/services/api/gen/identity/v1"
)

// IdentityGRPC 实现 identityv1.IdentityServiceServer（Phase 3：owner API gRPC 版）。
// 逻辑与 HTTP internal 端点一致（同一 db），供 notify/visibility/crm/bpm 经 Consul+gRPC 调用。
type IdentityGRPC struct {
	identityv1.UnimplementedIdentityServiceServer
	db *gorm.DB
}

// NewIdentityGRPC 创建 gRPC 服务实例。
func NewIdentityGRPC(db *gorm.DB) *IdentityGRPC { return &IdentityGRPC{db: db} }

func (g *IdentityGRPC) BatchUserContacts(ctx context.Context, req *identityv1.BatchUserContactsRequest) (*identityv1.BatchUserContactsResponse, error) {
	var rows []struct {
		ID     uint64
		Email  string
		Phone  string
		Status int
	}
	err := g.db.Table("users").Select("id, email, phone, status").
		Where("id IN ? AND tenant_id = ?", req.Ids, req.TenantId).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := &identityv1.BatchUserContactsResponse{Contacts: map[uint64]*identityv1.Contact{}}
	for _, r := range rows {
		if r.Status != 1 {
			continue
		}
		out.Contacts[r.ID] = &identityv1.Contact{Email: r.Email, Phone: r.Phone}
	}
	return out, nil
}

func (g *IdentityGRPC) BatchUserDisplayNames(ctx context.Context, req *identityv1.BatchUserDisplayNamesRequest) (*identityv1.BatchUserDisplayNamesResponse, error) {
	var rows []struct {
		ID       uint64
		Nickname string
		Username string
	}
	err := g.db.Table("users").Select("id, nickname, username").
		Where("id IN ?", req.Ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := &identityv1.BatchUserDisplayNamesResponse{Names: map[uint64]string{}}
	for _, r := range rows {
		n := r.Nickname
		if n == "" {
			n = r.Username
		}
		out.Names[r.ID] = n
	}
	return out, nil
}

func (g *IdentityGRPC) AdminUserIDs(ctx context.Context, req *identityv1.AdminUserIDsRequest) (*identityv1.AdminUserIDsResponse, error) {
	var ids []uint64
	if err := g.db.Table("users").
		Where("tenant_id = ? AND status = 1 AND is_platform_admin = true", req.TenantId).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return &identityv1.AdminUserIDsResponse{Ids: ids}, nil
}

func (g *IdentityGRPC) UserDepartment(ctx context.Context, req *identityv1.UserDepartmentRequest) (*identityv1.UserDepartmentResponse, error) {
	var deptID uint64
	if err := g.db.Table("users").Select("COALESCE(department_id,0)").
		Where("id = ? AND tenant_id = ?", req.UserId, req.TenantId).
		Limit(1).Scan(&deptID).Error; err != nil {
		return nil, err
	}
	return &identityv1.UserDepartmentResponse{DepartmentId: deptID}, nil
}

func (g *IdentityGRPC) UsersByRoles(ctx context.Context, req *identityv1.UsersByRolesRequest) (*identityv1.UsersByRolesResponse, error) {
	var ids []uint64
	err := g.db.Table("users").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Where("user_roles.role_id IN ?", req.RoleIds).
		Where("users.status = 1 AND users.tenant_id = ?", req.TenantId).
		Order("users.id ASC").Distinct().Pluck("users.id", &ids).Error
	if err != nil {
		return nil, err
	}
	return &identityv1.UsersByRolesResponse{Ids: ids}, nil
}

func (g *IdentityGRPC) UserDataScope(ctx context.Context, req *identityv1.UserDataScopeRequest) (*identityv1.UserDataScopeResponse, error) {
	var scopes []string
	if err := g.db.Table("roles").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ? AND roles.tenant_id = ?", req.UserId, req.TenantId).
		Pluck("roles.data_scope", &scopes).Error; err != nil {
		return nil, err
	}
	ds := "self"
	for _, sc := range scopes {
		if sc == "all" {
			ds = "all"
			break
		}
		if sc == "dept" || sc == "department" || sc == "department_tree" {
			ds = "dept"
		}
	}
	var deptID uint64
	_ = g.db.Table("users").Select("COALESCE(department_id,0)").
		Where("id = ? AND tenant_id = ?", req.UserId, req.TenantId).Limit(1).Scan(&deptID).Error
	return &identityv1.UserDataScopeResponse{DataScope: ds, DepartmentId: deptID}, nil
}

func (g *IdentityGRPC) DepartmentMembers(ctx context.Context, req *identityv1.DepartmentMembersRequest) (*identityv1.DepartmentMembersResponse, error) {
	var ids []uint64
	err := g.db.Raw(`
		WITH RECURSIVE tree AS (
			SELECT id FROM departments WHERE id = ? AND tenant_id = ?
			UNION ALL
			SELECT d.id FROM departments d JOIN tree t ON d.parent_id = t.id WHERE d.tenant_id = ?
		)
		SELECT DISTINCT u.id FROM users u JOIN tree ON u.department_id = tree.id
		WHERE u.tenant_id = ? AND u.status = 1 ORDER BY u.id ASC`,
		req.DepartmentId, req.TenantId, req.TenantId, req.TenantId).Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	return &identityv1.DepartmentMembersResponse{Ids: ids}, nil
}

func (g *IdentityGRPC) DepartmentInfo(ctx context.Context, req *identityv1.DepartmentInfoRequest) (*identityv1.DepartmentInfoResponse, error) {
	var row struct {
		ID           uint64
		Name         string
		LeaderUserID uint64
	}
	err := g.db.Table("departments").Select("id, name, leader_user_id").
		Where("id = ? AND tenant_id = ?", req.Id, req.TenantId).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &identityv1.DepartmentInfoResponse{Id: row.ID, Name: row.Name, LeaderUserId: row.LeaderUserID}, nil
}

func (g *IdentityGRPC) Org(ctx context.Context, req *identityv1.OrgRequest) (*identityv1.OrgResponse, error) {
	var depts []*identityv1.OrgDept
	if err := g.db.Table("departments").Select("id, name").
		Where("status = 1 AND tenant_id = ?", req.TenantId).Order("sort ASC, id ASC").
		Scan(&depts).Error; err != nil {
		return nil, err
	}
	var users []*identityv1.OrgUser
	if err := g.db.Table("users").
		Select("id, COALESCE(NULLIF(nickname,''), username) AS name, department_id").
		Where("status = 1 AND tenant_id = ?", req.TenantId).Order("id ASC").
		Scan(&users).Error; err != nil {
		return nil, err
	}
	return &identityv1.OrgResponse{Depts: depts, Users: users}, nil
}
