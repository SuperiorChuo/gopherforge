package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultOpenAIBaseURL = "https://api.openai.com"

// OpenAICompatProvider talks to any OpenAI-compatible API surface
// (/v1/chat/completions with stream=true and /v1/embeddings). Pointing
// BaseURL at DeepSeek, Qwen, Ollama, or another compatible vendor works
// without code changes.
type OpenAICompatProvider struct {
	baseURL    string
	apiKey     string
	chatModel  string
	embedModel string
	httpClient *http.Client
}

// NewOpenAICompatProvider builds an OpenAI-compatible provider. An empty
// baseURL defaults to the official OpenAI endpoint.
func NewOpenAICompatProvider(baseURL, apiKey, chatModel, embedModel string) *OpenAICompatProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 容忍 OpenAI SDK 惯例的 /v1 结尾（下面自己拼 /v1/...，保留会变成 /v1/v1 打 404）
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAICompatProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		chatModel:  chatModel,
		embedModel: embedModel,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name identifies the provider implementation.
func (p *OpenAICompatProvider) Name() string { return "openai" }

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Chat streams a completion through /v1/chat/completions with stream=true.
func (p *OpenAICompatProvider) Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error {
	payload := openAIChatRequest{
		Model:    p.chatModel,
		Messages: make([]openAIChatMessage, 0, len(msgs)),
		Stream:   true,
	}
	for _, m := range msgs {
		payload.Messages = append(payload.Messages, openAIChatMessage{Role: m.Role, Content: m.Content})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("openai chat request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai chat request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai chat request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return apiError("openai", resp)
	}

	done := false
	err = readSSE(resp.Body, func(ev sseEvent) error {
		if ev.data == "" {
			return nil
		}
		if ev.data == "[DONE]" {
			done = true
			return onDelta(ChatDelta{Done: true})
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(ev.data), &chunk); err != nil {
			return fmt.Errorf("openai stream chunk decode: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := onDelta(ChatDelta{Content: choice.Delta.Content}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !done {
		return onDelta(ChatDelta{Done: true})
	}
	return nil
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns embeddings from /v1/embeddings, ordered by input index.
func (p *OpenAICompatProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(openAIEmbedRequest{Model: p.embedModel, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("openai embed request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai embed request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError("openai", resp)
	}

	var decoded openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("openai embed response decode: %w", err)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("openai embed response returned %d vectors for %d inputs", len(decoded.Data), len(texts))
	}

	vectors := make([][]float32, len(texts))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("openai embed response index %d out of range", item.Index)
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

// apiError reads a bounded error body and folds it into a descriptive error.
func apiError(provider string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("%s api status %d: %s", provider, resp.StatusCode, message)
}
