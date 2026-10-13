package ai

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

const defaultOpenAIBaseURL = "https://api.openai.com"

// defaultOpenAIImageModel is used when no image model is configured.
const defaultOpenAIImageModel = "dall-e-3"

// OpenAICompatProvider talks to any OpenAI-compatible API surface
// (/v1/chat/completions with stream=true, /v1/embeddings, and
// /v1/images/generations). Pointing BaseURL at DeepSeek, Qwen, Ollama, or
// another compatible vendor works without code changes.
type OpenAICompatProvider struct {
	baseURL    string
	apiKey     string
	chatModel  string
	embedModel string
	imageModel string
	httpClient *http.Client
}

// NewOpenAICompatProvider builds an OpenAI-compatible provider. An empty
// baseURL defaults to the official OpenAI endpoint; an empty imageModel
// defaults to dall-e-3.
func NewOpenAICompatProvider(baseURL, apiKey, chatModel, embedModel, imageModel string) *OpenAICompatProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// 容忍 OpenAI SDK 惯例的 /v1 结尾（下面自己拼 /v1/...，保留会变成 /v1/v1 打 404）
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	imageModel = strings.TrimSpace(imageModel)
	if imageModel == "" {
		imageModel = defaultOpenAIImageModel
	}
	return &OpenAICompatProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		chatModel:  chatModel,
		embedModel: embedModel,
		imageModel: imageModel,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name identifies the provider implementation.
func (p *OpenAICompatProvider) Name() string { return "openai" }

// openAIStreamOptions 请求流式响应时附带用量。
//
// OpenAI 官方端点**默认不在流里推 usage 帧**，必须显式 include_usage；否则用完成为
// nil、成本恒为 0。已核实本项目实际使用的兼容网关（Sub2API 0.2.4）显式识别该字段，
// 不会因未知字段拒绝请求。
type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIChatRequest struct {
	Model         string               `json:"model"`
	Messages      []openAIChatMessage  `json:"messages"`
	Stream        bool                 `json:"stream"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIStreamChunk struct {
	// 上游在响应头已发出之后出错时，按 OpenAI 契约改发错误帧（{"error":{...}}）。
	// 这种帧会被 JSON 成功解析成 Choices 为空的 chunk；若不显式识别，就会被当成
	// "正常结束 + 空内容"，调用方拿到一个静默的空回答且 err=nil（用量还会记成功）。
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// streamErrorText 给错误帧文本加上界：上游异常输出不能把错误串撑成无界字符串。
func streamErrorText(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 256 {
		return trimmed[:256]
	}
	return trimmed
}

// Chat streams a completion through /v1/chat/completions with stream=true.
func (p *OpenAICompatProvider) Chat(ctx context.Context, msgs []ChatMessage, onDelta func(ChatDelta) error) error {
	payload := openAIChatRequest{
		Model:    p.chatModel,
		Messages: make([]openAIChatMessage, 0, len(msgs)),
		Stream:   true,
		// 不带这个字段，官方端点与部分网关都不会推 usage 帧，用量与成本会静默记成 0。
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	}
	for _, m := range msgs {
		payload.Messages = append(payload.Messages, openAIChatMessage{Role: m.Role, Content: m.Content})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("openai chat request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai chat request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai chat request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return apiError("openai", resp)
	}

	done := false
	var usage *Usage
	// 两个"静默空回答"的前置观察：上游到底有没有回过 SSE 数据帧、有没有真的产出内容。
	sawSSEFrame := false
	emittedContent := false
	finishReason := ""
	err = readSSE(resp.Body, func(ev sseEvent) error {
		if ev.data == "" {
			return nil
		}
		sawSSEFrame = true
		if ev.data == "[DONE]" {
			done = true
			return onDelta(ChatDelta{Done: true, Usage: usage})
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(ev.data), &chunk); err != nil {
			return fmt.Errorf("openai stream chunk decode: %w", err)
		}
		// 错误帧必须先于 delta 处理：它是流内失败，绝不能降级成"空回答 + 成功"。
		// 帧内文本属上游正文，故标记为 UpstreamError，由调用方在外发前脱敏。
		if chunk.Error != nil {
			return newUpstreamError("openai stream error %s: %s",
				streamErrorText(chunk.Error.Type, "unknown"),
				streamErrorText(chunk.Error.Message, "no message"))
		}
		// 提取 usage（OpenAI 在流的最后一帧返回）
		if chunk.Usage != nil {
			usage = &Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}
			if choice.Delta.Content != "" {
				emittedContent = true
				if err := onDelta(ChatDelta{Content: choice.Delta.Content}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 200 但不是事件流：一个数据帧都没有，说明上游没有按 stream=true 回应
	// （部分网关会直接回一个普通 JSON 完成体）。不拦就会被当成"空回答 + 成功"。
	if !sawSSEFrame {
		return fmt.Errorf("openai chat response was not an event stream")
	}
	// 内容被安全策略拦下时上游用 finish_reason=content_filter 收尾且没有任何内容：
	// 这不是"成功的空回答"，必须显式失败。有内容时的既有行为不变。
	if !emittedContent && finishReason == "content_filter" {
		return fmt.Errorf("openai chat response was blocked by content filter")
	}
	if !done {
		return onDelta(ChatDelta{Done: true, Usage: usage})
	}
	return nil
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns embeddings from /v1/embeddings, ordered by input index.
func (p *OpenAICompatProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(openAIEmbedRequest{Model: p.embedModel, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("openai embed request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai embed request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError("openai", resp)
	}

	var decoded openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("openai embed response decode: %w", err)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("openai embed response returned %d vectors for %d inputs", len(decoded.Data), len(texts))
	}

	vectors := make([][]float32, len(texts))
	// 只校验数量与下标范围不够：上游重复同一个 index 时两者都能通过，
	// 结果是有下标没被写入（nil 向量）却 err=nil，nil 向量会被静默写进向量列。
	assigned := make([]bool, len(texts))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("openai embed response index %d out of range", item.Index)
		}
		if assigned[item.Index] {
			return nil, fmt.Errorf("openai embed response repeated index %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("openai embed response index %d has an empty vector", item.Index)
		}
		assigned[item.Index] = true
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	ResponseFormat string `json:"response_format"`
}

type openAIImageResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

// GenerateImage produces images through /v1/images/generations (DALL·E
// compatible). Empty options fall back to a single 1024x1024 image returned
// as a URL.
func (p *OpenAICompatProvider) GenerateImage(ctx context.Context, prompt string, opts ImageOptions) (ImageResult, error) {
	size := strings.TrimSpace(opts.Size)
	if size == "" {
		size = "1024x1024"
	}
	n := opts.N
	if n <= 0 {
		n = 1
	}
	format := strings.TrimSpace(opts.ResponseFormat)
	if format == "" {
		format = "url"
	}

	body, err := json.Marshal(openAIImageRequest{
		Model:          p.imageModel,
		Prompt:         prompt,
		N:              n,
		Size:           size,
		ResponseFormat: format,
	})
	if err != nil {
		return ImageResult{}, fmt.Errorf("openai image request encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return ImageResult{}, fmt.Errorf("openai image request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ImageResult{}, fmt.Errorf("openai image request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ImageResult{}, apiError("openai", resp)
	}

	var decoded openAIImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ImageResult{}, fmt.Errorf("openai image response decode: %w", err)
	}

	var result ImageResult
	for _, item := range decoded.Data {
		if item.URL != "" {
			result.URLs = append(result.URLs, item.URL)
		}
		if item.B64JSON != "" {
			result.B64 = append(result.B64, item.B64JSON)
		}
	}
	return result, nil
}

// apiError reads a bounded error body and folds it into a descriptive error.
func apiError(provider string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return newUpstreamError("%s api status %d: %s", provider, resp.StatusCode, message)
}
