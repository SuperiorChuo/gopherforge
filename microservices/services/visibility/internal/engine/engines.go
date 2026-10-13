package engine

// Generative-engine abstraction for GEO detection. A run targets ONE engine;
// each engine either grounds itself (real engines like ChatGPT with web search,
// returning structured citations) or is grounded by the caller (selfhost, which
// uses injected retrieved sources). The mention analysis runs on the answer
// text regardless of engine.

import (
	"context"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

// Config is the credential surface an engine adapter needs.
// 与 runner.AIConfig 同形，但定义在本包以避免 engine → runner 的循环依赖。
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// EngineAnswer is one engine's grounded answer plus its real citations.
type EngineAnswer struct {
	// Answer 回答正文。
	Answer string
	// Sources 引擎引用的来源。真实引擎 = 结构化引用（Cited=true）；
	// 自建引擎 = 调用方注入的检索来源（经 ExtractCitations 标记 Cited）。
	Sources []model.GeoSource
	// Queries 真实引擎实际发起的检索词（如 ChatGPT web_search_call 的 queries）。
	Queries []string
}

// Engine is a generative-engine adapter.
type Engine interface {
	// Name 引擎标识：chatgpt | selfhost（预留 perplexity | gemini）。
	Name() string
	// Ask 向该引擎提问并返回回答 + 引用。injected 仅自建引擎使用（调用方已
	// 检索注入的来源）；真实引擎自备检索，忽略 injected。
	Ask(ctx context.Context, cfg Config, question string, injected []model.GeoSource) (EngineAnswer, error)
}

// NewEngine 按名字装配引擎适配器；未知名字返回 nil。
func NewEngine(name string) Engine {
	switch name {
	case "chatgpt":
		return chatgptEngine{}
	case "perplexity":
		return perplexityEngine{}
	case "gemini":
		return geminiEngine{}
	case "selfhost", "":
		return selfhostEngine{}
	}
	return nil
}
