package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompleteToolsUsesStructuredProtocol(t *testing.T) {
	var got openAIToolRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{
			"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"knowledge_search","arguments":"{\"query\":\"leases\"}"}}]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}
		}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "key", "model-a", "embed", "image")
	result, err := provider.CompleteTools(context.Background(), ToolChatRequest{
		Messages: []ToolChatMessage{
			{Role: RoleSystem, Content: "be factual"},
			{Role: RoleUser, Content: "find leases"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "old", Name: "knowledge_search", Arguments: json.RawMessage(`{"query":"old"}`)}}},
			{Role: RoleTool, ToolCallID: "old", Content: `{"matches":[]}`},
		},
		Tools: []ToolDefinition{{Name: "knowledge_search", Description: "search", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("CompleteTools() error = %v", err)
	}
	if got.Model != "model-a" || got.Stream || len(got.Tools) != 1 || len(got.Messages) != 4 {
		t.Fatalf("request = %#v", got)
	}
	if got.Messages[2].ToolCalls[0].Function.Arguments != `{"query":"old"}` || got.Messages[3].ToolCallID != "old" {
		t.Fatalf("tool transcript = %#v", got.Messages)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "knowledge_search" || string(result.ToolCalls[0].Arguments) != `{"query":"leases"}` {
		t.Fatalf("tool response = %#v", result)
	}
	if result.StopReason != "tool_calls" || result.Usage.TotalTokens != 16 {
		t.Fatalf("metadata = %#v", result)
	}
}

func TestOpenAICompleteToolsRejectsProseArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"knowledge_search","arguments":"not-json"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()
	provider := NewOpenAICompatProvider(server.URL, "key", "model", "embed", "image")
	_, err := provider.CompleteTools(context.Background(), ToolChatRequest{Messages: []ToolChatMessage{{Role: RoleUser, Content: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "invalid structured tool call") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicCompleteToolsMapsContentBlocks(t *testing.T) {
	var got anthropicToolRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, `{
			"content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"knowledge_search","input":{"query":"checkpoint"}}],
			"stop_reason":"tool_use","usage":{"input_tokens":9,"output_tokens":3}
		}`)
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "key", "claude-test")
	result, err := provider.CompleteTools(context.Background(), ToolChatRequest{
		Messages: []ToolChatMessage{
			{Role: RoleSystem, Content: "system"},
			{Role: RoleUser, Content: "question"},
			{Role: RoleAssistant, Content: "searching", ToolCalls: []ToolCall{{ID: "toolu_old", Name: "knowledge_search", Arguments: json.RawMessage(`{"query":"old"}`)}}},
			{Role: RoleTool, ToolCallID: "toolu_old", Content: `{"matches":[]}`},
		},
		Tools: []ToolDefinition{{Name: "knowledge_search", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("CompleteTools() error = %v", err)
	}
	if got.System != "system" || got.Model != "claude-test" || len(got.Messages) != 3 || len(got.Tools) != 1 {
		t.Fatalf("request = %#v", got)
	}
	if got.Messages[1].Role != RoleAssistant || len(got.Messages[1].Content) != 2 || got.Messages[1].Content[1].Type != "tool_use" {
		t.Fatalf("assistant blocks = %#v", got.Messages[1])
	}
	if got.Messages[2].Role != RoleUser || got.Messages[2].Content[0].Type != "tool_result" || got.Messages[2].Content[0].ToolUseID != "toolu_old" {
		t.Fatalf("tool result blocks = %#v", got.Messages[2])
	}
	if result.Content != "checking" || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "toolu_1" || result.StopReason != "tool_use" || result.Usage.TotalTokens != 12 {
		t.Fatalf("response = %#v", result)
	}
}

func TestAnthropicCompleteToolsRejectsUnknownRole(t *testing.T) {
	provider := NewAnthropicProvider("http://127.0.0.1", "key", "model")
	_, err := provider.CompleteTools(context.Background(), ToolChatRequest{Messages: []ToolChatMessage{{Role: "runtime", Content: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "unsupported message role") {
		t.Fatalf("error = %v", err)
	}
}
