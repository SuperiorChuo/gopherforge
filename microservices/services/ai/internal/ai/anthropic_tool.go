package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropicToolRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    string                 `json:"system,omitempty"`
	Messages  []anthropicToolMessage `json:"messages"`
	Tools     []anthropicToolSpec    `json:"tools,omitempty"`
}

type anthropicToolMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicToolResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// CompleteTools maps the provider-neutral transcript onto Anthropic content
// blocks, including tool_use and tool_result blocks.
func (p *AnthropicProvider) CompleteTools(ctx context.Context, request ToolChatRequest) (ToolChatResponse, error) {
	payload := anthropicToolRequest{Model: p.chatModel, MaxTokens: anthropicMaxTokens}
	var systemParts []string
	appendMessage := func(role string, blocks []anthropicContentBlock) {
		last := len(payload.Messages) - 1
		if last >= 0 && payload.Messages[last].Role == role {
			payload.Messages[last].Content = append(payload.Messages[last].Content, blocks...)
			return
		}
		payload.Messages = append(payload.Messages, anthropicToolMessage{Role: role, Content: blocks})
	}
	for _, message := range request.Messages {
		switch message.Role {
		case RoleSystem:
			if message.Content != "" {
				systemParts = append(systemParts, message.Content)
			}
		case RoleUser:
			appendMessage(RoleUser, []anthropicContentBlock{{Type: "text", Text: message.Content}})
		case RoleAssistant:
			blocks := make([]anthropicContentBlock, 0, 1+len(message.ToolCalls))
			if message.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: message.Content})
			}
			for _, call := range message.ToolCalls {
				if !json.Valid(call.Arguments) {
					return ToolChatResponse{}, fmt.Errorf("anthropic tool call %s has invalid arguments", call.Name)
				}
				blocks = append(blocks, anthropicContentBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: call.Arguments})
			}
			appendMessage(RoleAssistant, blocks)
		case RoleTool:
			appendMessage(RoleUser, []anthropicContentBlock{{Type: "tool_result", ToolUseID: message.ToolCallID, Content: message.Content}})
		default:
			return ToolChatResponse{}, fmt.Errorf("anthropic unsupported message role %q", message.Role)
		}
	}
	payload.System = strings.Join(systemParts, "\n\n")
	for _, tool := range request.Tools {
		if !json.Valid(tool.InputSchema) {
			return ToolChatResponse{}, fmt.Errorf("anthropic tool %s has invalid input schema", tool.Name)
		}
		payload.Tools = append(payload.Tools, anthropicToolSpec{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("anthropic tool request encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("anthropic tool request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("anthropic tool request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ToolChatResponse{}, apiError("anthropic", resp)
	}
	var decoded anthropicToolResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&decoded); err != nil {
		return ToolChatResponse{}, fmt.Errorf("anthropic tool response decode: %w", err)
	}
	result := ToolChatResponse{
		StopReason: decoded.StopReason,
		Usage:      Usage{PromptTokens: decoded.Usage.InputTokens, CompletionTokens: decoded.Usage.OutputTokens, TotalTokens: decoded.Usage.InputTokens + decoded.Usage.OutputTokens},
	}
	var textParts []string
	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			if block.ID == "" || block.Name == "" || !json.Valid(block.Input) {
				return ToolChatResponse{}, fmt.Errorf("anthropic returned invalid structured tool call")
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		}
	}
	result.Content = strings.Join(textParts, "")
	return result, nil
}
