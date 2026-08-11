package api

import (
	"context"

	"gorm.io/gorm"

	auditv1 "github.com/go-admin-kit/services/api/gen/audit/v1"
	"github.com/go-admin-kit/services/audit/internal/dao/system"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
)

// AuditGRPC 实现 auditv1.AuditServiceServer（Phase 1 服务发现试点端点）。
type AuditGRPC struct {
	auditv1.UnimplementedAuditServiceServer
	db *gorm.DB
}

// NewAuditGRPC 创建 gRPC 服务实例。
func NewAuditGRPC(db *gorm.DB) *AuditGRPC { return &AuditGRPC{db: db} }

// WriteLog 写一条审计日志（复用现有 CreateLogContext）。
func (g *AuditGRPC) WriteLog(ctx context.Context, req *auditv1.WriteLogRequest) (*auditv1.WriteLogResponse, error) {
	log := &localmodel.AuditLog{
		TenantID:   uint(req.TenantId),
		ActorType:  req.ActorType,
		ActorID:    req.ActorId,
		Action:     req.Action,
		TargetType: req.TargetType,
		TargetID:   req.TargetId,
		Summary:    req.Summary,
	}
	if err := system.NewAuditLogDAO(g.db).CreateLogContext(ctx, log); err != nil {
		return nil, err
	}
	return &auditv1.WriteLogResponse{Id: int64(log.ID)}, nil
}
