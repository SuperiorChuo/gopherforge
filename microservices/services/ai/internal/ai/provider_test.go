package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAICompatChatParsesSSEStream(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody openAIChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode chat request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "test-key", "test-chat", "test-embed")

	var content strings.Builder
	doneSeen := false
	err := provider.Chat(context.Background(), []ChatMessage{
		{Role: RoleSystem, Content: "be brief"},
		{Role: RoleUser, Content: "hi"},
	}, func(delta ChatDelta) error {
		if delta.Done {
			doneSeen = true
			return nil
		}
		content.WriteString(delta.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if content.String() != "Hello" {
		t.Fatalf("Chat() content = %q, want %q", content.String(), "Hello")
	}
	if !doneSeen {
		t.Fatal("Chat() never delivered a Done delta")
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("Chat() path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Chat() authorization = %q, want Bearer test-key", gotAuth)
	}
	if !gotBody.Stream {
		t.Fatal("Chat() request body stream = false, want true")
	}
	if gotBody.Model != "test-chat" {
		t.Fatalf("Chat() request model = %q, want test-chat", gotBody.Model)
	}
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != RoleSystem || gotBody.Messages[1].Content != "hi" {
		t.Fatalf("Chat() request messages = %+v", gotBody.Messages)
	}
}

func TestOpenAICompatChatPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "bad", "m", "e")
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(ChatDelta) error {
		t.Fatal("onDelta must not be called on API error")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Chat() error = %v, want status 401 error", err)
	}
}

func TestOpenAICompatChatPropagatesOnDeltaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	sentinel := errors.New("consumer stopped")
	provider := NewOpenAICompatProvider(server.URL, "k", "m", "e")
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(ChatDelta) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Chat() error = %v, want sentinel", err)
	}
}

func TestOpenAICompatEmbedOrdersVectorsByIndex(t *testing.T) {
	var gotBody openAIEmbedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("embed path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode embed request: %v", err)
		}
		// Return vectors out of order to prove index-based reassembly.
		_, _ = io.WriteString(w, `{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "k", "m", "test-embed")
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	want := [][]float32{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(vectors, want) {
		t.Fatalf("Embed() vectors = %v, want %v", vectors, want)
	}
	if gotBody.Model != "test-embed" {
		t.Fatalf("Embed() request model = %q, want test-embed", gotBody.Model)
	}
	if !reflect.DeepEqual(gotBody.Input, []string{"a", "b"}) {
		t.Fatalf("Embed() request input = %v", gotBody.Input)
	}
}

func TestOpenAICompatEmbedPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := NewOpenAICompatProvider(server.URL, "k", "m", "e")
	_, err := provider.Embed(context.Background(), []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("Embed() error = %v, want status 429 error", err)
	}
}

func TestAnthropicChatParsesSSEStream(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody anthropicChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode chat request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi \"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"there\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "test-key", "test-chat")

	var content strings.Builder
	doneSeen := false
	err := provider.Chat(context.Background(), []ChatMessage{
		{Role: RoleSystem, Content: "be brief"},
		{Role: RoleUser, Content: "hi"},
	}, func(delta ChatDelta) error {
		if delta.Done {
			doneSeen = true
			return nil
		}
		content.WriteString(delta.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if content.String() != "Hi there" {
		t.Fatalf("Chat() content = %q, want %q", content.String(), "Hi there")
	}
	if !doneSeen {
		t.Fatal("Chat() never delivered a Done delta")
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("Chat() path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "test-key" {
		t.Fatalf("Chat() x-api-key = %q, want test-key", gotKey)
	}
	if gotVersion == "" {
		t.Fatal("Chat() anthropic-version header missing")
	}
	if gotBody.System != "be brief" {
		t.Fatalf("Chat() request system = %q, want folded system prompt", gotBody.System)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != RoleUser {
		t.Fatalf("Chat() request messages = %+v, want single user turn", gotBody.Messages)
	}
	if !gotBody.Stream {
		t.Fatal("Chat() request stream = false, want true")
	}
}

func TestAnthropicChatPropagatesStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n")
	}))
	defer server.Close()

	provider := NewAnthropicProvider(server.URL, "k", "m")
	err := provider.Chat(context.Background(), []ChatMessage{{Role: RoleUser, Content: "hi"}}, func(ChatDelta) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "overloaded_error") {
		t.Fatalf("Chat() error = %v, want overloaded_error", err)
	}
}

func TestAnthropicEmbedReturnsExplicitError(t *testing.T) {
	provider := NewAnthropicProvider("", "k", "m")
	_, err := provider.Embed(context.Background(), []string{"a"})
	if !errors.Is(err, ErrAnthropicEmbeddingsUnsupported) {
		t.Fatalf("Embed() error = %v, want ErrAnthropicEmbeddingsUnsupported", err)
	}
}

func TestNewProvidersDefaultsEmbedToChatProvider(t *testing.T) {
	providers := NewProviders(Options{Provider: "openai", APIKey: "k"})
	if providers.Chat != providers.Embed {
		t.Fatal("NewProviders() embed provider should follow the chat provider by default")
	}
	if providers.Chat.Name() != "openai" {
		t.Fatalf("NewProviders() chat provider = %q, want openai", providers.Chat.Name())
	}
}

func TestNewProvidersSplitsEmbedProvider(t *testing.T) {
	providers := NewProviders(Options{
		Provider:      "anthropic",
		APIKey:        "k",
		EmbedProvider: "openai",
		EmbedBaseURL:  "http://localhost:11434",
	})
	if providers.Chat.Name() != "anthropic" {
		t.Fatalf("NewProviders() chat provider = %q, want anthropic", providers.Chat.Name())
	}
	if providers.Embed.Name() != "openai" {
		t.Fatalf("NewProviders() embed provider = %q, want openai", providers.Embed.Name())
	}
}

func TestReadSSEJoinsMultilineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	var got []sseEvent
	err := readSSE(strings.NewReader(input), func(ev sseEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("readSSE() error = %v", err)
	}
	if len(got) != 1 || got[0].data != "line1\nline2" {
		t.Fatalf("readSSE() events = %+v, want one joined event", got)
	}
}
