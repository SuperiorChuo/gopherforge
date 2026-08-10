package api

import (
	"context"
	"encoding/json"

	bpmv1 "github.com/go-admin-kit/services/api/gen/bpm/v1"
	"github.com/go-admin-kit/services/bpm/internal/engine"
)

// BpmGRPC 实现 bpmv1.BpmServiceServer（Phase 3：发起审批 gRPC 版）。
// 复用 Server.InternalCreateInstance 逻辑（Engine.Start + applyEffects）。
type BpmGRPC struct {
	bpmv1.UnimplementedBpmServiceServer
	srv *Server
}

// NewBpmGRPC 创建 gRPC 服务实例。
func NewBpmGRPC(srv *Server) *BpmGRPC { return &BpmGRPC{srv: srv} }

// StartInstance 发起流程实例。
func (g *BpmGRPC) StartInstance(ctx context.Context, req *bpmv1.StartInstanceRequest) (*bpmv1.StartInstanceResponse, error) {
	if req.InitiatorId == 0 {
		return nil, errInitiatorRequired
	}
	eff, err := g.srv.Engine.Start(engine.StartInput{
		TenantID:      req.TenantId,
		DefinitionKey: req.DefinitionKey,
		Title:         req.Title,
		BizType:       req.BizType,
		BizID:         req.BizId,
		FormSnapshot:  req.FormSnapshot,
		Variables:     req.Variables,
		InitiatorID:   req.InitiatorId,
		InitiatorDept: req.InitiatorDept,
	})
	if err != nil {
		return nil, err
	}
	g.srv.applyEffects(eff)
	return &bpmv1.StartInstanceResponse{InstanceId: eff.Instance.ID, Status: string(eff.Instance.Status)}, nil
}

// InstancesByBiz 按 (biz_type,biz_id) 反查实例列表，原样 JSON 透传（对齐 HTTP 端点）。
func (g *BpmGRPC) InstancesByBiz(ctx context.Context, req *bpmv1.InstancesByBizRequest) (*bpmv1.InstancesByBizResponse, error) {
	if req.BizType == "" || req.BizId == "" {
		return nil, errBizRequired
	}
	list, err := g.srv.Store.FindByBiz(req.TenantId, req.BizType, req.BizId)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(map[string]any{"list": list})
	if err != nil {
		return nil, err
	}
	return &bpmv1.InstancesByBizResponse{Json: raw}, nil
}

var errInitiatorRequired = &initiatorRequiredError{}

type initiatorRequiredError struct{}

func (*initiatorRequiredError) Error() string { return "initiator_id 必填" }

var errBizRequired = &bizRequiredError{}

type bizRequiredError struct{}

func (*bizRequiredError) Error() string { return "biz_type / biz_id 必填" }
