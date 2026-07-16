// Package ai wires the AI service HTTP handlers: chat streaming,
// conversation management, knowledge base, and AI-generated reports.
package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"

	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// Handler groups the AI endpoints and their dependencies.
type Handler struct {
	configured bool
	provider   string
	chatModel  string
	embedModel string
	chat       aisvc.ChatService
	knowledge  aisvc.KnowledgeService
	insight    aisvc.InsightService
}

// Config carries the provider metadata surfaced by GET /ai/status.
type Config struct {
	Configured bool
	Provider   string
	ChatModel  string
	EmbedModel string
}

// NewHandler builds the AI endpoint handler.
func NewHandler(cfg Config, chat aisvc.ChatService, knowledge aisvc.KnowledgeService, insight aisvc.InsightService) *Handler {
	return &Handler{
		configured: cfg.Configured,
		provider:   cfg.Provider,
		chatModel:  cfg.ChatModel,
		embedModel: cfg.EmbedModel,
		chat:       chat,
		knowledge:  knowledge,
		insight:    insight,
	}
}

// GetStatus reports provider configuration and knowledge-base size.
func (h *Handler) GetStatus(c *gin.Context) {
	var documents int64
	if h.configured {
		if count, err := h.knowledge.CountDocuments(c.Request.Context()); err == nil {
			documents = count
		}
	}
	response.Success(c, gin.H{
		"configured":   h.configured,
		"provider":     h.provider,
		"chat_model":   h.chatModel,
		"embed_model":  h.embedModel,
		"kb_documents": documents,
	})
}

// requireConfigured writes 503 AI_NOT_CONFIGURED and returns false when no
// provider credentials are present. Health checks are unaffected: only the
// AI endpoints call this guard.
func (h *Handler) requireConfigured(c *gin.Context) bool {
	if h.configured {
		return true
	}
	response.ErrorWithCode(c, http.StatusServiceUnavailable, response.ErrorCodeAINotConfigured,
		"AI provider is not configured; set AI_API_KEY")
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
