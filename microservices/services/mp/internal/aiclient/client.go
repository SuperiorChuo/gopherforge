// Package aiclient 调用 ai-service 生成文章/图片（M5）。走 compose 网络内容器名
// 直连（http://go-admin-kit-ai:8087），透传请求方的 X-Auth-* 头以通过 ai 侧鉴权。
package aiclient

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

type Client struct {
	baseURL string
	http    *http.Client
}

// New 构造 aiclient。baseURL 如 http://go-admin-kit-ai:8087（不带尾 /）。
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 3 * time.Minute}, // 生成/生图可能慢
	}
}

// Auth 透传给 ai-service 的鉴权头（来自网关注入或调用方 JWT）。
type Auth struct {
	UserID   string
	Username string
	TenantID string
	Bearer   string // 可选，X-Auth 头缺失时用
}

func (c *Client) do(ctx context.Context, path string, body any, auth Auth, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth.UserID != "" {
		req.Header.Set("X-Auth-User-ID", auth.UserID)
		req.Header.Set("X-Auth-Username", auth.Username)
		req.Header.Set("X-Auth-Tenant-ID", auth.TenantID)
	}
	if auth.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+auth.Bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ai-service %s status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	// ai-service 统一信封 {code,message,data}
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("ai-service %s decode envelope: %w", path, err)
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

// ComposeArticle 调 /ai/compose kind=wechat_article，返回原始 content（JSON 字符串，
// 形如 {"title","digest","content"}；解析交调用方）。
func (c *Client) ComposeArticle(ctx context.Context, prompt string, auth Auth) (string, error) {
	var out struct {
		Content string `json:"content"`
	}
	err := c.do(ctx, "/api/v1/ai/compose", map[string]any{
		"kind": "wechat_article", "prompt": prompt,
	}, auth, &out)
	return out.Content, err
}

// ComposeForPlatform 按目标社媒平台风格生成内容（social_post kind）。platform 空则
// 走通用图文。返回 JSON 字符串 {title,digest,content}。
func (c *Client) ComposeForPlatform(ctx context.Context, prompt, platform string, auth Auth) (string, error) {
	var out struct {
		Content string `json:"content"`
	}
	err := c.do(ctx, "/api/v1/ai/compose", map[string]any{
		"kind": "social_post", "prompt": prompt, "platform": platform,
	}, auth, &out)
	return out.Content, err
}

// GenerateImage 调 /ai/image，返回第一张图的 url（可能是 http 或 data:image URL）。
func (c *Client) GenerateImage(ctx context.Context, prompt, size string, auth Auth) (string, error) {
	var out struct {
		URLs []string `json:"urls"`
		B64  []string `json:"b64"`
	}
	if size == "" {
		size = "1024x1024"
	}
	if err := c.do(ctx, "/api/v1/ai/image", map[string]any{
		"prompt": prompt, "size": size,
	}, auth, &out); err != nil {
		return "", err
	}
	if len(out.URLs) > 0 {
		return out.URLs[0], nil
	}
	if len(out.B64) > 0 {
		return "data:image/png;base64," + out.B64[0], nil
	}
	return "", fmt.Errorf("ai image: empty result")
}
