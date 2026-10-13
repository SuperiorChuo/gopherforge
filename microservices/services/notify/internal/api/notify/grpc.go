package api

import (
	"context"

	notifyv1 "github.com/go-admin-kit/services/api/gen/notify/v1"
	"github.com/go-admin-kit/services/notify/internal/store/notify"
)

// NotifyGRPC 实现 notifyv1.NotifyServiceServer（Phase 3：发信 gRPC 版）。
// 复用 Server 的 Store.SendBatch / processAlerts，与 HTTP 端点同逻辑。
type NotifyGRPC struct {
	notifyv1.UnimplementedNotifyServiceServer
	srv *Server
}

// NewNotifyGRPC 创建 gRPC 服务实例。
func NewNotifyGRPC(srv *Server) *NotifyGRPC { return &NotifyGRPC{srv: srv} }

// InternalSend 发站内信（批量 fan-out）。
func (g *NotifyGRPC) InternalSend(ctx context.Context, req *notifyv1.InternalSendRequest) (*notifyv1.InternalSendResponse, error) {
	users := req.UserIds
	if req.UserId > 0 {
		users = append(users, req.UserId)
	}
	if len(users) == 0 {
		return nil, errNoRecipients
	}
	tenantID := req.TenantId
	if tenantID == 0 {
		tenantID = 1
	}
	res, err := g.srv.Store.SendBatch(ctx, store.BatchSendInput{
		TenantID:     tenantID,
		UserIDs:      users,
		TemplateCode: req.TemplateCode,
		Type:         req.Type,
		RefType:      req.RefType,
		RefID:        req.RefId,
		Vars:         req.Vars,
		Title:        req.Title,
		Content:      req.Content,
		Link:         req.Link,
	})
	if err != nil {
		return nil, err
	}
	return &notifyv1.InternalSendResponse{
		Created:  int64(res.Created),
		Rejected: int64(res.Rejected),
		Skipped:  int64(res.Skipped),
	}, nil
}

// InternalAlert 接收告警（monitor StationNotifier 等），Alertmanager 形状。
// 复用 HTTP ReceiveAlerts 的 processAlerts 逻辑。
func (g *NotifyGRPC) InternalAlert(ctx context.Context, req *notifyv1.InternalAlertRequest) (*notifyv1.InternalAlertResponse, error) {
	if req == nil {
		return nil, errNilAlertReq
	}
	webhook := amWebhook{Status: req.Status}
	for _, a := range req.Alerts {
		if a == nil {
			continue
		}
		webhook.Alerts = append(webhook.Alerts, amAlert{
			Status:      a.Status,
			Labels:      a.Labels,
			Annotations: a.Annotations,
			StartsAt:    a.StartsAt,
			Fingerprint: a.Fingerprint,
		})
	}
	created, skipped, err := g.srv.processAlerts(ctx, webhook, g.srv.AlertUserID)
	if err != nil {
		return nil, err
	}
	return &notifyv1.InternalAlertResponse{Created: int64(created), Skipped: int64(skipped)}, nil
}

var errNoRecipients = &noRecipientsError{}

type noRecipientsError struct{}

func (*noRecipientsError) Error() string { return "user_id or user_ids required" }

var errNilAlertReq = &nilAlertReqError{}

type nilAlertReqError struct{}

func (*nilAlertReqError) Error() string { return "nil InternalAlertRequest" }
