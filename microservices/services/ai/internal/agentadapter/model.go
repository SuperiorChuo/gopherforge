// Package agentadapter connects Agent Kernel ports to AI service infrastructure.
package agentadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-admin-kit/services/ai/internal/agentcore"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

type ModelConfig struct {
	ID              uint64
	Name            string
	Provider        string
	BaseURL         string
	APIKey          string
	Enabled         bool
	CostPer1KInput  float64
	CostPer1KOutput float64
}

type LoadModelFunc func(context.Context, uint64) (*ModelConfig, error)
type BeforeModelCallFunc func(context.Context, uint64, uint) error

type UsageRecord struct {
	ModelID      uint64
	ModelName    string
	ActorID      uint64
	TenantID     uint
	RunID        uint64
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Cost         float64
	LatencyMS    int
	Status       string
	ErrorMessage string
}

type RecordUsageFunc func(context.Context, UsageRecord) error

const (
	maxAgentModelToolCalls    = 8
	maxAgentModelContentBytes = 128 * 1024
	maxAgentToolArguments     = 16 * 1024
)

type ModelPort struct {
	load       LoadModelFunc
	before     BeforeModelCallFunc
	record     RecordUsageFunc
	onUsageErr func(error)
	now        func() time.Time
}

func NewModelPort(load LoadModelFunc, before BeforeModelCallFunc, record RecordUsageFunc, onUsageErr func(error)) *ModelPort {
	return &ModelPort{load: load, before: before, record: record, onUsageErr: onUsageErr, now: time.Now}
}

func (p *ModelPort) Validate(ctx context.Context, modelID uint64, requiresTools bool) error {
	if _, err := tenant.Require(ctx); err != nil {
		return fmt.Errorf("%w: tenant context required", agentcore.ErrValidation)
	}
	config, provider, err := p.resolve(ctx, modelID)
	if err != nil {
		return fmt.Errorf("%w: %v", agentcore.ErrValidation, err)
	}
	if requiresTools {
		if _, supported := provider.(aiclient.ToolCallingProvider); !supported {
			return fmt.Errorf("%w: model %s does not support structured tool calls", agentcore.ErrValidation, config.Name)
		}
	}
	return nil
}

func (p *ModelPort) Complete(ctx context.Context, request agentcore.ModelRequest) (agentcore.ModelResponse, error) {
	tenantID, err := tenant.Require(ctx)
	if err != nil {
		return agentcore.ModelResponse{}, fmt.Errorf("agent model call requires tenant context")
	}
	config, provider, err := p.resolve(ctx, request.ModelID)
	if err != nil {
		return agentcore.ModelResponse{}, err
	}
	if p.before != nil {
		if err := p.before(ctx, request.ActorID, tenantID); err != nil {
			return agentcore.ModelResponse{}, err
		}
	}
	toolProvider, supported := provider.(aiclient.ToolCallingProvider)
	clock := p.now
	if clock == nil {
		clock = time.Now
	}
	started := clock()
	var providerResponse aiclient.ToolChatResponse
	// providerErr 是 provider 调用失败；callErr 还额外承载本地响应校验错误。
	// 两者对外的处理不同：只有 providerErr 里"含上游正文"的那部分需要脱敏。
	var providerErr error
	if supported {
		providerRequest := aiclient.ToolChatRequest{
			Messages: make([]aiclient.ToolChatMessage, 0, len(request.Messages)),
			Tools:    make([]aiclient.ToolDefinition, 0, len(request.Tools)),
		}
		for _, message := range request.Messages {
			converted := aiclient.ToolChatMessage{
				Role: message.Role, Content: message.Content, ToolCallID: message.ToolCallID,
				ToolCalls: make([]aiclient.ToolCall, 0, len(message.ToolCalls)),
			}
			for _, call := range message.ToolCalls {
				converted.ToolCalls = append(converted.ToolCalls, aiclient.ToolCall{
					ID: call.ID, Name: call.Name, Arguments: json.RawMessage(call.Arguments.Clone()),
				})
			}
			providerRequest.Messages = append(providerRequest.Messages, converted)
		}
		for _, tool := range request.Tools {
			providerRequest.Tools = append(providerRequest.Tools, aiclient.ToolDefinition{
				Name: tool.Name, Description: tool.Description, InputSchema: json.RawMessage(tool.InputSchema.Clone()),
			})
		}
		providerResponse, providerErr = toolProvider.CompleteTools(ctx, providerRequest)
	} else {
		if len(request.Tools) > 0 {
			return agentcore.ModelResponse{}, fmt.Errorf("model %s does not support structured tool calls", config.Name)
		}
		providerResponse, providerErr = completePlainChat(ctx, provider, request)
	}
	callErr := providerErr
	if callErr == nil {
		callErr = validateProviderResponse(providerResponse)
	}
	latency := int(clock().Sub(started).Milliseconds())
	// 用量记账不应因模型上下文取消而丢失，但也不能让故障的记账
	// 存储无限期卡住 Worker；保留租户值并给独立的短 deadline。
	recordCtx, cancelRecord := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	p.recordUsage(recordCtx, config, request, tenantID, providerResponse.Usage, latency, callErr)
	cancelRecord()
	if callErr != nil {
		if providerErr != nil {
			logAgentModelFailure(providerErr, request.RunID, config.ID)
			// 只有"含上游正文"的错误需要脱敏；provider 层的本地错误（decode 失败、
			// choices 数不符、请求编码失败等）不含上游正文，原样透传以保留可诊断的原因。
			var upstream *aiclient.UpstreamError
			if errors.As(providerErr, &upstream) {
				return agentcore.ModelResponse{}, kernelSafeProviderError(providerErr)
			}
			return agentcore.ModelResponse{}, providerErr
		}
		return agentcore.ModelResponse{}, callErr
	}
	response := agentcore.ModelResponse{
		Content: providerResponse.Content, StopReason: providerResponse.StopReason,
		Usage:     agentcore.Usage{InputTokens: providerResponse.Usage.PromptTokens, OutputTokens: providerResponse.Usage.CompletionTokens},
		ToolCalls: make([]agentcore.ToolCall, 0, len(providerResponse.ToolCalls)),
	}
	for _, call := range providerResponse.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, agentcore.ToolCall{
			ID: call.ID, Name: call.Name, Arguments: agentcore.JSONValue(append([]byte(nil), call.Arguments...)),
		})
	}
	return response, nil
}

func completePlainChat(ctx context.Context, provider aiclient.Provider, request agentcore.ModelRequest) (aiclient.ToolChatResponse, error) {
	messages := make([]aiclient.ChatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if len(message.ToolCalls) > 0 || message.ToolCallID != "" {
			return aiclient.ToolChatResponse{}, fmt.Errorf("plain model cannot receive tool transcript")
		}
		messages = append(messages, aiclient.ChatMessage{Role: message.Role, Content: message.Content})
	}
	var content strings.Builder
	var usage aiclient.Usage
	callErr := provider.Chat(ctx, messages, func(delta aiclient.ChatDelta) error {
		if content.Len()+len(delta.Content) > maxAgentModelContentBytes {
			return fmt.Errorf("model response exceeds content limit")
		}
		content.WriteString(delta.Content)
		if delta.Usage != nil {
			usage = *delta.Usage
		}
		return nil
	})
	if callErr != nil {
		return aiclient.ToolChatResponse{}, callErr
	}
	return aiclient.ToolChatResponse{Content: content.String(), Usage: usage, StopReason: "stop"}, nil
}

// logAgentModelFailure 把上游原文写进服务日志（对外错误已被脱敏，这里是诊断的唯一落点）。
//
// 为什么要判空：共享 logger 的包级变量只有 InitLogger 之后才非 nil，而它是直接解引用调用的
// （logger.Warn → Logger.Warn），未初始化时调用会 nil panic。生产入口在启动时初始化，
// 但测试与任何早于初始化的路径都会踩到，这里显式挡住。
func logAgentModelFailure(err error, runID uint64, modelID uint64) {
	if logger.Logger == nil {
		return
	}
	logger.Warn("agent model call failed",
		logger.Err(err),
		logger.Int64("run_id", int64(runID)),
		logger.Int64("model_id", int64(modelID)))
}

// kernelSafeProviderError 把**含上游正文**的错误收敛成稳定的短摘要。
//
// 为什么必须收敛：Kernel 会把模型调用错误写进 run.error_message，而该字段随
// GET /runs/:id 与 SSE 详情对持 ai:agent:list 的用户可见；provider 层的 apiError
// 会把上游响应体（最多 4096 字节）折进 error，直接外发等于泄漏上游原文。
//
// 只处理 aiclient.UpstreamError（含正文的那两类）；其余本地/传输错误由调用方原样透传。
// 完整诊断不丢：调用方在此之前已把原始错误写进服务日志。
func kernelSafeProviderError(err error) error {
	if code, ok := providerStatusFromError(err); ok {
		return fmt.Errorf("model provider rejected the request with status %s", code)
	}
	return fmt.Errorf("model provider call failed")
}

// providerStatusFromError 只从 provider 错误里取回 HTTP 状态码，丢弃其后的上游正文。
// 依赖 provider 层 apiError 的既有格式 "<provider> api status <code>: <body>"。
func providerStatusFromError(err error) (string, bool) {
	const marker = " api status "
	message := err.Error()
	index := strings.Index(message, marker)
	if index < 0 {
		return "", false
	}
	rest := message[index+len(marker):]
	if end := strings.IndexByte(rest, ':'); end >= 0 {
		rest = rest[:end]
	}
	code := strings.TrimSpace(rest)
	if len(code) != 3 {
		return "", false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	return code, true
}

func validateProviderResponse(response aiclient.ToolChatResponse) error {
	if len(response.Content) > maxAgentModelContentBytes || len(response.ToolCalls) > maxAgentModelToolCalls || response.Usage.PromptTokens < 0 || response.Usage.CompletionTokens < 0 || response.Usage.TotalTokens < 0 {
		return fmt.Errorf("model response exceeds limits")
	}
	for _, call := range response.ToolCalls {
		if call.ID == "" || len(call.ID) > 128 || call.Name == "" || len(call.Name) > 64 || len(call.Arguments) == 0 || len(call.Arguments) > maxAgentToolArguments || !json.Valid(call.Arguments) {
			return fmt.Errorf("model returned an invalid structured tool call")
		}
	}
	if response.Content == "" && len(response.ToolCalls) == 0 {
		return fmt.Errorf("model returned neither content nor tool calls")
	}
	return nil
}

func (p *ModelPort) resolve(ctx context.Context, modelID uint64) (*ModelConfig, aiclient.Provider, error) {
	if p == nil || p.load == nil || modelID == 0 {
		return nil, nil, fmt.Errorf("agent model adapter is not configured")
	}
	config, err := p.load(ctx, modelID)
	if err != nil {
		return nil, nil, err
	}
	if config == nil || config.ID != modelID || !config.Enabled || config.Name == "" {
		return nil, nil, fmt.Errorf("agent model is unavailable")
	}
	if config.APIKey == "" && config.BaseURL == "" {
		return nil, nil, fmt.Errorf("agent model has no configured endpoint or credential")
	}
	providers := aiclient.NewProviders(aiclient.Options{
		Provider: config.Provider, BaseURL: config.BaseURL, APIKey: config.APIKey, ChatModel: config.Name,
	})
	if providers.Chat == nil {
		return nil, nil, fmt.Errorf("agent model provider is unavailable")
	}
	return config, providers.Chat, nil
}

func (p *ModelPort) recordUsage(
	ctx context.Context,
	config *ModelConfig,
	request agentcore.ModelRequest,
	tenantID uint,
	usage aiclient.Usage,
	latency int,
	callErr error,
) {
	if p.record == nil {
		return
	}
	status := "success"
	errorMessage := ""
	if callErr != nil {
		status = "error"
		errorMessage = "model call failed"
	}
	cost := float64(usage.PromptTokens)/1000*config.CostPer1KInput +
		float64(usage.CompletionTokens)/1000*config.CostPer1KOutput
	recordCtx := tenant.WithContext(ctx, tenantID)
	err := p.record(recordCtx, UsageRecord{
		ModelID: config.ID, ModelName: config.Name, ActorID: request.ActorID, TenantID: tenantID, RunID: request.RunID,
		InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens,
		Cost: cost, LatencyMS: latency, Status: status, ErrorMessage: errorMessage,
	})
	if err != nil && p.onUsageErr != nil && !errors.Is(err, context.Canceled) {
		p.onUsageErr(err)
	}
}
