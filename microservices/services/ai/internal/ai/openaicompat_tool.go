package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type openAIToolRequest struct {
	Model    string                 `json:"model"`
	Messages []openAIToolMessage    `json:"messages"`
	Tools    []openAIToolDefinition `json:"tools,omitempty"`
	Stream   bool                   `json:"stream"`
}

type openAIToolMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIToolResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// CompleteTools performs a non-streaming OpenAI-compatible structured tool
// completion. Tool arguments stay as JSON and are never parsed from prose.
func (p *OpenAICompatProvider) CompleteTools(ctx context.Context, request ToolChatRequest) (ToolChatResponse, error) {
	payload := openAIToolRequest{Model: p.chatModel, Stream: false}
	for _, message := range request.Messages {
		converted := openAIToolMessage{
			Role: message.Role, Content: message.Content, ToolCallID: message.ToolCallID,
		}
		for _, call := range message.ToolCalls {
			providerCall := openAIToolCall{ID: call.ID, Type: "function"}
			providerCall.Function.Name = call.Name
			providerCall.Function.Arguments = string(call.Arguments)
			converted.ToolCalls = append(converted.ToolCalls, providerCall)
		}
		payload.Messages = append(payload.Messages, converted)
	}
	for _, tool := range request.Tools {
		if !json.Valid(tool.InputSchema) {
			return ToolChatResponse{}, fmt.Errorf("openai tool %s has invalid input schema", tool.Name)
		}
		definition := openAIToolDefinition{Type: "function"}
		definition.Function.Name = tool.Name
		definition.Function.Description = tool.Description
		definition.Function.Parameters = tool.InputSchema
		payload.Tools = append(payload.Tools, definition)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("openai tool request encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("openai tool request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ToolChatResponse{}, fmt.Errorf("openai tool request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ToolChatResponse{}, apiError("openai", resp)
	}
	var decoded openAIToolResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024))
	if err := decoder.Decode(&decoded); err != nil {
		return ToolChatResponse{}, fmt.Errorf("openai tool response decode: %w", err)
	}
	if len(decoded.Choices) != 1 {
		return ToolChatResponse{}, fmt.Errorf("openai tool response choices = %d, want 1", len(decoded.Choices))
	}
	choice := decoded.Choices[0]
	result := ToolChatResponse{
		Content: choice.Message.Content, StopReason: choice.FinishReason,
		Usage: Usage{PromptTokens: decoded.Usage.PromptTokens, CompletionTokens: decoded.Usage.CompletionTokens, TotalTokens: decoded.Usage.TotalTokens},
	}
	for _, call := range choice.Message.ToolCalls {
		arguments := json.RawMessage(call.Function.Arguments)
		if call.ID == "" || call.Function.Name == "" || !json.Valid(arguments) {
			return ToolChatResponse{}, fmt.Errorf("openai returned invalid structured tool call")
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: arguments})
	}
	return result, nil
}
