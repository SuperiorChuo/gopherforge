package ai

import (
	"context"
	"encoding/json"
)

// ToolDefinition is the provider-neutral JSON Schema contract sent to a model.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolCall is one structured model request to execute a named tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolChatMessage supports assistant tool calls and matching tool results.
type ToolChatMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type ToolChatRequest struct {
	Messages []ToolChatMessage
	Tools    []ToolDefinition
}

type ToolChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	Usage      Usage
	StopReason string
}

// ToolCallingProvider is an optional capability. Existing Provider stubs do
// not need to implement it; Agent Kernel explicitly rejects missing support.
type ToolCallingProvider interface {
	CompleteTools(ctx context.Context, request ToolChatRequest) (ToolChatResponse, error)
}
