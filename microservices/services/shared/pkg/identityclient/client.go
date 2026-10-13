// Package identityclient 共享 identity gRPC 客户端（Phase 3）。
// 经 Consul 发现 identity-service → gRPC 调 owner API；gRPC 不可达回退 HTTP 内部端点。
// 供 notify/visibility/crm/bpm 等消费方使用，统一内部调用通道。
package identityclient

import (
	"context"
	"time"

	identityv1 "github.com/go-admin-kit/services/api/gen/identity/v1"
	"github.com/go-admin-kit/services/shared/pkg/grpcx"
	"github.com/go-admin-kit/services/shared/pkg/resilience"
)

// Client identity owner API 客户端。
type Client struct {
	// HTTP 回退配置（Phase 2C 实现）。
	httpBase       string
	internalToken  string
	consulResolver *grpcx.Resolver
	pool           *grpcx.ConnPool     // Phase 4 收尾：gRPC 连接复用（全端点共享）
	breaker        *resilience.Breaker // Phase 4：gRPC 路径熔断器（共享全端点）
}

// New 创建客户端。httpBase 空则禁用 HTTP 回退（仅 gRPC）。
func New(httpBase, internalToken string) (*Client, error) {
	resolver, err := grpcx.NewResolver("")
	if err != nil {
		return nil, err
	}
	return &Client{
		httpBase:       httpBase,
		internalToken:  internalToken,
		consulResolver: resolver,
		pool:           grpcx.NewConnPool(resolver, "identity-service"),
		breaker:        resilience.NewBreaker(5, 10*time.Second),
	}, nil
}

// BatchUserContacts 批量取用户联系方式 + 租户归属。
func (c *Client) BatchUserContacts(ctx context.Context, tenantID uint64, ids []uint64) (map[uint64]*identityv1.Contact, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.BatchUserContactsResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).BatchUserContacts(ctx, &identityv1.BatchUserContactsRequest{TenantId: tenantID, Ids: ids})
	}); err == nil {
		return resp.GetContacts(), nil
	}
	return c.httpContacts(ctx, tenantID, ids)
}

// BatchUserDisplayNames 批量取用户显示名。
func (c *Client) BatchUserDisplayNames(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.BatchUserDisplayNamesResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).BatchUserDisplayNames(ctx, &identityv1.BatchUserDisplayNamesRequest{Ids: ids})
	}); err == nil {
		return resp.GetNames(), nil
	}
	return c.httpDisplayNames(ctx, ids)
}

// AdminUserIDs 返回租户内启用+平台管理员 id。
func (c *Client) AdminUserIDs(ctx context.Context, tenantID uint64) ([]uint64, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.AdminUserIDsResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).AdminUserIDs(ctx, &identityv1.AdminUserIDsRequest{TenantId: tenantID})
	}); err == nil {
		return resp.GetIds(), nil
	}
	return c.httpAdminIDs(ctx, tenantID)
}

// UserDepartment 返回用户所属部门。
func (c *Client) UserDepartment(ctx context.Context, userID, tenantID uint64) (uint64, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.UserDepartmentResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).UserDepartment(ctx, &identityv1.UserDepartmentRequest{UserId: userID, TenantId: tenantID})
	}); err == nil {
		return resp.GetDepartmentId(), nil
	}
	return c.httpUserDepartment(ctx, userID, tenantID)
}

// UsersByRoles 返回拥有指定角色的启用用户 id。
func (c *Client) UsersByRoles(ctx context.Context, tenantID uint64, roleIDs []uint64) ([]uint64, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.UsersByRolesResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).UsersByRoles(ctx, &identityv1.UsersByRolesRequest{TenantId: tenantID, RoleIds: roleIDs})
	}); err == nil {
		return resp.GetIds(), nil
	}
	return c.httpUsersByRoles(ctx, tenantID, roleIDs)
}

// UserDataScope 返回用户最宽数据权限 + 部门。
func (c *Client) UserDataScope(ctx context.Context, userID, tenantID uint64) (string, uint64, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.UserDataScopeResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).UserDataScope(ctx, &identityv1.UserDataScopeRequest{UserId: userID, TenantId: tenantID})
	}); err == nil {
		return resp.GetDataScope(), resp.GetDepartmentId(), nil
	}
	return c.httpUserDataScope(ctx, userID, tenantID)
}

// DepartmentMembers 返回部门树内全部启用成员。
func (c *Client) DepartmentMembers(ctx context.Context, deptID, tenantID uint64) ([]uint64, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.DepartmentMembersResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).DepartmentMembers(ctx, &identityv1.DepartmentMembersRequest{DepartmentId: deptID, TenantId: tenantID})
	}); err == nil {
		return resp.GetIds(), nil
	}
	return c.httpDepartmentMembers(ctx, deptID, tenantID)
}

// DepartmentInfo 返回部门信息（含主管）。
func (c *Client) DepartmentInfo(ctx context.Context, id, tenantID uint64) (*identityv1.DepartmentInfoResponse, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.DepartmentInfoResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).DepartmentInfo(ctx, &identityv1.DepartmentInfoRequest{Id: id, TenantId: tenantID})
	}); err == nil {
		return resp, nil
	}
	return c.httpDepartmentInfo(ctx, id, tenantID)
}

// Org 返回租户组织架构。
func (c *Client) Org(ctx context.Context, tenantID uint64) (*identityv1.OrgResponse, error) {
	if resp, err := grpcCall(ctx, c, func(ctx context.Context, conn *grpcxConn) (*identityv1.OrgResponse, error) {
		return identityv1.NewIdentityServiceClient(conn.Conn).Org(ctx, &identityv1.OrgRequest{TenantId: tenantID})
	}); err == nil {
		return resp, nil
	}
	return c.httpOrg(ctx, tenantID)
}
