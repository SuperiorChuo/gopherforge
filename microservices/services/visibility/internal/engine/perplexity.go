package engine

// Perplexity (Sonar) engine adapter — a REAL generative engine with native web
// search. OpenAI-compatible chat completions; the response carries a top-level
// `citations` array of real source URLs. 如网络需要代理，可配置可达渠道
// （配置见 runner.SetExtraEngines）。
//
// Perplexity 文档要点：只有 user 消息驱动检索（system 不参与搜索）；模型从
// 通用知识回答时 citations 可能为空；建议 return_citations=true。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

type perplexityEngine struct{}

func (perplexityEngine) Name() string { return "perplexity" }

type perplexityMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type perplexityRequest struct {
	Model           string              `json:"model"`
	Messages        []perplexityMessage `json:"messages"`
	ReturnCitations bool                `json:"return_citations,omitempty"`
	Temperature     float64             `json:"temperature,omitempty"`
}

type perplexityResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Citations []string `json:"citations"`
}

// Ask 调用 Perplexity chat completions；citations 数组直接映射为引用来源。
// injected 忽略（真实引擎自备检索）。
func (perplexityEngine) Ask(ctx context.Context, cfg Config, question string, _ []model.GeoSource) (EngineAnswer, error) {
	if cfg.APIKey == "" {
		return EngineAnswer{}, fmt.Errorf("PERPLEXITY_API_KEY empty")
	}
	body := perplexityRequest{
		Model:           cfg.Model,
		Messages:        []perplexityMessage{{Role: "user", Content: strings.TrimSpace(question)}},
		ReturnCitations: true,
		Temperature:     0.3,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, perplexityCompletionsURL(cfg.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return EngineAnswer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	res, err := llmHTTPClient.Do(req)
	if err != nil {
		return EngineAnswer{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return EngineAnswer{}, fmt.Errorf("perplexity status %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var parsed perplexityResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return EngineAnswer{}, err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return EngineAnswer{}, fmt.Errorf("perplexity empty answer")
	}
	ans := EngineAnswer{Answer: strings.TrimSpace(parsed.Choices[0].Message.Content)}
	for i, u := range parsed.Citations {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		ans.Sources = append(ans.Sources, model.GeoSource{Position: i + 1, URL: u, Cited: true})
	}
	return ans, nil
}

func perplexityCompletionsURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "https://api.perplexity.ai/chat/completions"
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/chat/completions"
}
