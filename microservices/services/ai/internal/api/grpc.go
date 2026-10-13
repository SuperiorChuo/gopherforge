package api

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/go-admin-kit/services/ai/internal/middleware"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	aiv1 "github.com/go-admin-kit/services/api/gen/ai/v1"
)

// AiGRPC 实现 aiv1.AiServiceServer（Phase 3：im 机器人 RAG 向量检索 gRPC 版）。
// 复用 KnowledgeService.Search，与 HTTP InternalSearchDocuments 同逻辑。
type AiGRPC struct {
	aiv1.UnimplementedAiServiceServer
	knowledge  *aisvc.KnowledgeService
	configured func(ctx context.Context) bool
}

// NewAiGRPC 创建 gRPC 服务实例。configured 判 AI provider 是否配置（nil 则跳过门禁）。
func NewAiGRPC(knowledge aisvc.KnowledgeService, configured func(ctx context.Context) bool) *AiGRPC {
	return &AiGRPC{knowledge: &knowledge, configured: configured}
}

// InternalKBSearch 向量检索知识库，返回 topK 匹配（余弦相似度降序）。
func (g *AiGRPC) InternalKBSearch(ctx context.Context, req *aiv1.InternalKBSearchRequest) (*aiv1.InternalKBSearchResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil InternalKBSearchRequest")
	}
	if g.configured != nil && !g.configured(ctx) {
		// 与 HTTP 端点同口径：AI provider 未配置返回不可用，消费方走本地回退。
		return nil, status.Error(codes.Unavailable, "AI provider is not configured")
	}
	tenantID := req.TenantId
	if tenantID == 0 {
		tenantID = 1
	}
	searchCtx := context.WithValue(ctx, middleware.TenantIDContextKey, uint(tenantID))
	matches, err := g.knowledge.Search(searchCtx, req.Query, int(req.TopK))
	if err != nil {
		return nil, err
	}
	list := make([]*aiv1.KBSearchMatch, 0, len(matches))
	for _, m := range matches {
		list = append(list, &aiv1.KBSearchMatch{
			DocumentId: uint64(m.DocumentID),
			Title:      m.Title,
			ChunkIndex: int32(m.ChunkIndex),
			Content:    m.Content,
			Score:      m.Score,
		})
	}
	return &aiv1.InternalKBSearchResponse{List: list}, nil
}
