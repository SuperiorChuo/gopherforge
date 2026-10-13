package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/service/hermes"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// HermesProxyHandler 暴露 OpenAI 兼容的入站接口给受管 Hermes。
//
// 这里不能用项目统一的 response 包装：Hermes 是原生上游程序，按 OpenAI 规范
// 解析响应体，多包一层 {code,data} 它就读不懂了。因此成功路径直接写 OpenAI
// 形状的 JSON / SSE，错误路径写 OpenAI 的 {"error":{...}} 形状。
type HermesProxyHandler struct {
	proxy *service.HermesProxyService
}

func NewHermesProxyHandler(proxy *service.HermesProxyService) *HermesProxyHandler {
	return &HermesProxyHandler{proxy: proxy}
}

// IssueToken 由 manager 用设备签名身份调用，换取给 Hermes 用的短期 bearer token。
// 这个端点走设备签名中间件，因此用项目统一响应格式。
func (h *HermesProxyHandler) IssueToken(c *gin.Context) {
	device, ok := hermesDeviceFromContext(c)
	if !ok {
		response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_DEVICE_CONTEXT_MISSING"), "device context missing")
		return
	}
	token, err := h.proxy.IssueModelToken(c.Request.Context(), device)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHermesProxyForbidden):
			response.ForbiddenWithCode(c, response.ErrorCode("HERMES_PROXY_FORBIDDEN"), err.Error())
		case errors.Is(err, service.ErrHermesProxyUnauthorized):
			response.UnauthorizedWithCode(c, response.ErrorCode("HERMES_PROXY_UNAUTHORIZED"), "unauthorized")
		default:
			response.InternalServerError(c, "issue Hermes model token failed")
		}
		return
	}
	response.Success(c, token)
}

// openAIError 是 OpenAI 规范的错误形状。
func openAIError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"message": message, "type": "invalid_request_error", "code": code},
	})
}

// writeProxyStreamError 把转发阶段的错误映射成 OpenAI 客户端能理解的状态码。
// 配额与授权类错误要区分于上游故障——前者重试无用，后者可能是临时的。
func writeProxyStreamError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrHermesQuotaExceeded):
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{"message": err.Error(), "type": "insufficient_quota", "code": "quota_exceeded"},
		})
	case errors.Is(err, service.ErrHermesProxyForbidden):
		openAIError(c, http.StatusForbidden, "hermes_forbidden", err.Error())
	case errors.Is(err, service.ErrHermesInvalidInput):
		openAIError(c, http.StatusBadRequest, "invalid_body", err.Error())
	default:
		openAIError(c, http.StatusBadGateway, "upstream_error", "model backend request failed")
	}
}

// ChatCompletions 是 POST /v1/chat/completions 的入口。
func (h *HermesProxyHandler) ChatCompletions(c *gin.Context) {
	caller, ctx, err := h.proxy.Authorize(c.Request.Context(), c.GetHeader("Authorization"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHermesProxyForbidden):
			openAIError(c, http.StatusForbidden, "hermes_forbidden", err.Error())
		default:
			openAIError(c, http.StatusUnauthorized, "hermes_unauthorized", "invalid or expired model token")
		}
		return
	}

	var req service.HermesProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		openAIError(c, http.StatusBadRequest, "invalid_body", "invalid chat completion payload")
		return
	}
	if len(req.Messages) == 0 {
		openAIError(c, http.StatusBadRequest, "invalid_body", "messages is required")
		return
	}

	if req.Stream {
		h.streamCompletion(c, ctx, caller, req)
		return
	}
	h.blockingCompletion(c, ctx, caller, req)
}

func completionID() string {
	return "chatcmpl-" + strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()), " ", "")
}

func (h *HermesProxyHandler) blockingCompletion(
	c *gin.Context,
	ctx context.Context,
	caller *service.HermesProxyCaller,
	req service.HermesProxyRequest,
) {
	var builder strings.Builder
	var usage *aiclient.Usage
	err := h.proxy.StreamChat(ctx, caller, req, func(delta aiclient.ChatDelta) error {
		builder.WriteString(delta.Content)
		if delta.Done && delta.Usage != nil {
			usage = delta.Usage
		}
		return nil
	})
	if err != nil {
		writeProxyStreamError(c, err)
		return
	}

	payload := gin.H{
		"id":      completionID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []gin.H{{
			"index":         0,
			"message":       gin.H{"role": "assistant", "content": builder.String()},
			"finish_reason": "stop",
		}},
	}
	if usage != nil {
		payload["usage"] = gin.H{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}
	c.JSON(http.StatusOK, payload)
}

func (h *HermesProxyHandler) streamCompletion(
	c *gin.Context,
	ctx context.Context,
	caller *service.HermesProxyCaller,
	req service.HermesProxyRequest,
) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		openAIError(c, http.StatusInternalServerError, "stream_unsupported", "streaming is not supported")
		return
	}

	// 配额/授权要在写 SSE 头之前判掉：头一旦发出就只能返回 200，客户端拿不到
	// 429/403 状态码。这里先做一次预检，让这类错误走正常的 HTTP 错误响应。
	if err := h.proxy.Precheck(ctx, caller); err != nil {
		writeProxyStreamError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := completionID()
	created := time.Now().Unix()

	writeChunk := func(delta gin.H, finish any) error {
		chunk := gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   req.Model,
			"choices": []gin.H{{"index": 0, "delta": delta, "finish_reason": finish}},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("data: " + string(data) + "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := writeChunk(gin.H{"role": "assistant"}, nil); err != nil {
		return
	}

	streamErr := h.proxy.StreamChat(ctx, caller, req, func(delta aiclient.ChatDelta) error {
		if delta.Content == "" {
			return nil
		}
		return writeChunk(gin.H{"content": delta.Content}, nil)
	})
	if streamErr != nil {
		// 头已发出，无法再改状态码；按 OpenAI 客户端能识别的方式在流里发错误
		data, _ := json.Marshal(gin.H{
			"error": gin.H{"message": "model backend request failed", "type": "upstream_error"},
		})
		_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
		flusher.Flush()
		return
	}

	_ = writeChunk(gin.H{}, "stop")
	_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	flusher.Flush()
}
