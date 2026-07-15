package ai

import "strings"

// Options selects and configures providers, mirroring the AI_* environment
// variables so callers can map config onto it directly.
type Options struct {
	Provider      string
	BaseURL       string
	APIKey        string
	ChatModel     string
	EmbedModel    string
	EmbedProvider string
	EmbedBaseURL  string
	EmbedAPIKey   string
}

// Providers bundles the chat and embedding backends selected by Options.
// Chat and Embed may point at the same provider instance.
type Providers struct {
	Chat  Provider
	Embed Provider
}

// NewProviders builds chat and embedding providers from opts. The embedding
// provider follows the chat provider unless AI_EMBED_PROVIDER (or the embed
// base URL / API key) overrides it. Callers must gate on APIKey being
// non-empty before invoking the returned providers.
func NewProviders(opts Options) Providers {
	chat := newProvider(opts.Provider, opts.BaseURL, opts.APIKey, opts.ChatModel, opts.EmbedModel)

	embedProvider := strings.TrimSpace(opts.EmbedProvider)
	embedBaseURL := strings.TrimSpace(opts.EmbedBaseURL)
	embedAPIKey := strings.TrimSpace(opts.EmbedAPIKey)
	if embedProvider == "" && embedBaseURL == "" && embedAPIKey == "" {
		return Providers{Chat: chat, Embed: chat}
	}

	if embedProvider == "" {
		embedProvider = opts.Provider
	}
	if embedBaseURL == "" {
		embedBaseURL = opts.BaseURL
	}
	if embedAPIKey == "" {
		embedAPIKey = opts.APIKey
	}
	embed := newProvider(embedProvider, embedBaseURL, embedAPIKey, opts.ChatModel, opts.EmbedModel)
	return Providers{Chat: chat, Embed: embed}
}

// newProvider maps a provider name onto an implementation, defaulting to the
// OpenAI-compatible client for unknown or empty names.
func newProvider(name, baseURL, apiKey, chatModel, embedModel string) Provider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "anthropic":
		return NewAnthropicProvider(baseURL, apiKey, chatModel)
	default:
		return NewOpenAICompatProvider(baseURL, apiKey, chatModel, embedModel)
	}
}
