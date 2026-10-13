// Package fsclient calls the freeswitch-cc control-api (X-CC-Token auth).
package fsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// Get proxies a GET and decodes the JSON body into a generic map.
func (c *Client) Get(ctx context.Context, path string) (map[string]any, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post proxies a POST with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any) (map[string]any, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

// Put proxies a PUT with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body any) (map[string]any, error) {
	return c.do(ctx, http.MethodPut, path, body)
}

// Delete proxies a DELETE.
func (c *Client) Delete(ctx context.Context, path string) (map[string]any, error) {
	return c.do(ctx, http.MethodDelete, path, nil)
}

// PostMultipart 透传 multipart 表单（MOH 上传）。contentType 是入站请求原始
// Content-Type（含 boundary），body 直接透传不落盘。
func (c *Client) PostMultipart(ctx context.Context, path, contentType string, body io.Reader) (map[string]any, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("fs control-api is not configured (CC_FS_API_BASE empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-CC-Token", c.token)
	uploadClient := &http.Client{Timeout: 2 * time.Minute}
	resp, err := uploadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("control-api status %d: decode: %w", resp.StatusCode, err)
	}
	if resp.StatusCode >= 300 {
		msg, _ := out["error"].(string)
		return out, fmt.Errorf("control-api status %d: %s", resp.StatusCode, msg)
	}
	return out, nil
}

// Stream issues a GET and returns the raw response for streaming bodies
// (录音 wav 透传)。Caller must Close the body. 非 2xx 也原样返回，由调用方转发状态码。
func (c *Client) Stream(ctx context.Context, path string) (*http.Response, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("fs control-api is not configured (CC_FS_API_BASE empty)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-CC-Token", c.token)
	// 录音文件可能较大，流式请求不复用带 8s 超时的 JSON client
	streamClient := &http.Client{Timeout: 2 * time.Minute}
	return streamClient.Do(req)
}

func (c *Client) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("fs control-api is not configured (CC_FS_API_BASE empty)")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-CC-Token", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("control-api status %d: decode: %w", resp.StatusCode, err)
	}
	if resp.StatusCode >= 300 {
		msg, _ := out["error"].(string)
		return out, fmt.Errorf("control-api status %d: %s", resp.StatusCode, msg)
	}
	return out, nil
}
