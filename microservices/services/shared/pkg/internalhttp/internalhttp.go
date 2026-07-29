// Package internalhttp 是服务间内网直连调用的公共传输层。
//
// 各服务此前各写一份 notifyclient/bpmclient/crmclient/kbclient/aiclient/fsclient
// （11 份，逐字节相同的居多），共同点是：base URL 归一化、X-Internal-Token 头、
// {code,message,data} 信封解包、LimitReader 截断。差异只有超时与 payload 类型。
//
// 复制的代价不只是行数——11 份实现里没有任何一份带连接复用调优或重试：
//   - 裸 &http.Client{} 用的是 DefaultTransport，MaxIdleConnsPerHost 只有 2，
//     并发一上来就在反复建 TCP 连接（内网调用尤其吃亏）。
//   - 任何一次瞬时抖动（对端滚动更新、网络抖一下）都直接冒泡成业务失败，
//     而这些调用大多是幂等的读或可重投的通知。
//
// 沉到这里之后，上面两件事改一次就是 11 处受益。
package internalhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultTimeout 单次请求超时（不含重试）。
	DefaultTimeout = 8 * time.Second
	// DefaultMaxRetries 重试次数（不含首次）。0 = 不重试。
	DefaultMaxRetries = 2
	// DefaultRetryBackoff 首次重试前的等待，其后指数增长。
	DefaultRetryBackoff = 100 * time.Millisecond
	// maxResponseBytes 响应体读取上限，防止对端异常时把内存吃穿。
	maxResponseBytes = 1 << 20
)

// sharedTransport 让所有内部客户端共用连接池。DefaultTransport 的
// MaxIdleConnsPerHost=2 对"少数几个固定对端、高并发"的内网调用是最差配置：
// 空闲连接刚建好就被挤掉，下一个请求又要重新握手。
var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// Options 配置项，零值即为可用的默认配置。
type Options struct {
	// Timeout 单次请求超时，<=0 用 DefaultTimeout。
	Timeout time.Duration
	// MaxRetries 重试次数，<0 视为 0。零值取 DefaultMaxRetries；
	// 明确不要重试请用 NoRetry。
	MaxRetries int
	// RetryBackoff 首次重试等待，<=0 用 DefaultRetryBackoff。
	RetryBackoff time.Duration
	// NoRetry 显式关闭重试（区别于 MaxRetries 零值取默认）。
	// 非幂等且不可重投的调用应设置它。
	NoRetry bool
	// TokenHeader 鉴权头名，空则用 X-Internal-Token。
	TokenHeader string
}

// Client 一个内网服务端点的客户端。零 base 或零 token 时 Enabled() 为 false，
// 调用方据此静默跳过（沿用各服务原有语义：通知发不出去不该阻断主流程）。
type Client struct {
	base        string
	token       string
	tokenHeader string
	maxRetries  int
	backoff     time.Duration
	http        *http.Client
}

// New 构造客户端。base 尾部斜杠与首尾空白会被归一化。
func New(base, token string, opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	retries := opts.MaxRetries
	switch {
	case opts.NoRetry:
		retries = 0
	case retries == 0:
		retries = DefaultMaxRetries
	case retries < 0:
		retries = 0
	}
	backoff := opts.RetryBackoff
	if backoff <= 0 {
		backoff = DefaultRetryBackoff
	}
	header := strings.TrimSpace(opts.TokenHeader)
	if header == "" {
		header = "X-Internal-Token"
	}
	return &Client{
		base:        strings.TrimRight(strings.TrimSpace(base), "/"),
		token:       strings.TrimSpace(token),
		tokenHeader: header,
		maxRetries:  retries,
		backoff:     backoff,
		http:        &http.Client{Timeout: timeout, Transport: sharedTransport},
	}
}

// Enabled 端点是否已配置。未配置时各 Do* 方法直接返回 nil error 且不写出参，
// 与各服务原实现的"未配置即静默跳过"一致。
func (c *Client) Enabled() bool {
	return c != nil && c.base != "" && c.token != ""
}

// Base 返回归一化后的基地址（排障与日志用）。
func (c *Client) Base() string {
	if c == nil {
		return ""
	}
	return c.base
}

// envelope 是本项目统一的响应信封。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// HTTPError 承载非 2xx 响应，调用方可据状态码分流。
type HTTPError struct {
	StatusCode int
	Body       string
	URL        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("internal call %s: HTTP %d: %s", e.URL, e.StatusCode, e.Body)
}

// APIError 承载信封里的业务错误码（HTTP 200 但 code 非 0/200）。
type APIError struct {
	Code    int
	Message string
	URL     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("internal call %s: code %d: %s", e.URL, e.Code, e.Message)
}

// DoJSON 发起一次 JSON 调用：in 序列化为请求体（nil 表示无体），响应信封的
// data 反序列化进 out（nil 表示不关心）。端点未配置时直接返回 nil。
func (c *Client) DoJSON(ctx context.Context, method, path string, in, out any) error {
	if !c.Enabled() {
		return nil
	}
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	url := c.base + path

	var lastErr error
	for attempt := 0; ; attempt++ {
		raw, err := c.attempt(ctx, method, url, body)
		if err == nil {
			return decodeEnvelope(raw, out, url)
		}
		lastErr = err
		// ctx 取消/超时不该被重试掩盖：调用方已经不等了
		if ctx.Err() != nil {
			return lastErr
		}
		if attempt >= c.maxRetries || !retryable(err) {
			return lastErr
		}
		// 指数退避；等待期间 ctx 取消则立刻返回
		wait := c.backoff << attempt
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}

// attempt 执行单次请求并读回响应体。
func (c *Client) attempt(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		// 每次重试都要新的 reader：Body 是一次性的
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(c.tokenHeader, c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 300 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(raw)),
			URL:        url,
		}
	}
	return raw, nil
}

// decodeEnvelope 解信封并把 data 写进 out。
func decodeEnvelope(raw []byte, out any, url string) error {
	if len(raw) == 0 {
		return nil
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("internal call %s: decode envelope: %w", url, err)
	}
	if env.Code != 0 && env.Code != 200 {
		return &APIError{Code: env.Code, Message: env.Message, URL: url}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("internal call %s: decode data: %w", url, err)
		}
	}
	return nil
}

// retryable 判断错误值不值得重试。
//
// 只重试"重来一次可能就好了"的情况：网络层错误，以及 5xx/429。
// 4xx（除 429）是请求本身的问题，重试只是把同一个错误再发一遍。
func retryable(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}
	// 传输层错误（连接被拒、超时、对端在滚动更新中断开等）
	return true
}
