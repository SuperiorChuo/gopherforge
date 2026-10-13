package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeResponsesServer 模拟中转 /v1/responses + web_search：返回一个
// web_search_call（真实检索词）+ 带 url_citation 注解的 message。
func fakeResponsesServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatgptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}
		if len(req.Tools) != 1 || req.Tools[0].Type != "web_search" {
			t.Errorf("expected web_search tool, got %+v", req.Tools)
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{
			"id":"resp_test","object":"response","status":"completed",
			"output":[
				{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","queries":["go-admin-kit 开源项目","\"go-admin-kit\""]}},
				{"type":"message","id":"m1","status":"completed","role":"assistant","content":[
					{"type":"output_text","text":"推荐 go-admin-kit。","annotations":[
						{"type":"url_citation","start_index":5,"end_index":20,"url":"https://github.com/SuperiorChuo/gopherforge","title":"GopherForge"}
					]}
				]}
			]
		}`))
	}))
}

func TestChatGPTAsk(t *testing.T) {
	srv := fakeResponsesServer(t)
	defer srv.Close()
	ans, err := (chatgptEngine{}).Ask(context.Background(),
		Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-5.6"}, "go-admin-kit 是什么？", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(ans.Answer, "go-admin-kit") {
		t.Fatalf("answer = %q", ans.Answer)
	}
	if len(ans.Queries) != 2 || ans.Queries[0] != "go-admin-kit 开源项目" {
		t.Fatalf("queries = %v", ans.Queries)
	}
	if len(ans.Sources) != 1 {
		t.Fatalf("sources = %+v", ans.Sources)
	}
	s := ans.Sources[0]
	if s.URL != "https://github.com/SuperiorChuo/gopherforge" || !s.Cited || s.Title != "GopherForge" {
		t.Fatalf("source = %+v", s)
	}
}

func TestChatGPTAskEmptyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"resp","object":"response","status":"completed","output":[]}`))
	}))
	defer srv.Close()
	if _, err := (chatgptEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, "q", nil); err == nil {
		t.Fatal("empty output should error")
	}
}

func TestChatGPTAskTopLevelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"resp","object":"response","status":"failed","error":{"message":"web search unavailable"}}`))
	}))
	defer srv.Close()
	_, err := (chatgptEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, "q", nil)
	if err == nil || !strings.Contains(err.Error(), "web search unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatgptResponsesURL(t *testing.T) {
	cases := map[string]string{
		"http://x:8082/v1": "http://x:8082/v1/responses",
		"http://x:8082/":   "http://x:8082/v1/responses",
		"http://x:8082":    "http://x:8082/v1/responses",
		"":                 "https://api.openai.com/v1/responses",
	}
	for in, want := range cases {
		if got := chatgptResponsesURL(in); got != want {
			t.Errorf("chatgptResponsesURL(%q) = %q, want %q", in, got, want)
		}
	}
}
