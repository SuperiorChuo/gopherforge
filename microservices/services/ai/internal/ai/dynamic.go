package ai

import (
	"context"
	"sync"
)

// OptionsSource returns the provider options in effect right now. Wired to
// the runtime-config reader in main so console edits to the ai.provider
// setting take effect without a restart.
type OptionsSource func(ctx context.Context) Options

// NewDynamicProviders returns a Providers pair whose backends re-resolve
// their options on every call. The underlying clients are rebuilt only when
// the options actually change, so the steady-state cost is one mutex read.
func NewDynamicProviders(source OptionsSource) Providers {
	d := &dynamicState{source: source}
	return Providers{
		Chat:  &dynamicChat{state: d},
		Embed: &dynamicEmbed{state: d},
	}
}

type dynamicState struct {
	source OptionsSource

	mu        sync.RWMutex
	opts      Options
	providers Providers
	built     bool
}

// resolve returns providers built from the current options, rebuilding when
// options differ from the cached generation.
func (d *dynamicState) resolve(ctx context.Context) Providers {
	opts := d.source(ctx)

	d.mu.RLock()
	if d.built && d.opts == opts {
		providers := d.providers
		d.mu.RUnlock()
		return providers
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.built || d.opts != opts {
		d.providers = NewProviders(opts)
		d.opts = opts
		d.built = true
	}
	return d.providers
}

type dynamicChat struct {
	state *dynamicState
}

func (p *dynamicChat) Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error {
	return p.state.resolve(ctx).Chat.Chat(ctx, msgs, onDelta)
}

func (p *dynamicChat) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.state.resolve(ctx).Chat.Embed(ctx, texts)
}

func (p *dynamicChat) Name() string {
	return p.state.resolve(context.Background()).Chat.Name()
}

type dynamicEmbed struct {
	state *dynamicState
}

func (p *dynamicEmbed) Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error {
	return p.state.resolve(ctx).Embed.Chat(ctx, msgs, onDelta)
}

func (p *dynamicEmbed) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.state.resolve(ctx).Embed.Embed(ctx, texts)
}

func (p *dynamicEmbed) Name() string {
	return p.state.resolve(context.Background()).Embed.Name()
}
