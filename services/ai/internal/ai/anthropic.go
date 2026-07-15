package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	anthropicMaxTokens      = 4096
)

// ErrAnthropicEmbeddingsUnsupported reports that the Anthropic API has no
// embeddings endpoint.
var ErrAnthropicEmbeddingsUnsupported = errors.New(
	"anthropic provider does not support embeddings; configure AI_EMBED_PROVIDER")

// AnthropicProvider talks to the Anthropic Messages API (/v1/messages with
// stream=true). Embeddings are not supported by Anthropic; configure a
// separate embedding provider via AI_EMBED_PROVIDER.
type AnthropicProvider struct {
	baseURL    string
	apiKey     string
	chatModel  string
	httpClient *http.Client
}

// NewAnthropicProvider builds an Anthropic Messages API provider. An empty
// baseURL defaults to the official Anthropic endpoint.
func NewAnthropicProvider(baseURL, apiKey, chatModel string) *AnthropicProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	return &AnthropicProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		chatModel:  chatModel,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name identifies the provider implementation.
func (p *AnthropicProvider) Name() string { return "anthropic" }

type anthropicChatRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    string                 `json:"system,omitempty"`
	Messages  []anthropicChatMessage `json:"messages"`
	Stream    bool                   `json:"stream"`
}

type anthropicChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat streams a completion through /v1/messages with stream=true. System
// messages are folded into the top-level system field as the Messages API
// requires; only user/assistant turns go into the messages array.
func (p *AnthropicProvider) Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error {
	payload := anthropicChatRequest{
		Model:     p.chatModel,
		MaxTokens: anthropicMaxTokens,
		Stream:    true,
	}
	var systemParts []string
	for _, m := range msgs {
		if m.Role == RoleSystem {
			systemParts = append(systemParts, m.Content)
			continue
		}
		payload.Messages = append(payload.Messages, anthropicChatMessage{Role: m.Role, Content: m.Content})
	}
	payload.System = strings.Join(systemParts, "\n\n")

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("anthropic chat request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("anthropic chat request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic chat request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return apiError("anthropic", resp)
	}

	done := false
	err = readSSE(resp.Body, func(ev sseEvent) error {
		if ev.data == "" {
			return nil
		}
		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(ev.data), &event); err != nil {
			return fmt.Errorf("anthropic stream event decode: %w", err)
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				return onDelta(ChatDelta{Content: event.Delta.Text})
			}
		case "message_stop":
			done = true
			return onDelta(ChatDelta{Done: true})
		case "error":
			return fmt.Errorf("anthropic stream error %s: %s", event.Error.Type, event.Error.Message)
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

// Embed always fails: the Anthropic API does not offer embeddings.
func (p *AnthropicProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, ErrAnthropicEmbeddingsUnsupported
}
