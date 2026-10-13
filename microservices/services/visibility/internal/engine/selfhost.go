package engine

// selfhostEngine 自建接地引擎：用调用方注入的检索来源（bing 搜索 + 抓页，
// 由 runner.groundSources 完成）做 RAG 式回答。injected 为空时回退无来源
// LLMAnswer（诚实降级，不假装检索过）。

import (
	"context"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

type selfhostEngine struct{}

func (selfhostEngine) Name() string { return "selfhost" }

func (selfhostEngine) Ask(ctx context.Context, cfg Config, question string, injected []model.GeoSource) (EngineAnswer, error) {
	if len(injected) > 0 {
		answer, err := GroundedLLMAnswer(ctx, cfg.BaseURL, cfg.APIKey, cfg.Model, question, injected)
		if err != nil {
			return EngineAnswer{}, err
		}
		return EngineAnswer{
			Answer:  answer,
			Sources: ExtractCitations(answer, injected),
		}, nil
	}
	answer, err := LLMAnswer(ctx, cfg.BaseURL, cfg.APIKey, cfg.Model, question)
	if err != nil {
		return EngineAnswer{}, err
	}
	return EngineAnswer{Answer: answer}, nil
}

var _ Engine = (selfhostEngine{})
var _ Engine = (chatgptEngine{})
