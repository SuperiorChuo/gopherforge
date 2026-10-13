package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPerplexityAsk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req perplexityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}
		if !req.ReturnCitations || len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("bad request: %+v", req)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"推荐 MyBrand。"}}],"citations":["https://example.com/a","https://example.com/b"]}`))
	}))
	defer srv.Close()

	ans, err := (perplexityEngine{}).Ask(context.Background(),
		Config{BaseURL: srv.URL, APIKey: "k", Model: "sonar"}, "有什么后台推荐？", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(ans.Answer, "MyBrand") {
		t.Fatalf("answer = %q", ans.Answer)
	}
	if len(ans.Sources) != 2 || ans.Sources[0].URL != "https://example.com/a" || !ans.Sources[0].Cited {
		t.Fatalf("sources = %+v", ans.Sources)
	}
}

func TestPerplexityAskNoCitations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模型从通用知识回答，无 citations——不应报错，只是 sources 为空
		w.Write([]byte(`{"choices":[{"message":{"content":"从记忆回答"}}],"citations":[]}`))
	}))
	defer srv.Close()
	ans, err := (perplexityEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "sonar"}, "q", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(ans.Sources) != 0 {
		t.Fatalf("sources = %+v", ans.Sources)
	}
}

func TestPerplexityAskEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	if _, err := (perplexityEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "sonar"}, "q", nil); err == nil {
		t.Fatal("empty choices should error")
	}
}

func TestPerplexityCompletionsURL(t *testing.T) {
	cases := map[string]string{
		"https://api.perplexity.ai":  "https://api.perplexity.ai/chat/completions",
		"https://api.perplexity.ai/": "https://api.perplexity.ai/chat/completions",
		"https://relay:8082/v1":      "https://relay:8082/v1/chat/completions",
		"https://x/chat/completions": "https://x/chat/completions",
	}
	for in, want := range cases {
		if got := perplexityCompletionsURL(in); got != want {
			t.Errorf("perplexityCompletionsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeminiAsk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		var req geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(400)
			return
		}
		if len(req.Tools) != 1 || req.Tools[0].GoogleSearch == nil {
			t.Errorf("expected google_search tool: %+v", req.Tools)
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"推荐 MyBrand。"}]},"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://example.com/a","title":"A"}},{"web":{"uri":"https://example.com/b","title":"B"}}]}}]}`))
	}))
	defer srv.Close()

	ans, err := (geminiEngine{}).Ask(context.Background(),
		Config{BaseURL: srv.URL, APIKey: "k", Model: "gemini-2.5-flash"}, "有什么后台推荐？", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(ans.Answer, "MyBrand") {
		t.Fatalf("answer = %q", ans.Answer)
	}
	if len(ans.Sources) != 2 || ans.Sources[0].URL != "https://example.com/a" || ans.Sources[0].Title != "A" || !ans.Sources[0].Cited {
		t.Fatalf("sources = %+v", ans.Sources)
	}
}

func TestGeminiAskNoGrounding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 无 groundingChunks（部分模型不返回引用）——不报错，sources 为空
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"普通回答"}]}}]}`))
	}))
	defer srv.Close()
	ans, err := (geminiEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, "q", nil)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(ans.Sources) != 0 {
		t.Fatalf("sources = %+v", ans.Sources)
	}
}

func TestGeminiAskError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":{"message":"API key invalid"}}`))
	}))
	defer srv.Close()
	if _, err := (geminiEngine{}).Ask(context.Background(), Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, "q", nil); err == nil || !strings.Contains(err.Error(), "API key invalid") {
		t.Fatalf("err = %v", err)
	}
}

func TestGeminiGenerateURL(t *testing.T) {
	if got := geminiGenerateURL("https://generativelanguage.googleapis.com", "gemini-2.5-flash"); got != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("url = %q", got)
	}
}
