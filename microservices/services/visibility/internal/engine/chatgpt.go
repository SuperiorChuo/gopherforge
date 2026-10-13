package engine

// ChatGPT (Responses API + web_search) engine adapter — a REAL generative
// engine: the model runs its own web search and returns structured citations
// (url_citation annotations). This is what "真正的 GEO" measures: not our own
// bing-grounded simulation, but what a real grounded ChatGPT answers and cites.
//
// Responses API contract: POST {base}/responses with
// tools:[{type:"web_search"}] returns output[] containing a web_search_call
// (action.queries = the actual queries it ran) and a message whose
// content[0].annotations[] hold url_citation {url,title} entries.

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

type chatgptEngine struct{}

func (chatgptEngine) Name() string { return "chatgpt" }

type chatgptRequest struct {
	Model string        `json:"model"`
	Input string        `json:"input"`
	Tools []chatgptTool `json:"tools"`
}

type chatgptTool struct {
	Type string `json:"type"`
}

type chatgptResponse struct {
	ID     string          `json:"id"`
	Error  *chatgptError   `json:"error"`
	Output []chatgptOutput `json:"output"`
}

type chatgptError struct {
	Message string `json:"message"`
}

type chatgptOutput struct {
	Type    string           `json:"type"`
	Action  *chatgptAction   `json:"action"`
	Content []chatgptContent `json:"content"`
}

type chatgptAction struct {
	Type    string   `json:"type"`
	Queries []string `json:"queries"`
}

type chatgptContent struct {
	Type        string              `json:"type"`
	Text        string              `json:"text"`
	Annotations []chatgptAnnotation `json:"annotations"`
}

type chatgptAnnotation struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Ask 调用真实 ChatGPT 联网搜索：自备检索、自备引用，忽略 injected。
// 用原始 question（真实引擎自己处理检索词，不需要 BuildSearchQuery）。
func (chatgptEngine) Ask(ctx context.Context, cfg Config, question string, _ []model.GeoSource) (EngineAnswer, error) {
	if cfg.APIKey == "" {
		return EngineAnswer{}, fmt.Errorf("AI_API_KEY empty")
	}
	body := chatgptRequest{
		Model: cfg.Model,
		Input: strings.TrimSpace(question),
		Tools: []chatgptTool{{Type: "web_search"}},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptResponsesURL(cfg.BaseURL), bytes.NewReader(raw))
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
		return EngineAnswer{}, fmt.Errorf("chatgpt status %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var parsed chatgptResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return EngineAnswer{}, err
	}
	if parsed.Error != nil {
		return EngineAnswer{}, fmt.Errorf("chatgpt error: %s", parsed.Error.Message)
	}

	var ans EngineAnswer
	for _, it := range parsed.Output {
		switch it.Type {
		case "web_search_call":
			if it.Action != nil {
				ans.Queries = append(ans.Queries, it.Action.Queries...)
			}
		case "message":
			for _, c := range it.Content {
				if c.Type != "output_text" {
					continue
				}
				if ans.Answer == "" {
					ans.Answer = strings.TrimSpace(c.Text)
				}
				for i, a := range c.Annotations {
					if a.Type != "url_citation" || strings.TrimSpace(a.URL) == "" {
						continue
					}
					ans.Sources = append(ans.Sources, model.GeoSource{
						Position: i + 1,
						Title:    a.Title,
						URL:      a.URL,
						Cited:    true, // 引擎结构化引用即视为被引用
					})
				}
			}
		}
	}
	if ans.Answer == "" {
		return EngineAnswer{}, fmt.Errorf("chatgpt empty answer output")
	}
	return ans, nil
}

// chatgptResponsesURL 组装 Responses 端点：base 以 /v1 结尾（中转惯例）直接拼
// /responses；否则拼 /v1/responses。
func chatgptResponsesURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "https://api.openai.com/v1/responses"
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/responses"
	}
	return base + "/v1/responses"
}
