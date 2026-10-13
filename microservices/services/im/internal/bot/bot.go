package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message 是模型的一次对话轮次。
type Message struct {
	Role    string // system | user | assistant
	Content string
}

// Client 生成机器人回复。
type Client interface {
	// Name 返回 provider id，用于日志/事件。
	Name() string
	// Complete 返回助手文本（非流式）。
	Complete(ctx context.Context, system string, history []Message) (string, error)
}

// Config 是面向 OpenAI 兼容提供方的配置（与 ai-service 同一套面）。
type Config struct {
	Enabled bool
	BaseURL string // 例如 https://api.openai.com 或内部代理
	APIKey  string
	Model   string
	Timeout time.Duration
	// SystemPrompt 默认提示词（站点未覆盖时使用）。
	SystemPrompt string
}

func (c Config) WithDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.openai.com"
	}
	c.BaseURL = NormalizeBaseURL(c.BaseURL)
	if c.Model == "" {
		c.Model = "gpt-4o-mini"
	}
	if c.Timeout <= 0 {
		c.Timeout = 45 * time.Second
	}
	if c.SystemPrompt == "" {
		c.SystemPrompt = defaultSystemPrompt
	}
	return c
}

// NormalizeBaseURL 兼容 OpenAI SDK 尾部带 /v1 的惯例
// （我们自行拼接 /v1/chat/completions；若保留会造成 /v1/v1 而 404）。
func NormalizeBaseURL(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	return strings.TrimSuffix(s, "/v1")
}

const defaultSystemPrompt = `你是企业在线客服助手，回答简洁、礼貌、可用中文。
若无法确定答案或用户要求人工/真人/转接，请明确告知可转人工，并建议用户点击「转人工」或回复「转人工」。
不要编造订单、物流、退款等具体业务数据。`

// NewClient 在配置了 API key 时选用 OpenAI 兼容实现，否则使用规则桩。
func NewClient(cfg Config) Client {
	cfg = cfg.WithDefaults()
	if cfg.Enabled && cfg.APIKey != "" {
		return NewOpenAI(cfg)
	}
	return NewStub()
}

// OpenAI 面向 /v1/chat/completions 实现 Client（非流式）。
type OpenAI struct {
	cfg        Config
	httpClient *http.Client
}

func NewOpenAI(cfg Config) *OpenAI {
	cfg = cfg.WithDefaults()
	return &OpenAI{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (o *OpenAI) Name() string { return "openai_compat" }

type openAIReq struct {
	Model    string      `json:"model"`
	Messages []openAIMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *OpenAI) Complete(ctx context.Context, system string, history []Message) (string, error) {
	if system == "" {
		system = o.cfg.SystemPrompt
	}
	msgs := make([]openAIMsg, 0, len(history)+1)
	msgs = append(msgs, openAIMsg{Role: "system", Content: system})
	for _, h := range history {
		role := h.Role
		if role == "bot" {
			role = "assistant"
		}
		if role == "visitor" {
			role = "user"
		}
		if role == "agent" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		msgs = append(msgs, openAIMsg{Role: role, Content: h.Content})
	}
	body, err := json.Marshal(openAIReq{Model: o.cfg.Model, Messages: msgs, Stream: false})
	if err != nil {
		return "", err
	}
	url := o.cfg.BaseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ai http %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out openAIResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("ai error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("empty ai response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// Stub 是离线演示机器人（不调用外部 API）。
type Stub struct{}

func NewStub() *Stub { return &Stub{} }

func (s *Stub) Name() string { return "stub" }

func (s *Stub) Complete(_ context.Context, _ string, history []Message) (string, error) {
	// 最后一条用户消息
	user := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" || history[i].Role == "visitor" {
			user = history[i].Content
			break
		}
	}
	u := strings.ToLower(user)
	switch {
	case strings.Contains(user, "你好") || strings.Contains(u, "hello") || strings.Contains(user, "在吗"):
		return "您好，我是智能客服助手。请问需要咨询什么？如需人工服务，请回复「转人工」或点击转人工按钮。", nil
	case strings.Contains(user, "订单") || strings.Contains(user, "物流") || strings.Contains(user, "快递"):
		return "关于订单/物流，请提供订单号；更复杂的问题建议转人工处理。回复「转人工」即可排队。", nil
	case strings.Contains(user, "退款") || strings.Contains(user, "发票") || strings.Contains(user, "投诉"):
		return "退款、发票、投诉类问题建议由人工客服处理，请回复「转人工」。", nil
	case strings.Contains(user, "工作时间") || strings.Contains(user, "几点"):
		return "人工客服工作时间一般为工作日 9:00–18:00（以实际配置为准）。非工作时间也可先留言，上线后会接入。", nil
	default:
		if strings.TrimSpace(user) == "" {
			return "您好，请问有什么可以帮您？", nil
		}
		return "已收到您的问题。当前为演示机器人（未配置 AI_API_KEY）。您可以继续描述问题，或回复「转人工」接入坐席。", nil
	}
}

// WantsHuman 从访客文本中检测转人工意图。
func WantsHuman(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	if t == "" {
		return false
	}
	keys := []string{
		"转人工", "人工客服", "人工服务", "找人工", "真人", "转接人工",
		"human", "agent", "live agent", "real person",
	}
	for _, k := range keys {
		if strings.Contains(t, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// ExtractText 从消息内容 JSON 或原始文本中提取 text 字段。
func ExtractText(content string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return strings.TrimSpace(content)
	}
	if t, ok := m["text"].(string); ok {
		return strings.TrimSpace(t)
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
