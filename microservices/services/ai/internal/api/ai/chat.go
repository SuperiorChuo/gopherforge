package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	aisvc "github.com/go-admin-kit/services/ai/internal/service/ai"
	coresvc "github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

const usageRecordTimeout = 5 * time.Second

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
		// 记录失败的调用
		h.recordUsage(c, userID, nil, err)
		_ = writeEvent(gin.H{"type": "error", "message": message})
		return
	}

	// 记录成功的调用
	h.recordUsage(c, userID, result.Usage, nil)

	_ = writeEvent(gin.H{
		"type":            "done",
		"conversation_id": result.ConversationID,
		"message_id":      result.MessageID,
	})
}

// recordUsage 记录AI调用到 ai_usage_logs
func (h *Handler) recordUsage(c *gin.Context, userID uint, usage *aiclient.Usage, err error) {
	if h.usage == nil {
		return // usage service 未注入，跳过记录
	}

	// Usage persistence must outlive a disconnected/cancelled streaming request,
	// while retaining tenant and auth values carried by its context.
	recordBaseCtx := context.WithoutCancel(c.Request.Context())
	status := h.status(recordBaseCtx)
	modelName := status.ChatModel
	if modelName == "" {
		modelName = "unknown"
	}

	tenantID := tenant.FromContextOrDefault(c.Request.Context())
	var tenantIDPtr *uint64
	if tenantID > 0 {
		tid := uint64(tenantID)
		tenantIDPtr = &tid
	}

	userIDU64 := uint64(userID)
	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	log := &localmodel.AIUsageLog{
		ModelName: modelName,
		UserID:    &userIDU64,
		Username:  usernameStr,
		TenantID:  tenantIDPtr,
		Operation: localmodel.AIOperationChat,
		Status:    localmodel.AIUsageStatusSuccess,
		CreatedAt: time.Now(),
	}

	if usage != nil {
		log.PromptTokens = &usage.PromptTokens
		log.CompletionTokens = &usage.CompletionTokens
		log.TotalTokens = &usage.TotalTokens
	}

	if err != nil {
		log.Status = localmodel.AIUsageStatusError
		log.ErrorMsg = err.Error()
	}

	// 异步记录，不影响主流程。模型查询只取 ID 与费率，不解密 API Key。
	go func(baseCtx context.Context) {
		recordCtx, cancel := context.WithTimeout(baseCtx, usageRecordTimeout)
		defer cancel()
		if h.models != nil {
			if pricing, lookupErr := h.models.GetUsagePricingByName(recordCtx, modelName); lookupErr == nil {
				log.ModelID = pricing.ID
				log.ModelName = pricing.Name
				if usage != nil {
					log.Cost = calculateUsageCost(usage, pricing)
				}
			}
		}
		_ = h.usage.RecordUsage(recordCtx, log)
	}(recordBaseCtx)
}

func calculateUsageCost(usage *aiclient.Usage, pricing *coresvc.AIModelUsagePricing) float64 {
	if pricing == nil {
		return 0
	}
	// 公式下沉到 AIModelUsagePricing.CostForUsage，与 Hermes proxy 共用同一实现
	return pricing.CostForUsage(usage)
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

// ListConversationMessages returns one keyset page of a conversation owned by
// the current user, newest window first. Query params: limit (default 50, max
// 200) and before_id (exclusive cursor; omit for the newest page). The
// response carries list / has_more / next_before_id so the client can walk
// backwards through a long conversation instead of loading every turn.
func (h *Handler) ListConversationMessages(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseIDParam(c)
	if !ok {
		return
	}
	limit, ok := parseUintQuery(c, "limit")
	if !ok {
		return
	}
	beforeID, ok := parseUintQuery(c, "before_id")
	if !ok {
		return
	}

	messages, hasMore, err := h.chat.ListMessages(c.Request.Context(), conversationID, userID, beforeID, int(limit))
	if err != nil {
		writeAIServiceError(c, "list conversation messages failed", err)
		return
	}

	// next_before_id is the oldest id of this window: feed it back as
	// before_id to fetch the page before it (0 when nothing older remains).
	var nextBeforeID uint
	if hasMore && len(messages) > 0 {
		nextBeforeID = messages[0].ID
	}
	response.Success(c, gin.H{
		"list":           messages,
		"has_more":       hasMore,
		"next_before_id": nextBeforeID,
	})
}

// parseUintQuery reads an optional non-negative integer query param
// (absent or empty → 0). It rejects malformed values instead of silently
// falling back, so a typo cannot quietly change the page being read.
func parseUintQuery(c *gin.Context, name string) (uint, bool) {
	raw := c.Query(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid "+name)
		return 0, false
	}
	return uint(value), true
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
