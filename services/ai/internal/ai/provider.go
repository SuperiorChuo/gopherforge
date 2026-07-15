// Package ai defines the model-provider abstraction used by the AI service.
// Providers implement streaming chat and batch embeddings over plain
// net/http, without any third-party SDK.
package ai

import "context"

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
}

// Provider is a chat + embedding backend.
type Provider interface {
	// Chat streams a completion for msgs, invoking onDelta once per
	// fragment. Returning an error from onDelta aborts the stream.
	Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error
	// Embed returns one embedding vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Name identifies the provider implementation.
	Name() string
}

// Chat message roles shared by all providers.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)
