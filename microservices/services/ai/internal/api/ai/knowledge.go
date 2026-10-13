package ai

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/middleware"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/pagination"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// documentRequest is the POST /ai/kb/documents body.
type documentRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// CreateDocument ingests a knowledge-base document: chunk, embed, persist.
func (h *Handler) CreateDocument(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var req documentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "title and content are required")
		return
	}

	result, err := h.knowledge.CreateDocument(c.Request.Context(), req.Title, req.Content, userID)
	if err != nil {
		writeAIServiceError(c, "create document failed", err)
		return
	}
	response.Success(c, result)
}

// ListDocuments returns one page of knowledge-base documents.
func (h *Handler) ListDocuments(c *gin.Context) {
	req := paginationFromQuery(c)
	documents, total, err := h.knowledge.ListDocuments(c.Request.Context(), req)
	if err != nil {
		writeAIServiceError(c, "list documents failed", err)
		return
	}
	response.PageSuccess(c, documents, total, req.Page, req.PageSize)
}

// DeleteDocument removes a knowledge-base document and its chunks.
func (h *Handler) DeleteDocument(c *gin.Context) {
	documentID, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.knowledge.DeleteDocument(c.Request.Context(), documentID); err != nil {
		writeAIServiceError(c, "delete document failed", err)
		return
	}
	response.Success(c, nil)
}

// UpdateDocument replaces a knowledge-base document and re-ingests its chunks.
func (h *Handler) UpdateDocument(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}
	documentID, ok := parseIDParam(c)
	if !ok {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var req documentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "title and content are required")
		return
	}

	result, err := h.knowledge.UpdateDocument(c.Request.Context(), documentID, req.Title, req.Content, userID)
	if err != nil {
		writeAIServiceError(c, "update document failed", err)
		return
	}
	response.Success(c, result)
}

// GetDocument returns one document including its full content.
func (h *Handler) GetDocument(c *gin.Context) {
	documentID, ok := parseIDParam(c)
	if !ok {
		return
	}
	document, err := h.knowledge.GetDocument(c.Request.Context(), documentID)
	if err != nil {
		writeAIServiceError(c, "get document failed", err)
		return
	}
	response.Success(c, document)
}

// ListDocumentChunks returns the chunks of one document in index order.
func (h *Handler) ListDocumentChunks(c *gin.Context) {
	documentID, ok := parseIDParam(c)
	if !ok {
		return
	}
	chunks, err := h.knowledge.ListDocumentChunks(c.Request.Context(), documentID)
	if err != nil {
		writeAIServiceError(c, "list document chunks failed", err)
		return
	}
	response.Success(c, chunks)
}

// searchRequest is the POST /ai/kb/search body.
type searchRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k"`
}

// SearchDocuments runs a cosine similarity search over embedded chunks.
func (h *Handler) SearchDocuments(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}

	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "query is required")
		return
	}

	matches, err := h.knowledge.Search(c.Request.Context(), req.Query, req.TopK)
	if err != nil {
		writeAIServiceError(c, "knowledge base search failed", err)
		return
	}
	response.Success(c, matches)
}

// internalSearchRequest is the body for service-to-service KB search (IM bot RAG).
type internalSearchRequest struct {
	Query    string `json:"query" binding:"required"`
	TopK     int    `json:"top_k"`
	TenantID uint   `json:"tenant_id"`
}

// InternalSearchDocuments is called by im-service over the docker network.
// Auth: X-Internal-Token must match AI_INTERNAL_TOKEN (empty token disables endpoint).
func (h *Handler) InternalSearchDocuments(c *gin.Context) {
	expected := strings.TrimSpace(os.Getenv("AI_INTERNAL_TOKEN"))
	if expected == "" {
		response.Error(c, http.StatusServiceUnavailable, "internal kb search not configured")
		return
	}
	if c.GetHeader("X-Internal-Token") != expected {
		response.Unauthorized(c, "invalid internal token")
		return
	}
	if !h.requireConfigured(c) {
		return
	}
	var req internalSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "query is required")
		return
	}
	tenantID := req.TenantID
	if tenantID == 0 {
		if h := c.GetHeader("X-Auth-Tenant-ID"); h != "" {
			if n, err := strconv.ParseUint(h, 10, 64); err == nil && n > 0 {
				tenantID = uint(n)
			}
		}
	}
	if tenantID == 0 {
		tenantID = 1
	}
	ctx := context.WithValue(c.Request.Context(), middleware.TenantIDContextKey, tenantID)
	matches, err := h.knowledge.Search(ctx, req.Query, req.TopK)
	if err != nil {
		writeAIServiceError(c, "knowledge base search failed", err)
		return
	}
	response.Success(c, gin.H{"list": matches, "source": "vector"})
}

// insightRequest is the POST /ai/logs/insight body.
type insightRequest struct {
	Days int `json:"days"`
}

// GenerateLogInsight produces an AI-written security report over recent
// login and operation logs.
func (h *Handler) GenerateLogInsight(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}

	// The body is optional; an empty or malformed body falls back to the
	// default window handled by the service layer.
	var req insightRequest
	_ = c.ShouldBindJSON(&req)

	report, err := h.insight.GenerateLogInsight(c.Request.Context(), req.Days)
	if err != nil {
		writeAIServiceError(c, "log insight generation failed", err)
		return
	}
	response.Success(c, gin.H{"report": report})
}

// composeRequest is the POST /ai/compose body.
type composeRequest struct {
	Kind     string `json:"kind" binding:"required"`
	Prompt   string `json:"prompt" binding:"required"`
	Draft    string `json:"draft"`
	Platform string `json:"platform"` // social_post kind：目标平台风格
}

// Compose drafts operator-facing content such as notices.
func (h *Handler) Compose(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}

	var req composeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "kind and prompt are required")
		return
	}

	content, err := h.insight.Compose(c.Request.Context(), aisvc.ComposeRequest{
		Kind:     req.Kind,
		Prompt:   req.Prompt,
		Draft:    req.Draft,
		Platform: req.Platform,
	})
	if err != nil {
		writeAIServiceError(c, "compose failed", err)
		return
	}
	response.Success(c, gin.H{"content": content})
}

// imageRequest is the POST /ai/image body. size is optional and falls back
// to the provider default (1024x1024) when omitted.
type imageRequest struct {
	Prompt string `json:"prompt" binding:"required"`
	Size   string `json:"size"`
}

// Image generates AI images from a text prompt.
func (h *Handler) Image(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}

	var req imageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "prompt is required")
		return
	}

	result, err := h.insight.GenerateImage(c.Request.Context(), req.Prompt, aiclient.ImageOptions{
		Size: req.Size,
	})
	if err != nil {
		writeAIServiceError(c, "image generation failed", err)
		return
	}
	response.Success(c, gin.H{"urls": result.URLs, "b64": result.B64})
}

// paginationFromQuery reads page/page_size query parameters.
func paginationFromQuery(c *gin.Context) pagination.PageRequest {
	return pagination.GetPageRequest(c)
}

// writeAIServiceError maps service errors onto the shared response envelope.
func writeAIServiceError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, aisvc.ErrConversationNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeAIConversationNotFound, aisvc.ErrConversationNotFound.Error())
	case errors.Is(err, aisvc.ErrDocumentNotFound):
		response.NotFoundWithCode(c, response.ErrorCodeAIDocumentNotFound, aisvc.ErrDocumentNotFound.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		internalServerError(c, operation, err)
	default:
		internalServerError(c, operation, err)
	}
}

func internalServerError(c *gin.Context, message string, err error) {
	if logger.Logger != nil && err != nil {
		logger.Error(message, logger.Err(err))
	}
	response.InternalServerErrorWithCode(c, response.ErrorCodeAIProviderError, message)
}
