package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/pkg/response"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
)

// chatRequest is the POST /ai/chat body.
type chatRequest struct {
	ConversationID   uint   `json:"conversation_id"`
	Message          string `json:"message" binding:"required"`
	UseKnowledgeBase bool   `json:"use_knowledge_base"`
}

// Chat streams one chat turn as Server-Sent Events. The stream carries
// delta events while the model generates, a final done event with the
// persisted identifiers, and an error event if the turn fails mid-stream.
func (h *Handler) Chat(c *gin.Context) {
	if !h.requireConfigured(c) {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "message is required")
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalServerError(c, "streaming unsupported by response writer")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	writeEvent := func(payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("data: " + string(data) + "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	result, err := h.chat.StreamChat(c.Request.Context(), aisvc.ChatRequest{
		ConversationID:   req.ConversationID,
		UserID:           userID,
		Message:          req.Message,
		UseKnowledgeBase: req.UseKnowledgeBase,
	}, func(delta aiclient.ChatDelta) error {
		if delta.Done || delta.Content == "" {
			return nil
		}
		return writeEvent(gin.H{"type": "delta", "content": delta.Content})
	})
	if err != nil {
		message := "chat completion failed"
		if errors.Is(err, aisvc.ErrConversationNotFound) {
			message = aisvc.ErrConversationNotFound.Error()
		}
		_ = writeEvent(gin.H{"type": "error", "message": message})
		return
	}

	_ = writeEvent(gin.H{
		"type":            "done",
		"conversation_id": result.ConversationID,
		"message_id":      result.MessageID,
	})
}

// ListConversations returns one page of the current user's conversations.
func (h *Handler) ListConversations(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	req := paginationFromQuery(c)
	conversations, total, err := h.chat.ListConversations(c.Request.Context(), userID, req)
	if err != nil {
		writeAIServiceError(c, "list conversations failed", err)
		return
	}
	response.PageSuccess(c, conversations, total, req.Page, req.PageSize)
}

// ListConversationMessages returns all messages of one conversation owned
// by the current user.
func (h *Handler) ListConversationMessages(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseIDParam(c)
	if !ok {
		return
	}
	messages, err := h.chat.ListMessages(c.Request.Context(), conversationID, userID)
	if err != nil {
		writeAIServiceError(c, "list conversation messages failed", err)
		return
	}
	response.Success(c, messages)
}

// DeleteConversation removes one conversation owned by the current user.
func (h *Handler) DeleteConversation(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.chat.DeleteConversation(c.Request.Context(), conversationID, userID); err != nil {
		writeAIServiceError(c, "delete conversation failed", err)
		return
	}
	response.Success(c, nil)
}

// parseIDParam reads the :id path parameter or writes a 400.
func parseIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return uint(id), true
}
