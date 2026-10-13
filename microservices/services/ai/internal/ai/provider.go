// Package ai defines the model-provider abstraction used by the AI service.
// Providers implement streaming chat and batch embeddings over plain
// net/http, without any third-party SDK.
package ai

import (
	"context"
	"fmt"
)

// UpstreamError 标记"错误里含上游响应正文"——目前只有两处产生点：HTTP 非 200 时
// apiError 折入的响应体，以及流内错误帧（data: {"error":{...}}）。
//
// 为什么要专门标记：调用方（Kernel）会把模型错误写进 run.error_message，而该字段
// 对持 ai:agent:list 的用户可见，含正文的错误必须脱敏；但 provider 层还有大量
// **本地**错误（decode 失败、choices 数不符、请求编码失败等），它们不含上游正文，
// 一律收敛会把可诊断的原因抹平成一句"provider call failed"。类型标记让调用方能精确区分。
type UpstreamError struct{ err error }

func newUpstreamError(format string, args ...any) *UpstreamError {
	return &UpstreamError{err: fmt.Errorf(format, args...)}
}

func (e *UpstreamError) Error() string { return e.err.Error() }

func (e *UpstreamError) Unwrap() error { return e.err }

// ChatMessage is one turn of a chat conversation.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatDelta is one streamed fragment of a chat completion. Done marks the
// final delta; Content may be empty on the final delta.
type ChatDelta struct {
	Content string
	Done    bool
	Usage   *Usage // 仅在 Done=true 时有值（流式响应的最后一帧）
}

// Usage holds token consumption statistics for an AI call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ImageOptions configures image generation.
type ImageOptions struct {
	Size           string // 如 "1024x1024"，空则用默认
	N              int    // 生成张数，默认 1
	ResponseFormat string // "url" 或 "b64_json"，默认 "url"
}

// ImageResult holds generated images (URL 或 base64，取决于 ResponseFormat)。
type ImageResult struct {
	URLs []string
	B64  []string
}

// Provider is a chat + embedding + image backend.
type Provider interface {
	// Chat streams a completion for msgs, invoking onDelta once per
	// fragment. Returning an error from onDelta aborts the stream.
	Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error
	// Embed returns one embedding vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// GenerateImage produces one or more images for prompt. The result
	// carries URLs or base64 payloads depending on opts.ResponseFormat.
	GenerateImage(ctx context.Context, prompt string, opts ImageOptions) (ImageResult, error)
	// Name identifies the provider implementation.
	Name() string
}

// Chat message roles shared by all providers.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)
