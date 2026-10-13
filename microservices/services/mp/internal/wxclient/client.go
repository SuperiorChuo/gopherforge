package wxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// Client 是微信 API 通用请求层：拿 token、拼 access_token query、发请求、解析
// {errcode,errmsg}，并在 token 失效时自动失效重刷重试一次。
type Client struct {
	tokens  *TokenManager
	http    *http.Client
	baseURL string // 默认 defaultWxBaseURL，测试可注入
}

// ClientOption 定制 Client（主要给测试注入 baseURL / httpClient）。
type ClientOption func(*Client)

// WithBaseURL 覆盖微信 API 根地址（测试指向 httptest.Server）。
func WithBaseURL(base string) ClientOption {
	return func(c *Client) { c.baseURL = base }
}

// WithHTTPClient 覆盖 http.Client（上传大文件时可给更长超时）。
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) { c.http = h }
}

// NewClient 构造通用请求层。tokens 为 access_token 中控。
func NewClient(tokens *TokenManager, opts ...ClientOption) *Client {
	c := &Client{
		tokens:  tokens,
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: defaultWxBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// buildURL 把 access_token 拼进 path 的 query。
func (c *Client) buildURL(path, token string) string {
	sep := "?"
	if u, err := url.Parse(path); err == nil && u.RawQuery != "" {
		sep = "&"
	}
	return c.baseURL + path + sep + "access_token=" + url.QueryEscape(token)
}

// doJSON 发 JSON 请求并把响应解进 out。body 为 nil 时不带请求体。命中 token
// 失效（40001/42001）会 Invalidate + 重刷重试一次（最多一次，防死循环）。
func (c *Client) doJSON(ctx context.Context, account *store.MpAccount, method, path string, body, out any) error {
	return c.withRetry(ctx, account, func(token string) error {
		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path, token), reader)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return c.send(req, out)
	})
}

// doUpload 发 multipart/form-data 上传（素材上传）。field 为文件字段名（微信固定
// "media"），extraFields 为附加表单字段（如永久图文素材的 description）。同样注入
// token、同样失效重试一次。
func (c *Client) doUpload(ctx context.Context, account *store.MpAccount, path, field, filename string, r io.Reader, extraFields map[string]string, out any) error {
	// 上传的 body 只能读一遍：先缓冲到内存，失效重试时可重放。素材通常 ≤10MB，
	// 缓冲成本可接受，也避免调用方传不可重读的 Reader 时重试失败。
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return fmt.Errorf("upload: read body: %w", err)
	}
	payload := buf.Bytes()

	return c.withRetry(ctx, account, func(token string) error {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, err := mw.CreateFormFile(field, filename)
		if err != nil {
			return err
		}
		if _, err := fw.Write(payload); err != nil {
			return err
		}
		for k, v := range extraFields {
			if err := mw.WriteField(k, v); err != nil {
				return err
			}
		}
		if err := mw.Close(); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL(path, token), &body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return c.send(req, out)
	})
}

// withRetry 取 token、执行 do，命中 token 失效则 Invalidate 后重试一次。
func (c *Client) withRetry(ctx context.Context, account *store.MpAccount, do func(token string) error) error {
	token, err := c.tokens.GetToken(ctx, account)
	if err != nil {
		return err
	}
	err = do(token)
	if err != nil && asWxTokenExpired(err) {
		// token 服务端已失效：清缓存后强制重刷一次。
		if ierr := c.tokens.Invalidate(ctx, account.AppID); ierr != nil {
			return err // 保留原始 wx 错误更有信息量
		}
		token, gerr := c.tokens.GetToken(ctx, account)
		if gerr != nil {
			return gerr
		}
		return do(token)
	}
	return err
}

// send 执行请求并把响应解进 out，先探 {errcode,errmsg}，errcode!=0 转 WxError。
// out 为 nil 时只校验 errcode 不解析业务字段。
func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return safeRequestError("wx api", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("wx api: read body (status %d): %w", resp.StatusCode, err)
	}

	// 先探公共错误头：上传成功也可能返回 {errcode:0,...}，失败返回非 0。
	var common wxCommonResp
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &common); err != nil {
			if !successfulHTTPStatus(resp.StatusCode) {
				return unexpectedHTTPStatus("wx api", resp.StatusCode)
			}
			return fmt.Errorf("wx api: decode errhead (status %d): %w", resp.StatusCode, err)
		}
	}
	if err := common.asWxError(); err != nil {
		return err
	}
	if !successfulHTTPStatus(resp.StatusCode) {
		return unexpectedHTTPStatus("wx api", resp.StatusCode)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("wx api: decode body (status %d): %w", resp.StatusCode, err)
		}
	}
	return nil
}
