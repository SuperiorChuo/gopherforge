package internalhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type reqBody struct {
	Name string `json:"name"`
}

type respData struct {
	Created int `json:"created"`
}

func TestDoJSONSendsTokenAndDecodesEnvelope(t *testing.T) {
	var gotToken, gotCT, gotPath, gotMethod string
	var gotBody reqBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok", "data": map[string]int{"created": 3},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", Options{})
	var out respData
	if err := c.DoJSON(context.Background(), http.MethodPost, "/api/v1/x/send",
		reqBody{Name: "a"}, &out); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if gotToken != "secret" {
		t.Errorf("token 头 = %q, want secret", gotToken)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if gotPath != "/api/v1/x/send" || gotMethod != http.MethodPost {
		t.Errorf("请求行 = %s %s", gotMethod, gotPath)
	}
	if gotBody.Name != "a" {
		t.Errorf("请求体 = %+v", gotBody)
	}
	if out.Created != 3 {
		t.Errorf("data 解包 = %+v, want created 3", out)
	}
}

// base 尾斜杠与首尾空白必须归一化——11 份原实现都做了这件事。
func TestNewNormalizesBase(t *testing.T) {
	for _, in := range []string{"http://h:1/", "  http://h:1  ", "http://h:1///"} {
		if got := New(in, "t", Options{}).Base(); got != "http://h:1" {
			t.Errorf("New(%q).Base() = %q", in, got)
		}
	}
}

// 未配置端点时静默跳过：通知发不出去不该阻断业务主流程。
func TestDisabledClientIsNoOp(t *testing.T) {
	for _, c := range []*Client{
		New("", "token", Options{}),
		New("http://h:1", "", Options{}),
		nil,
	} {
		if c.Enabled() {
			t.Fatal("Enabled() 应为 false")
		}
		out := respData{Created: 42}
		if err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, &out); err != nil {
			t.Fatalf("未配置时应返回 nil，实得 %v", err)
		}
		if out.Created != 42 {
			t.Error("未配置时不应写出参")
		}
	}
}

func TestRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]int{"created": 1}})
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{RetryBackoff: time.Millisecond})
	var out respData
	if err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, &out); err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("调用次数 = %d, want 3（首次 + 2 次重试）", n)
	}
	if out.Created != 1 {
		t.Errorf("out = %+v", out)
	}
}

// 4xx 是请求本身的问题，重试只是把同一个错误再发一遍。
func TestDoesNotRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad input"))
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{RetryBackoff: time.Millisecond})
	err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, nil)
	if err == nil {
		t.Fatal("4xx 应返回错误")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v, want HTTPError 400", err)
	}
	if httpErr.Body != "bad input" {
		t.Errorf("错误体 = %q", httpErr.Body)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("调用次数 = %d, want 1（4xx 不重试）", n)
	}
}

// 429 例外：限流是"稍后再来"，值得退避重试。
func TestRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{RetryBackoff: time.Millisecond})
	if err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, nil); err != nil {
		t.Fatalf("429 重试后应成功: %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("调用次数 = %d, want 2", n)
	}
}

func TestNoRetryOption(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{NoRetry: true, RetryBackoff: time.Millisecond})
	if err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, nil); err == nil {
		t.Fatal("应返回错误")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("调用次数 = %d, want 1（NoRetry）", n)
	}
}

// 业务错误码（HTTP 200 但 code 非 0/200）要能被识别，且不该触发重试。
func TestAPIErrorFromEnvelopeCode(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 40001, "message": "模板不存在"})
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{RetryBackoff: time.Millisecond})
	err := c.DoJSON(context.Background(), http.MethodPost, "/x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want APIError", err)
	}
	if apiErr.Code != 40001 || apiErr.Message != "模板不存在" {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("业务错误码不该重试，调用次数 = %d", n)
	}
}

// ctx 取消后调用方已经不等了，重试没有意义。
func TestCanceledContextStopsRetrying(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := New(srv.URL, "t", Options{RetryBackoff: 50 * time.Millisecond})
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if err := c.DoJSON(ctx, http.MethodPost, "/x", nil, nil); err == nil {
		t.Fatal("应返回错误")
	}
	if n := calls.Load(); n > 2 {
		t.Errorf("ctx 取消后仍在重试，调用次数 = %d", n)
	}
}

// 重试要能重发请求体（Body 是一次性的，每次得给新 reader）。
func TestRetryResendsRequestBody(t *testing.T) {
	var calls atomic.Int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b reqBody
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b.Name)
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer srv.Close()

	c := New(srv.URL, "t", Options{RetryBackoff: time.Millisecond})
	if err := c.DoJSON(context.Background(), http.MethodPost, "/x", reqBody{Name: "payload"}, nil); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Errorf("重试未重发请求体: %v", bodies)
	}
}

func TestCustomTokenHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-FS-Token")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", Options{TokenHeader: "X-FS-Token"})
	if err := c.DoJSON(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got != "tok" {
		t.Errorf("自定义 token 头 = %q", got)
	}
}
