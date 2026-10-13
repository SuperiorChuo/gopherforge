package bot

import (
	"context"
	"strings"
	"sync"
)

// Overrides 是叠加在 env 配置之上的控制台管理字段。
// 空字段保留 env 的值。Provider "anthropic" 不被 IM bot 支持
// （仅 OpenAI 兼容）并会整体禁用该覆盖。
type Overrides struct {
	Provider  string
	BaseURL   string
	APIKey    string
	ChatModel string
}

// OverridesSource 返回当前控制台覆盖；零值表示
// "无覆盖，使用 env 配置"。
type OverridesSource func(ctx context.Context) Overrides

// Dynamic 每次调用都重新解析生效的客户端，使控制台对 ai.provider 的修改
// 无需重启 im-service 即生效。仅当生效配置变化时才会重建内部客户端。
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
		cfg.BaseURL = NormalizeBaseURL(o.BaseURL)
	}
	if o.APIKey != "" {
		cfg.APIKey = o.APIKey
	}
	if o.ChatModel != "" {
		cfg.Model = o.ChatModel
	}
	return cfg
}
