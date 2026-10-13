package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-kit/services/visibility/internal/model"
	"github.com/go-admin-kit/services/visibility/internal/serp"
)

type fakeCollector struct {
	name  string
	items []serp.Item
	err   error
}

func (f *fakeCollector) Name() string { return f.name }
func (f *fakeCollector) Fetch(context.Context, string, int) ([]serp.Item, error) {
	return f.items, f.err
}

// fakeLLMServer 按请求路由：含【检索来源】→ 带 [1] 引用的接地回答；
// 含【自有品牌】→ 提及分析 JSON；其余 → 无来源回答。
func fakeLLMServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		user := ""
		if len(body.Messages) > 1 {
			user = body.Messages[1].Content
		}
		switch {
		case strings.Contains(user, "【检索来源】"):
			w.Write([]byte(`{"choices":[{"message":{"content":"推荐使用 MyBrand，详见 [1]。"}}]}`))
		case strings.Contains(user, "【自有品牌】"):
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"mention_brand\":true,\"matched_brands\":[\"MyBrand\"],\"recommended\":true,\"sentiment\":\"positive\",\"list_position\":1}"}}]}`))
		default:
			// 无来源回答里带上编造的 URL：降级路径的 has_link 不得因此为 true
			w.Write([]byte(`{"choices":[{"message":{"content":"无来源回答，详见 https://hallucinated.example/doc"}}]}`))
		}
	}))
}

type resultCapture struct {
	mu  sync.Mutex
	res *model.GeoResult
}

func (c *resultCapture) set(res *model.GeoResult) { c.mu.Lock(); c.res = res; c.mu.Unlock() }
func (c *resultCapture) get() *model.GeoResult    { c.mu.Lock(); defer c.mu.Unlock(); return c.res }

func TestGeoRunGroundedWithCitations(t *testing.T) {
	srv := fakeLLMServer(t)
	defer srv.Close()

	fs := newFakeStore()
	var cap resultCapture
	fs.onCreateResult = func(res *model.GeoResult) error {
		cap.set(res)
		return nil
	}

	r := New(fs, AIConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"},
		[]serp.Collector{&fakeCollector{name: "bing", items: []serp.Item{
			{Position: 1, URL: "https://example.com/doc", Title: "Example"},
		}}}, 5, nil)
	r.fetchText = func(context.Context, string, int) (string, error) {
		return "MyBrand 是开源后台脚手架，支持 RBAC。", nil
	}
	defer r.Shutdown()

	run, _, err := r.SubmitGeoRun(1, 1, "llm", "selfhost", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return cap.get() != nil })
	if st := fs.runStatus(run.ID); st != "done" {
		t.Fatalf("run status = %s", st)
	}
	got := cap.get()
	if got == nil {
		t.Fatal("no result captured")
	}
	if got.Engine != "selfhost" {
		t.Fatalf("engine = %q, want selfhost", got.Engine)
	}
	if !got.HasLink {
		t.Fatalf("grounded + cited answer should have has_link=true: %+v", got)
	}
	if !strings.Contains(got.Answer, "MyBrand") {
		t.Fatalf("answer = %q", got.Answer)
	}
	var sources []model.GeoSource
	if err := json.Unmarshal([]byte(got.SourcesJSON), &sources); err != nil {
		t.Fatalf("bad sources json: %v", err)
	}
	if len(sources) != 1 || sources[0].URL != "https://example.com/doc" || !sources[0].Cited {
		t.Fatalf("sources = %+v", sources)
	}
}

func TestGeoRunGroundingFallback(t *testing.T) {
	srv := fakeLLMServer(t)
	defer srv.Close()

	fs := newFakeStore()
	var cap resultCapture
	fs.onCreateResult = func(res *model.GeoResult) error {
		cap.set(res)
		return nil
	}

	// 采集器报错（被反爬/拦截）→ 检索失败 → 降级为无来源回答
	r := New(fs, AIConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"},
		[]serp.Collector{&fakeCollector{name: "bing", err: errors.New("blocked")}}, 5, nil)
	defer r.Shutdown()

	run, _, err := r.SubmitGeoRun(1, 1, "llm", "selfhost", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return cap.get() != nil })
	if st := fs.runStatus(run.ID); st != "done" {
		t.Fatalf("run status = %s", st)
	}
	got := cap.get()
	if got == nil {
		t.Fatal("no result captured")
	}
	if got.Engine != "selfhost" {
		t.Fatalf("engine = %q, want selfhost", got.Engine)
	}
	if got.SourcesJSON != "" {
		t.Fatalf("fallback must have no sources, got %q", got.SourcesJSON)
	}
	// 回答里即便带了 URL，降级行也不能计 has_link——那是模型编造的链接
	if !strings.Contains(got.Answer, "https://hallucinated.example") {
		t.Fatalf("answer should contain the hallucinated url to exercise the guard: %q", got.Answer)
	}
	if got.HasLink {
		t.Fatalf("ungrounded fallback must have has_link=false despite url in answer: %+v", got)
	}
}

func TestIsJunkSource(t *testing.T) {
	junk := []string{
		"https://www.hanyuguoxue.com/zidian/zi-24819",
		"http://iciba.com/word?w=simple",
		"https://www.iqiyi.com/v_19rr73ibuw.html",
		"https://zdic.net/hans/%E6%83%B3",
		"https://www.youtube.com/watch?v=x",
		"https://mc.163.com/",
	}
	ok := []string{
		"https://golangstar.cn/go_series/go_framework/gin.html",
		"https://www.runoob.com/go/go-tutorial.html",
		"https://golang.google.cn/",
		"https://github.com/SuperiorChuo/gopherforge",
	}
	for _, u := range junk {
		if !isJunkSource(u) {
			t.Errorf("expected junk: %s", u)
		}
	}
	for _, u := range ok {
		if isJunkSource(u) {
			t.Errorf("expected not junk: %s", u)
		}
	}
}

func TestGeoRunChatGPTEngine(t *testing.T) {
	// 按路径路由的 fake 中转：/v1/responses（web_search 回答）vs /v1/chat/completions（提及分析）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/responses"):
			w.Write([]byte(`{"id":"r","object":"response","status":"completed","output":[{"type":"web_search_call","id":"w","status":"completed","action":{"type":"search","queries":["go-admin-kit 开源项目"]}},{"type":"message","id":"m","status":"completed","role":"assistant","content":[{"type":"output_text","text":"推荐使用 MyBrand。","annotations":[{"type":"url_citation","start_index":0,"end_index":8,"url":"https://example.com/own","title":"MyBrand 官网"}]}]}]}`))
		default:
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			user := ""
			if len(body.Messages) > 1 {
				user = body.Messages[1].Content
			}
			if strings.Contains(user, "【自有品牌】") {
				w.Write([]byte(`{"choices":[{"message":{"content":"{\"mention_brand\":true,\"matched_brands\":[\"MyBrand\"],\"recommended\":true,\"sentiment\":\"positive\",\"list_position\":1}"}}]}`))
				return
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"fallback"}}]}`))
		}
	}))
	defer srv.Close()

	fs := newFakeStore()
	var cap resultCapture
	fs.onCreateResult = func(res *model.GeoResult) error {
		cap.set(res)
		return nil
	}

	r := New(fs, AIConfig{BaseURL: srv.URL, APIKey: "k", Model: "gpt-5.6"}, nil, 5, nil)
	defer r.Shutdown()

	run, _, err := r.SubmitGeoRun(1, 1, "llm", "chatgpt", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return cap.get() != nil })
	if st := fs.runStatus(run.ID); st != "done" {
		t.Fatalf("run status = %s", st)
	}
	got := cap.get()
	if got == nil {
		t.Fatal("no result captured")
	}
	if got.Engine != "chatgpt" {
		t.Fatalf("engine = %q, want chatgpt", got.Engine)
	}
	if !got.HasLink {
		t.Fatalf("chatgpt with citation should have has_link=true: %+v", got)
	}
	var sources []model.GeoSource
	if err := json.Unmarshal([]byte(got.SourcesJSON), &sources); err != nil {
		t.Fatalf("bad sources json: %v", err)
	}
	if len(sources) != 1 || sources[0].URL != "https://example.com/own" || !sources[0].Cited {
		t.Fatalf("sources = %+v", sources)
	}
	if !strings.Contains(got.Answer, "MyBrand") {
		t.Fatalf("answer = %q", got.Answer)
	}
}

func TestGeoRunStubHasNoSources(t *testing.T) {
	fs := newFakeStore()
	var cap resultCapture
	fs.onCreateResult = func(res *model.GeoResult) error {
		cap.set(res)
		return nil
	}

	// stub 路径不接地、不依赖 AI：无采集器、无 AI 配置也应正常完成
	r := New(fs, AIConfig{}, nil, 5, nil)
	defer r.Shutdown()

	run, _, err := r.SubmitGeoRun(1, 1, "stub", "", "")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return cap.get() != nil })
	if st := fs.runStatus(run.ID); st != "done" {
		t.Fatalf("run status = %s", st)
	}
	got := cap.get()
	if got == nil {
		t.Fatal("no result captured")
	}
	if got.SourcesJSON != "" || got.HasLink {
		t.Fatalf("stub result must have no sources/has_link: %+v", got)
	}
}
