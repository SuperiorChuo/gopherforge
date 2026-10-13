package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/ai/internal/agentcore"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

const (
	agentUnavailableMessage = "Agent Kernel is not available"
	maxAgentDBID            = uint64(^uint64(0) >> 1)
	maxAgentBodyBytes       = 256 * 1024
)

func (h *Handler) requireAgentService(c *gin.Context) bool {
	if h.agents != nil {
		return true
	}
	response.Error(c, http.StatusServiceUnavailable, agentUnavailableMessage)
	return false
}

func parseAgentID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 || id > maxAgentDBID {
		response.BadRequestWithCode(c, response.ErrorCode("AGENT_INVALID_ID"), "invalid agent resource id")
		return 0, false
	}
	return id, true
}

func parseAgentPagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return page, pageSize
}

// SSE 事件流的轮询与保活间隔。提成包级变量只为让测试能在毫秒级覆盖
// "跨批续作"、"空闲保活"与"分档退避"——生产取值不变。测试串行运行，
// 覆盖前保存、用 t.Cleanup 还原。
var (
	// ssePollInterval：run 已被认领（running）时的基线。这是实时性契约，不要再放大。
	ssePollInterval = 500 * time.Millisecond
	// sseQueuedPollInterval：run 还没有被认领（只有 run.queued）时的基线。
	// queued→running 的转变不需要亚秒级感知，用更长的间隔换更低的空转负载。
	sseQueuedPollInterval = 2 * time.Second
	// ssePollMaxInterval：连续空轮次退避的封顶。
	ssePollMaxInterval   = 5 * time.Second
	sseKeepAliveInterval = 15 * time.Second
)

// sseNextPollDelay 决定下一轮轮询的间隔。
//
//   - 未被认领时用较长基线（queued 阶段没必要亚秒感知）；
//   - 已被认领时保持基线，实时性不变；
//   - 连续空轮次逐级翻倍退避、封顶，一旦取到新事件立即回基线
//     （运行中的 run 一直在产生事件，因此退避几乎不会影响实时性）。
func sseNextPollDelay(claimed bool, idlePolls int) time.Duration {
	delay := sseQueuedPollInterval
	if claimed {
		delay = ssePollInterval
	}
	for i := 0; i < idlePolls && delay < ssePollMaxInterval; i++ {
		delay *= 2
	}
	if delay > ssePollMaxInterval {
		delay = ssePollMaxInterval
	}
	return delay
}

func bindAgentJSON(c *gin.Context, destination any) error {
	if c.Request == nil || c.Request.Body == nil {
		return fmt.Errorf("agent request body is missing")
	}
	// Field validation happens after decoding; cap the decoder first so an
	// authenticated caller cannot turn a large ignored JSON field into a
	// memory-amplification request.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAgentBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("agent request contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeAgentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agentcore.ErrValidation):
		response.BadRequestWithCode(c, response.ErrorCode("AGENT_INVALID_REQUEST"), "invalid Agent request")
	case errors.Is(err, agentcore.ErrNotFound):
		response.NotFoundWithCode(c, response.ErrorCode("AGENT_NOT_FOUND"), "Agent resource not found")
	case errors.Is(err, agentcore.ErrIdempotencyConflict):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("AGENT_IDEMPOTENCY_CONFLICT"), "idempotency key belongs to another request")
	case errors.Is(err, agentcore.ErrConflict):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("AGENT_CONFLICT"), "Agent resource conflict")
	case errors.Is(err, agentcore.ErrDefinitionDisabled):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("AGENT_DISABLED"), "Agent is disabled")
	case errors.Is(err, agentcore.ErrNoPublishedVersion):
		response.ErrorWithCode(c, http.StatusConflict, response.ErrorCode("AGENT_VERSION_REQUIRED"), "publish an Agent version before running")
	case errors.Is(err, agentcore.ErrToolNotFound), errors.Is(err, agentcore.ErrUnsafeTool):
		response.BadRequestWithCode(c, response.ErrorCode("AGENT_TOOL_INVALID"), "selected Agent tool is unavailable")
	default:
		response.InternalServerErrorWithCode(c, response.ErrorCode("AGENT_INTERNAL_ERROR"), err.Error())
	}
}

func (h *Handler) ListAgentDefinitions(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	page, pageSize := parseAgentPagination(c)
	items, total, err := h.agents.ListDefinitions(c.Request.Context(), page, pageSize)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.Success(c, gin.H{"list": items, "total": total})
}

func (h *Handler) CreateAgentDefinition(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var input agentcore.CreateDefinitionInput
	if err := bindAgentJSON(c, &input); err != nil {
		writeAgentError(c, agentcore.ErrValidation)
		return
	}
	definition, err := h.agents.CreateDefinition(c.Request.Context(), uint64(userID), input)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Agent 已创建", definition)
}

func (h *Handler) GetAgentDefinition(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	definition, err := h.agents.GetDefinition(c.Request.Context(), id)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	versions, err := h.agents.ListVersions(c.Request.Context(), id)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.Success(c, gin.H{"definition": definition, "versions": versions})
}

func (h *Handler) UpdateAgentDefinition(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	var input agentcore.UpdateDefinitionInput
	if err := bindAgentJSON(c, &input); err != nil {
		writeAgentError(c, agentcore.ErrValidation)
		return
	}
	definition, err := h.agents.UpdateDefinition(c.Request.Context(), id, input)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Agent 已更新", definition)
}

func (h *Handler) ListAgentVersions(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	versions, err := h.agents.ListVersions(c.Request.Context(), id)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.Success(c, versions)
}

func (h *Handler) PublishAgentVersion(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var input agentcore.PublishVersionInput
	if err := bindAgentJSON(c, &input); err != nil {
		writeAgentError(c, agentcore.ErrValidation)
		return
	}
	version, err := h.agents.PublishVersion(c.Request.Context(), id, uint64(userID), input)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Agent 版本已发布", version)
}

func (h *Handler) ListAgentTools(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	response.Success(c, h.agents.ToolDefinitions())
}

func (h *Handler) CreateAgentRun(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	definitionID, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	var input agentcore.CreateRunInput
	if err := bindAgentJSON(c, &input); err != nil {
		writeAgentError(c, agentcore.ErrValidation)
		return
	}
	input.IdempotencyKey = c.GetHeader("Idempotency-Key")
	result, err := h.agents.CreateRun(c.Request.Context(), definitionID, uint64(userID), input)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Agent Run 已提交", result)
}

func (h *Handler) ListAgentRuns(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	page, pageSize := parseAgentPagination(c)
	definitionID, valid := parseOptionalAgentID(c)
	if !valid {
		return
	}
	items, total, err := h.agents.ListRuns(c.Request.Context(), definitionID, c.Query("status"), page, pageSize)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.Success(c, gin.H{"list": items, "total": total})
}

func (h *Handler) GetAgentRun(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	// 与SSE共用严格游标校验；坏游标不能悄悄退回首批。
	afterID, valid := parseEventCursor(c)
	if !valid {
		return
	}
	details, err := h.agents.GetRunDetailsAfter(c.Request.Context(), id, afterID)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.Success(c, details)
}

func (h *Handler) CancelAgentRun(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	id, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	run, err := h.agents.CancelRun(c.Request.Context(), id)
	if err != nil {
		writeAgentError(c, err)
		return
	}
	response.SuccessWithMessage(c, "Agent Run 已取消", run)
}

func (h *Handler) StreamAgentRunEvents(c *gin.Context) {
	if !h.requireAgentService(c) {
		return
	}
	runID, ok := parseAgentID(c, "id")
	if !ok {
		return
	}
	afterID, valid := parseEventCursor(c)
	if !valid {
		return
	}
	// Authorize ownership before committing SSE headers.
	if _, _, err := h.agents.ListEventsAfter(c.Request.Context(), runID, afterID, 1); err != nil {
		writeAgentError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()

	// 用可重置计时器而不是固定 ticker：间隔要随"是否已被认领"与"连续空轮次"变化。
	claimed := false
	idlePolls := 0
	pollTimer := time.NewTimer(sseNextPollDelay(false, 0))
	defer pollTimer.Stop()
	keepAlive := time.NewTicker(sseKeepAliveInterval)
	defer keepAlive.Stop()
	for {
		events, terminal, err := h.agents.ListEventsAfter(c.Request.Context(), runID, afterID, 200)
		if err != nil {
			writeSSE(c, "error", 0, gin.H{"message": "event stream unavailable"})
			flusher.Flush()
			return
		}
		for _, event := range events {
			if !writeSSE(c, event.EventType, event.ID, event) {
				return
			}
			afterID = event.ID
			// run.started / run.resumed 是 Worker 认领时写入的（repository_worker.go），
			// 见到它就说明 run 已进入执行阶段，轮询该回到实时基线。
			if event.EventType == "run.started" || event.EventType == "run.resumed" {
				claimed = true
			}
		}
		if len(events) > 0 {
			idlePolls = 0
			flusher.Flush()
		} else {
			idlePolls++
		}
		if terminal && len(events) < 200 {
			if !writeSSE(c, "done", afterID, gin.H{"run_id": runID}) {
				return
			}
			flusher.Flush()
			return
		}
		pollTimer.Reset(sseNextPollDelay(claimed, idlePolls))
		select {
		case <-c.Request.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-pollTimer.C:
		}
	}
}

func parseOptionalAgentID(c *gin.Context) (uint64, bool) {
	value := strings.TrimSpace(c.Query("agent_id"))
	if value == "" {
		return 0, true
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 || id > maxAgentDBID {
		response.BadRequestWithCode(c, response.ErrorCode("AGENT_INVALID_ID"), "invalid agent filter id")
		return 0, false
	}
	return id, true
}

func parseEventCursor(c *gin.Context) (uint64, bool) {
	value := strings.TrimSpace(c.Query("after_id"))
	if value == "" {
		value = strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	}
	if value == "" {
		return 0, true
	}
	cursor, err := strconv.ParseUint(value, 10, 64)
	if err != nil || cursor > maxAgentDBID {
		response.BadRequestWithCode(c, response.ErrorCode("AGENT_INVALID_CURSOR"), "invalid event cursor")
		return 0, false
	}
	return cursor, true
}

func writeSSE(c *gin.Context, eventType string, id uint64, value any) bool {
	eventType = safeSSEEventType(eventType)
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	if id > 0 && !writeSSEText(c.Writer, fmt.Sprintf("id: %d\n", id)) {
		return false
	}
	return writeSSEText(c.Writer, fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, encoded))
}

func writeSSEText(writer io.Writer, value string) bool {
	written, err := io.WriteString(writer, value)
	return err == nil && written == len(value)
}

func safeSSEEventType(eventType string) string {
	if eventType == "" {
		return "message"
	}
	for _, char := range eventType {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '.' && char != '-' && char != '_' {
			// Event names come from the database; reject malformed values rather
			// than allowing a newline/colon to inject another SSE field.
			return "message"
		}
	}
	return eventType
}
