package api

import (
	"context"

	crmv1 "github.com/go-admin-kit/services/api/gen/crm/v1"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/crm/internal/store/crm"
)

// CrmGRPC 实现 crmv1.CrmServiceServer（Phase 3：号码识客/挂机跟进 gRPC 版）。
// 复用 Store 逻辑，与 HTTP internal 端点同。
type CrmGRPC struct {
	crmv1.UnimplementedCrmServiceServer
	Store *store.Store
}

// CreateFollowUp 挂机写跟进（支持 idempotency_key，防 gRPC→HTTP 回退双写）。
func (g *CrmGRPC) CreateFollowUp(ctx context.Context, req *crmv1.CreateFollowUpRequest) (*crmv1.CreateFollowUpResponse, error) {
	if req.TargetId == 0 {
		return nil, errTargetRequired
	}
	f := &model.FollowUp{
		TenantID:   req.TenantId,
		TargetType: req.TargetType,
		TargetID:   req.TargetId,
		Content:    req.Content,
		ByUserID:   req.ByUserId,
	}
	if err := g.Store.CreateFollowUpIdempotent(ctx, f, req.GetIdempotencyKey()); err != nil {
		return nil, err
	}
	return &crmv1.CreateFollowUpResponse{Ok: true, FollowUpId: f.ID}, nil
}

var errTargetRequired = &targetRequiredError{}

type targetRequiredError struct{}

func (*targetRequiredError) Error() string { return "target_id 必填" }

// NewCrmGRPC 创建 gRPC 服务实例。
func NewCrmGRPC(st *store.Store) *CrmGRPC { return &CrmGRPC{Store: st} }

// LookupByPhone 号码识客。
func (g *CrmGRPC) LookupByPhone(ctx context.Context, req *crmv1.LookupByPhoneRequest) (*crmv1.LookupByPhoneResponse, error) {
	res, err := g.Store.LookupByPhone(req.TenantId, req.Phone)
	if err != nil {
		return nil, err
	}
	out := &crmv1.LookupByPhoneResponse{Matched: res.Matched}
	if res.Customer != nil {
		out.Customer = &crmv1.CrmCustomer{
			Id:          res.Customer.ID,
			Name:        res.Customer.Name,
			Phone:       res.Customer.Phone,
			OwnerUserId: res.Customer.OwnerUserID,
		}
	}
	if res.Lead != nil {
		out.Lead = &crmv1.CrmLead{
			Id:          res.Lead.ID,
			Name:        res.Lead.Name,
			Phone:       res.Lead.Phone,
			OwnerUserId: res.Lead.OwnerUserID,
		}
	}
	return out, nil
}
