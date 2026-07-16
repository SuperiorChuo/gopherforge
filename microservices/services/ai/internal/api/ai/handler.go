// Package ai wires the AI service HTTP handlers: chat streaming,
// conversation management, knowledge base, and AI-generated reports.
package ai

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// StatusInfo is the provider metadata surfaced by GET /ai/status, resolved
// per request so console edits to the ai.provider setting show up live.
type StatusInfo struct {
	Configured bool
	Provider   string
	ChatModel  string
	EmbedModel string
}

// StatusSource resolves the current provider status.
type StatusSource func(ctx context.Context) StatusInfo

// Handler groups the AI endpoints and their dependencies.
type Handler struct {
	status    StatusSource
	chat      aisvc.ChatService
	knowledge aisvc.KnowledgeService
	insight   aisvc.InsightService
}

// NewHandler builds the AI endpoint handler.
func NewHandler(status StatusSource, chat aisvc.ChatService, knowledge aisvc.KnowledgeService, insight aisvc.InsightService) *Handler {
	return &Handler{
		status:    status,
		chat:      chat,
		knowledge: knowledge,
		insight:   insight,
	}
}

// GetStatus reports provider configuration and knowledge-base size.
func (h *Handler) GetStatus(c *gin.Context) {
	status := h.status(c.Request.Context())
	var documents int64
	if status.Configured {
		if count, err := h.knowledge.CountDocuments(c.Request.Context()); err == nil {
			documents = count
		}
	}
	response.Success(c, gin.H{
		"configured":   status.Configured,
		"provider":     status.Provider,
		"chat_model":   status.ChatModel,
		"embed_model":  status.EmbedModel,
		"kb_documents": documents,
	})
}

// requireConfigured writes 503 AI_NOT_CONFIGURED and returns false when no
// provider credentials are present. Health checks are unaffected: only the
// AI endpoints call this guard.
func (h *Handler) requireConfigured(c *gin.Context) bool {
	if h.status(c.Request.Context()).Configured {
		return true
	}
	response.ErrorWithCode(c, http.StatusServiceUnavailable, response.ErrorCodeAINotConfigured,
		"AI provider is not configured; set an API key in system settings or AI_API_KEY")
	return false
}

// currentUserID reads the authenticated user from the gin context.
func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok
}

// requireUserID resolves the authenticated user or writes a 401.
func requireUserID(c *gin.Context) (uint, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCodeAuthContextMissing, "user not found in context")
	}
	return userID, ok
}
