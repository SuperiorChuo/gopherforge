package bot

import (
	"context"
	"strings"
	"sync"
)

// Overrides are console-managed fields layered over the env-based Config.
// Empty fields keep the env value. Provider "anthropic" is not supported by
// the IM bot (OpenAI-compatible only) and disables the override entirely.
type Overrides struct {
	Provider  string
	BaseURL   string
	APIKey    string
	ChatModel string
}

// OverridesSource returns the current console overrides; the zero value means
// "no override, use env config".
type OverridesSource func(ctx context.Context) Overrides

// Dynamic re-resolves the effective client per call so console edits to the
// ai.provider setting apply without restarting im-service. The inner client
// is rebuilt only when the effective config changes.
type Dynamic struct {
	base   Config
	source OverridesSource

	mu      sync.RWMutex
	current Config
	client  Client
	built   bool
}

func NewDynamic(base Config, source OverridesSource) *Dynamic {
	return &Dynamic{base: base.WithDefaults(), source: source}
}

func (d *Dynamic) Name() string {
	return d.resolve(context.Background()).Name()
}

func (d *Dynamic) Complete(ctx context.Context, system string, history []Message) (string, error) {
	return d.resolve(ctx).Complete(ctx, system, history)
}

func (d *Dynamic) resolve(ctx context.Context) Client {
	cfg := d.effectiveConfig(ctx)

	d.mu.RLock()
	if d.built && d.current == cfg {
		client := d.client
		d.mu.RUnlock()
		return client
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.built || d.current != cfg {
		d.client = NewClient(cfg)
		d.current = cfg
		d.built = true
	}
	return d.client
}

func (d *Dynamic) effectiveConfig(ctx context.Context) Config {
	cfg := d.base
	if d.source == nil {
		return cfg
	}
	o := d.source(ctx)
	// IM bot 只讲 OpenAI 兼容协议；控制台选了 anthropic 时忽略整组覆盖，
	// 走环境变量（或 stub），避免拿 anthropic Key 打 openai 端点。
	if strings.EqualFold(o.Provider, "anthropic") {
		return cfg
	}
	if o.BaseURL != "" {
		cfg.BaseURL = strings.TrimRight(o.BaseURL, "/")
	}
	if o.APIKey != "" {
		cfg.APIKey = o.APIKey
	}
	if o.ChatModel != "" {
		cfg.Model = o.ChatModel
	}
	return cfg
}
