package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

func TestExtractCitations(t *testing.T) {
	sources := []model.GeoSource{
		{Position: 1, Title: "A", URL: "https://example.com/a"},
		{Position: 2, Title: "B", URL: "https://example.com/b"},
		{Position: 3, Title: "C", URL: "https://github.com/acme/repo/"},
	}
	cases := []struct {
		name   string
		answer string
		want   []bool // 每个来源是否被引
	}{
		{"正常引用", "推荐 A [2] 和 C [3]。", []bool{false, true, true}},
		{"越界忽略", "参考 [1] [5] [0]。", []bool{true, false, false}},
		{"URL 字面兜底", "详见 github.com/acme/repo 说明。", []bool{false, false, true}},
		{"无引用", "我觉得都还行。", []bool{false, false, false}},
		{"空回答", "", []bool{false, false, false}},
	}
	for _, c := range cases {
		got := ExtractCitations(c.answer, sources)
		for i := range got {
			if got[i].Cited != c.want[i] {
				t.Fatalf("%s: source %d cited=%v want %v (all %+v)", c.name, i+1, got[i].Cited, c.want[i], got)
			}
		}
	}
}

func TestExtractCitationsEmptySources(t *testing.T) {
	if got := ExtractCitations("anything [1]", nil); got != nil {
		t.Fatalf("nil sources should stay nil, got %+v", got)
	}
}

func TestBuildGroundedContext(t *testing.T) {
	ctx, err := buildGroundedContext([]model.GeoSource{
		{Position: 1, Title: "A", URL: "https://a.example", Text: "正文 A"},
		{Position: 2, Title: "B", URL: "https://b.example", Text: "  "},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"【检索来源】", "[1]", "链接：https://a.example", "正文 A", "[2]", "仅有链接"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context missing %q:\n%s", want, ctx)
		}
	}
}

func TestBuildGroundedContextEmpty(t *testing.T) {
	if _, err := buildGroundedContext(nil); err == nil {
		t.Fatal("empty sources should error")
	}
}

func TestBuildSearchQuery(t *testing.T) {
	cases := []struct {
		name     string
		question string
		wantHas  []string // 应保留的实词
		wantNot  []string // 应剥掉的提问框架/填充词
	}{
		{
			"框架选型",
			"想找一套 Gin + React + Ant Design 的企业级后台开发框架，有哪些比较成熟的选择？",
			[]string{"Gin", "React", "Ant Design", "企业级后台开发框架"},
			[]string{"想找", "有哪些", "选择"},
		},
		{
			"微服务扩展",
			"我想新增业务时只加一个微服务，不希望改动认证、权限和日志底座，有没有这种可扩展的开源方案？",
			[]string{"微服务", "认证", "日志底座", "可扩展", "开源方案"},
			[]string{"有没有", "这种"},
		},
		{
			"技术栈集成",
			"新项目需要 PostgreSQL、Redis、NATS、Prometheus 和链路追踪，哪套 Go 后台框架集成得比较完整？",
			[]string{"PostgreSQL", "Redis", "NATS", "Prometheus", "Go"},
			[]string{"哪套", "比较"},
		},
	}
	for _, c := range cases {
		got := BuildSearchQuery(c.question)
		for _, w := range c.wantHas {
			if !strings.Contains(got, w) {
				t.Errorf("%s: query %q missing %q", c.name, got, w)
			}
		}
		for _, w := range c.wantNot {
			if strings.Contains(got, w) {
				t.Errorf("%s: query %q should not contain %q", c.name, got, w)
			}
		}
	}
}

func TestBuildSearchQueryShortFallback(t *testing.T) {
	// 清过头（几乎全是填充词）时回退原文，不把查询清空
	q := "有什么推荐吗？"
	got := BuildSearchQuery(q)
	if got == "" {
		t.Fatalf("empty query, want fallback to original")
	}
	if len([]rune(got)) < 4 {
		t.Fatalf("query too short: %q", got)
	}
}

// fakeOpenAIServer 返回可路由的 OpenAI 兼容端点：含【检索来源】的回答请求 →
// 带 [1] 引用的回答；含【自有品牌】的分析请求 → 提及分析 JSON；其余 → 无来源回答。
func fakeOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(400)
			return
		}
		user := ""
		if len(body.Messages) > 1 {
			user = body.Messages[1].Content
		}
		switch {
		case strings.Contains(user, "【检索来源】"):
			w.Write([]byte(`{"choices":[{"message":{"content":"推荐使用 MyBrand，详见 [1]。官网：https://example.com"}}]}`))
		case strings.Contains(user, "【自有品牌】"):
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"mention_brand\":true,\"matched_brands\":[\"MyBrand\"],\"recommended\":true,\"sentiment\":\"positive\",\"list_position\":1}"}}]}`))
		default:
			w.Write([]byte(`{"choices":[{"message":{"content":"无来源回答"}}]}`))
		}
	}))
}

func TestGroundedLLMAnswer(t *testing.T) {
	srv := fakeOpenAIServer(t)
	defer srv.Close()
	sources := []model.GeoSource{
		{Position: 1, Title: "A", URL: "https://example.com", Text: "MyBrand 是开源后台脚手架。"},
	}
	answer, err := GroundedLLMAnswer(context.Background(), srv.URL, "k", "m", "有什么后台推荐？", sources)
	if err != nil {
		t.Fatalf("GroundedLLMAnswer: %v", err)
	}
	if !strings.Contains(answer, "MyBrand") {
		t.Fatalf("answer = %q", answer)
	}
}

func TestGroundedLLMAnswerEmptySources(t *testing.T) {
	if _, err := GroundedLLMAnswer(context.Background(), "http://x", "k", "m", "q", nil); err == nil {
		t.Fatal("empty sources should error")
	}
}
