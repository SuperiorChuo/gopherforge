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

// 上游在响应头已发出后出错时会改发 `data: {"error":{...}}`。该帧能被 JSON 成功解析成
// Choices 为空的 chunk，若不显式识别就会被当成"正常结束 + 空内容"——调用方拿到静默空
// 回答且 err=nil，用量还会记成功。本用例钉死这条路径必须报错。
func TestOpenAICompatChatFailsOnStreamErrorFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"error\":{\"type\":\"server_error\",\"message\":\"upstream exploded\"}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	var content strings.Builder
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(delta ChatDelta) error {
		if !delta.Done {
			content.WriteString(delta.Content)
		}
		return nil
	})
	if err == nil {
		t.Fatalf("stream error frame was swallowed as success: content=%q", content.String())
	}
	if !strings.Contains(err.Error(), "server_error") || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("error = %v, want it to carry the stream error frame", err)
	}
}

// 不显式要求 usage 时，官方端点（以及部分兼容网关）不会在流里推 usage 帧，用量与成本
// 会静默记成 0。本用例同时钉住"请求必须带 stream_options.include_usage"与"独立 usage 帧
// 必须被送到调用方"——只钉前者会让"带了但没解析"的实现蒙混过关。
func TestOpenAICompatChatRequestsAndSurfacesStreamUsage(t *testing.T) {
	var gotBody openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode chat request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		// include_usage 下 usage 走一个 choices 为空的独立帧
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	var final ChatDelta
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(delta ChatDelta) error {
		if delta.Done {
			final = delta
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if gotBody.StreamOptions == nil || !gotBody.StreamOptions.IncludeUsage {
		t.Fatalf("chat request did not ask for stream usage: %#v", gotBody.StreamOptions)
	}
	if final.Usage == nil || final.Usage.TotalTokens != 10 {
		t.Fatalf("usage frame was not surfaced to the caller: %#v", final.Usage)
	}
}

// 上游忽略 stream=true 直接回一个普通 JSON 完成体（200 + application/json）时，
// readSSE 一个数据帧都解析不到；若放行，调用方拿到的是"成功 + 空回答"。
func TestOpenAICompatChatRejectsNonEventStreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	var content strings.Builder
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(delta ChatDelta) error {
		if !delta.Done {
			content.WriteString(delta.Content)
		}
		return nil
	})
	if err == nil {
		t.Fatalf("非事件流响应被当成成功空回答：content=%q", content.String())
	}
	if !strings.Contains(err.Error(), "not an event stream") {
		t.Fatalf("错误原因不是非事件流：%v", err)
	}
}

// 内容被安全策略拦下（finish_reason=content_filter）且没有任何内容时，
// 必须显式失败；"成功的空回答"会让调用方把被拦的请求记成成功。
func TestOpenAICompatChatRejectsContentFilteredEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(ChatDelta) error { return nil })
	if err == nil {
		t.Fatal("被内容过滤拦下的空回答被当成了成功")
	}
	if !strings.Contains(err.Error(), "content filter") {
		t.Fatalf("错误原因不是内容过滤：%v", err)
	}
}

// 负控：已经有内容产出时，content_filter 收尾不得把整段回答判失败——
// 否则会把"部分内容 + 被截断"的正常响应变成硬错误。
func TestOpenAICompatChatKeepsContentWhenFilteredAfterOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	var content strings.Builder
	if err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(delta ChatDelta) error {
		if !delta.Done {
			content.WriteString(delta.Content)
		}
		return nil
	}); err != nil {
		t.Fatalf("有内容产出时不应判失败: %v", err)
	}
	if content.String() != "partial" {
		t.Fatalf("内容丢失: %q", content.String())
	}
}

// 错误消息必须加上界：上游异常输出不能把错误串撑成无界字符串。
func TestOpenAICompatChatBoundsStreamErrorMessage(t *testing.T) {
	huge := strings.Repeat("e", 4096)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"error\":{\"type\":\"server_error\",\"message\":\""+huge+"\"}}\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(ChatDelta) error { return nil })
	if err == nil {
		t.Fatal("oversized stream error frame was swallowed")
	}
	if len(err.Error()) > 512 {
		t.Fatalf("error length = %d, want a bounded message", len(err.Error()))
	}
}

// 只校验数量与下标范围不够：上游重复同一个 index 时两者都能通过，结果是有下标没被写入
// （nil 向量）却 err=nil，nil 向量随后会被静默写进向量列。
func TestOpenAICompatEmbedRejectsRepeatedIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[3,4]}]}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	if _, err := provider.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("repeated embedding index was accepted, leaving a nil vector")
	}
}

func TestOpenAICompatEmbedRejectsEmptyVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"index":0,"embedding":[1,2]},{"index":1,"embedding":[]}]}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed", "test-image")
	if _, err := provider.Embed(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatal("empty embedding vector was accepted")
	}
}
