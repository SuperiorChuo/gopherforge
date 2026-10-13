package engine

// Gemini (Google Search grounding) engine adapter — a REAL generative engine.
// REST generateContent with tools:[{google_search:{}}]; the response carries
// groundingMetadata.groundingChunks[] (uri+title) as the real citations.
// 如网络需要代理，可配置可达渠道（配置见 runner.SetExtraEngines）。
//
// 注意：部分模型（如 gemini-3-flash 早期版本）不返回 groundingMetadata；配置
// 默认 gemini-2.5-flash（实测稳定返回引用）。groundingChunks 为空时视为无引用。

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

type geminiEngine struct{}

func (geminiEngine) Name() string { return "gemini" }

type geminiRequest struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
	Tools []struct {
		GoogleSearch map[string]any `json:"google_search"`
	} `json:"tools"`
}

type geminiResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		GroundingMetadata struct {
			GroundingChunks []struct {
				Web struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
}

// Ask 调用 Gemini generateContent + googleSearch；groundingChunks 映射为引用。
// injected 忽略。
func (geminiEngine) Ask(ctx context.Context, cfg Config, question string, _ []model.GeoSource) (EngineAnswer, error) {
	if cfg.APIKey == "" {
		return EngineAnswer{}, fmt.Errorf("GEMINI_API_KEY empty")
	}
	body := geminiRequest{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{{Parts: []struct {
			Text string `json:"text"`
		}{{Text: strings.TrimSpace(question)}}}},
		Tools: []struct {
			GoogleSearch map[string]any `json:"google_search"`
		}{{GoogleSearch: map[string]any{}}},
	}
	raw, _ := json.Marshal(body)
	url := geminiGenerateURL(cfg.BaseURL, cfg.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return EngineAnswer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Gemini 用 x-goog-api-key（兼容 query 参数形式也支持）。
	req.Header.Set("x-goog-api-key", cfg.APIKey)
	res, err := llmHTTPClient.Do(req)
	if err != nil {
		return EngineAnswer{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return EngineAnswer{}, fmt.Errorf("gemini status %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var parsed geminiResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return EngineAnswer{}, err
	}
	if parsed.Error != nil {
		return EngineAnswer{}, fmt.Errorf("gemini error: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return EngineAnswer{}, fmt.Errorf("gemini empty candidates")
	}
	cand := parsed.Candidates[0]
	if len(cand.Content.Parts) == 0 || strings.TrimSpace(cand.Content.Parts[0].Text) == "" {
		return EngineAnswer{}, fmt.Errorf("gemini empty answer")
	}
	ans := EngineAnswer{Answer: strings.TrimSpace(cand.Content.Parts[0].Text)}
	for i, c := range cand.GroundingMetadata.GroundingChunks {
		if strings.TrimSpace(c.Web.URI) == "" {
			continue
		}
		ans.Sources = append(ans.Sources, model.GeoSource{
			Position: i + 1,
			Title:    c.Web.Title,
			URL:      c.Web.URI,
			Cited:    true,
		})
	}
	return ans, nil
}

func geminiGenerateURL(base, modelName string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	return base + "/v1beta/models/" + modelName + ":generateContent"
}
