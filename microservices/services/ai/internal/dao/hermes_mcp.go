package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes M3：MCP 管理的 DAO。挂在现有 *HermesDAO 上（同类型分文件）。

// ErrHermesMcpDuplicate 对应 uk_hermes_mcp_servers_tenant_name 唯一索引。
var ErrHermesMcpDuplicate = errors.New("hermes mcp server already exists")

// ErrHermesClusterMcpDuplicate 对应 uk_hermes_cluster_mcp_servers_cluster_server。
var ErrHermesClusterMcpDuplicate = errors.New("cluster mcp binding already exists")

func (d *HermesDAO) CreateMcpServer(ctx context.Context, server *localmodel.HermesMcpServer) error {
	err := d.db.WithContext(ctx).Create(server).Error
	if err != nil && isMcpUniqueViolation(err) {
		return ErrHermesMcpDuplicate
	}
	return err
}

func isMcpUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uk_hermes_mcp_servers_tenant_name") ||
		strings.Contains(msg, "uk_hermes_cluster_mcp_servers_cluster_server") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "unique constraint failed")
}

// ListMcpServers 分页列出 MCP server（secret_json 不进列表查询）。
func (d *HermesDAO) ListMcpServers(ctx context.Context, page, pageSize int) ([]localmodel.HermesMcpServer, int64, error) {
	servers := make([]localmodel.HermesMcpServer, 0)
	var total int64
	query := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Omit("secret_json").Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&servers).Error
	return servers, total, err
}

// GetMcpServer 取单个（secret_json 不进返回，供 console 展示与绑定校验）。
func (d *HermesDAO) GetMcpServer(ctx context.Context, id uint64) (*localmodel.HermesMcpServer, error) {
	var server localmodel.HermesMcpServer
	err := d.db.WithContext(ctx).Omit("secret_json").First(&server, id).Error
	return &server, err
}

// SetMcpServerStatus 带来源状态守卫的状态流转。
func (d *HermesDAO) SetMcpServerStatus(ctx context.Context, id uint64, from, to string, actor uint, now time.Time) error {
	fields := map[string]any{"status": to, "updated_at": now}
	if to == localmodel.HermesMcpApproved {
		fields["approved_by"] = actor
		fields["approved_at"] = now
	}
	result := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{}).
		Where("id = ? AND status = ?", id, from).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("set Hermes mcp status %s->%s: %w", from, to, gorm.ErrRecordNotFound)
	}
	return nil
}

// UpdateMcpServer 更新功能字段并重置状态为 pending（需重审批）。secret 变更时
// 由 service 层 bump secret_revision。
func (d *HermesDAO) UpdateMcpServer(ctx context.Context, id uint64, fields map[string]any) error {
	result := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{}).
		Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("update Hermes mcp server: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

// GetMcpSecrets 取 server 的明文密钥（设备签名端点用，仅已审批）。
func (d *HermesDAO) GetMcpSecrets(ctx context.Context, serverID uint64) (map[string]string, error) {
	var server localmodel.HermesMcpServer
	err := d.db.WithContext(ctx).Select("secret_json").
		Where("id = ? AND status = ?", serverID, localmodel.HermesMcpApproved).
		First(&server).Error
	if err != nil {
		return nil, err
	}
	return server.SecretJSON, nil
}

func (d *HermesDAO) BindMcpServerToCluster(ctx context.Context, clusterID, serverID uint64, actor uint, now time.Time) error {
	binding := &localmodel.HermesClusterMcpServer{
		TenantID:  tenant.FromContextOrDefault(ctx),
		ClusterID: clusterID,
		ServerID:  serverID,
		CreatedBy: actor,
		CreatedAt: now,
	}
	err := d.db.WithContext(ctx).Create(binding).Error
	if err != nil && isMcpUniqueViolation(err) {
		return ErrHermesClusterMcpDuplicate
	}
	return err
}

func (d *HermesDAO) UnbindMcpServerFromCluster(ctx context.Context, clusterID, serverID uint64) error {
	return d.db.WithContext(ctx).
		Where("cluster_id = ? AND server_id = ?", clusterID, serverID).
		Delete(&localmodel.HermesClusterMcpServer{}).Error
}

// ListClusterMcpServers 列出集群绑定的 MCP server（含状态，secret_json 不进列表）。
func (d *HermesDAO) ListClusterMcpServers(ctx context.Context, clusterID uint64) ([]localmodel.HermesMcpServer, error) {
	servers := make([]localmodel.HermesMcpServer, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{}).
		Select(`hermes_mcp_servers.id, hermes_mcp_servers.tenant_id, hermes_mcp_servers.name,
			hermes_mcp_servers.display_name, hermes_mcp_servers.type, hermes_mcp_servers.command,
			hermes_mcp_servers.url, hermes_mcp_servers.status, hermes_mcp_servers.secret_revision,
			hermes_mcp_servers.created_at, hermes_mcp_servers.updated_at`).
		Joins("JOIN hermes_cluster_mcp_servers cs ON cs.server_id = hermes_mcp_servers.id").
		Where("cs.cluster_id = ?", clusterID).
		Order("hermes_mcp_servers.name ASC").Scan(&servers).Error
	return servers, err
}

// ListApprovedMcpServersForCluster 返回集群绑定的已审批 MCP server（buildDesiredState 用）。
func (d *HermesDAO) ListApprovedMcpServersForCluster(ctx context.Context, clusterID uint64) ([]localmodel.HermesMcpServer, error) {
	servers := make([]localmodel.HermesMcpServer, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{}).
		Select(`hermes_mcp_servers.id, hermes_mcp_servers.tenant_id, hermes_mcp_servers.name,
			hermes_mcp_servers.type, hermes_mcp_servers.command, hermes_mcp_servers.args,
			hermes_mcp_servers.url, hermes_mcp_servers.env, hermes_mcp_servers.headers,
			hermes_mcp_servers.tools_include, hermes_mcp_servers.tools_exclude,
			hermes_mcp_servers.timeout, hermes_mcp_servers.secret_revision`).
		Joins("JOIN hermes_cluster_mcp_servers cs ON cs.server_id = hermes_mcp_servers.id").
		Where("cs.cluster_id = ? AND hermes_mcp_servers.status = ?", clusterID, localmodel.HermesMcpApproved).
		Order("hermes_mcp_servers.name ASC").Scan(&servers).Error
	return servers, err
}

// ListClusterServerIDs 返回集群绑定的已审批 server id（IssueMcpSecrets 用）。
func (d *HermesDAO) ListClusterMcpServerIDs(ctx context.Context, clusterID uint64) ([]uint64, error) {
	ids := make([]uint64, 0)
	err := d.db.WithContext(ctx).Model(&localmodel.HermesMcpServer{}).
		Select("hermes_mcp_servers.id").
		Joins("JOIN hermes_cluster_mcp_servers cs ON cs.server_id = hermes_mcp_servers.id").
		Where("cs.cluster_id = ? AND hermes_mcp_servers.status = ?", clusterID, localmodel.HermesMcpApproved).
		Scan(&ids).Error
	return ids, err
}
