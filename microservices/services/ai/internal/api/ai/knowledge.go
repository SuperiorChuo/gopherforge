package ai

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/go-admin-kit/services/ai/internal/pkg/logger"
	"github.com/go-admin-kit/services/ai/internal/pkg/pagination"
	"github.com/go-admin-kit/services/ai/internal/pkg/response"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
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
	Kind   string `json:"kind" binding:"required"`
	Prompt string `json:"prompt" binding:"required"`
	Draft  string `json:"draft"`
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
		Kind:   req.Kind,
		Prompt: req.Prompt,
		Draft:  req.Draft,
	})
	if err != nil {
		writeAIServiceError(c, "compose failed", err)
		return
	}
	response.Success(c, gin.H{"content": content})
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
