package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

// 三期：manager 下发通道。控制面把任务意图经签名 desired-state 下发，manager
// 在这里把 description 推给本地 Hermes 的 Gateway API server（loopback，
// 复用现有 `gateway run` 启动，.env 里带 API_SERVER_KEY 时上游自动启用）。

const (
	// maxPushTaskResultLen 与 server 侧 result 长度上限一致，防止回传超限。
	maxPushTaskResultLen = 4000
	// apiServerHTTPTimeout 是单次任务执行的上限。同步 chat completion 长任务
	// 可能跑几分钟；超时按失败处理（下轮 sync 重试）。
	apiServerHTTPTimeout = 5 * time.Minute
)

// PushTask 把任务意图推给本地 Hermes API server 并返回执行结果。
// 路径 /p/<profile>/v1/chat/completions（上游 per-profile 前缀）；Bearer 用
// manager 自持的 apiServerKey。返回 choices[0].message.content（截断到
// maxPushTaskResultLen）。
func (b *Base) PushTask(_ context.Context, task protocol.DesiredStateTask, apiServerKey string) (string, error) {
	host, port := b.apiServerEndpoint()
	profile := b.profileProfile()
	url := fmt.Sprintf("http://%s:%s/p/%s/v1/chat/completions", host, port, profile)

	body := map[string]any{
		"model":    task.Model,
		"stream":   false,
		"messages": []map[string]string{{"role": "user", "content": task.Description}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal task push: %w", err)
	}

	client := &http.Client{Timeout: apiServerHTTPTimeout}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("build task push request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiServerKey)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("push task to api server: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20)) // 1 MiB 上限
	if err != nil {
		return "", fmt.Errorf("read task push response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("task push rejected by api server: status=%d body=%s", response.StatusCode, truncate(string(responseBody), 200))
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return "", fmt.Errorf("parse task push completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("task push returned no completion choices")
	}
	result := strings.TrimSpace(completion.Choices[0].Message.Content)
	if len(result) > maxPushTaskResultLen {
		result = result[:maxPushTaskResultLen]
	}
	if result == "" {
		return "", errors.New("task push returned an empty result")
	}
	return result, nil
}

// apiServerEndpoint 读受管 profile .env 的 API_SERVER_HOST/PORT，缺省
// loopback 127.0.0.1:8642。manager 写这些键时已钉死 loopback。
func (b *Base) apiServerEndpoint() (host, port string) {
	host, port = apiServerHost, apiServerPort
	envPath := filepath.Join(b.profileDir(), ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return host, port
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case envAPIServerHost:
			if value != "" {
				host = value
			}
		case envAPIServerPort:
			if value != "" {
				port = value
			}
		}
	}
	return host, port
}

// profileProfile 返回受管 orchestrator profile 名。上游 per-profile API 路径
// 需要 profile 名；manager 只渲染 company profile（与 home/profiles/company 对齐）。
func (b *Base) profileProfile() string {
	return orchestratorProfileName
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
