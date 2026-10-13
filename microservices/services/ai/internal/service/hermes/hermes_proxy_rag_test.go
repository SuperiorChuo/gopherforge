package service

import (
	"context"
	"strings"
	"testing"
	"time"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/ai/internal/dao"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
)

// fakeProxyProvider 同时充当 Chat 与 Embed 通道：捕获交给模型的消息序列以便
// 断言注入结果，Embed 返回固定向量（或按配置报错以测降级路径）。
type fakeProxyProvider struct {
	msgs     []aiclient.ChatMessage
	embedErr error
}

func (f *fakeProxyProvider) Chat(ctx context.Context, msgs []aiclient.ChatMessage, onDelta func(aiclient.ChatDelta) error) error {
	f.msgs = append([]aiclient.ChatMessage{}, msgs...)
	return onDelta(aiclient.ChatDelta{Content: "ok", Done: true})
}

func (f *fakeProxyProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	return [][]float32{{0.1, 0.2, 0.3}}, nil
}

func (f *fakeProxyProvider) GenerateImage(ctx context.Context, prompt string, opts aiclient.ImageOptions) (aiclient.ImageResult, error) {
	return aiclient.ImageResult{}, nil
}

func (f *fakeProxyProvider) Name() string { return "fake" }

// fakeChunkSearcher 返回固定知识库命中，或按配置返回空/错误。
type fakeChunkSearcher struct {
	matches   []aidao.ChunkMatch
	searchErr error
}

func (f *fakeChunkSearcher) SearchChunksContext(ctx context.Context, embedding []float32, topK int) ([]aidao.ChunkMatch, error) {
	return f.matches, f.searchErr
}

func newRAGTestProxy(t *testing.T, svc *HermesService, store *dao.HermesDAO, chat *fakeProxyProvider, searcher *fakeChunkSearcher) *HermesProxyService {
	t.Helper()
	db := store.DB()
	if err := db.AutoMigrate(&localmodel.HermesModelToken{}); err != nil {
		t.Fatalf("migrate tokens: %v", err)
	}
	return NewHermesProxyService(
		svc,
		dao.NewHermesModelTokenDAO(db),
		nil,
		aiclient.Providers{Chat: chat, Embed: chat},
		nil,
		nil,
		searcher,
		func() time.Time { return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC) },
	)
}

// M4 企业知识库 RAG：策略 enable_knowledge_rag 为真时，proxy 转发前检索知识库
// 并把相关块折成 system message 置前；原消息序列一字不动跟在后面。
func TestModelProxyInjectsKnowledgeRAGWhenPolicyEnables(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	chat := &fakeProxyProvider{}
	searcher := &fakeChunkSearcher{matches: []aidao.ChunkMatch{{
		DocumentID: 1, Title: "假期制度", ChunkIndex: 0, Content: "员工每年享有 15 天年假", Score: 0.9,
	}}}
	proxy := newRAGTestProxy(t, svc, store, chat, searcher)

	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:               "rag policy",
		DesiredState:       localmodel.HermesDesiredStateActive,
		Toolsets:           localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		ModelName:          "model-rag",
		ModelBaseURL:       "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds:       3600,
		EnableKnowledgeRAG: true,
	})
	if err != nil {
		t.Fatalf("create rag policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)
	token, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	caller, scoped, err := proxy.Authorize(ctx, "Bearer "+token.Token)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if caller.Policy == nil || !caller.Policy.EnableKnowledgeRAG {
		t.Fatalf("caller policy = %+v, want enable_knowledge_rag=true", caller.Policy)
	}

	err = proxy.StreamChat(scoped, caller, HermesProxyRequest{
		Model: "model-rag",
		Messages: []HermesProxyMessage{
			{Role: "system", Content: "existing system prompt"},
			{Role: "user", Content: "今年我还能休几天年假？"},
		},
	}, func(delta aiclient.ChatDelta) error { return nil })
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	if len(chat.msgs) == 0 {
		t.Fatal("no messages reached the model channel")
	}

	// 注入的 system message 置前，携带知识库检索块
	first := chat.msgs[0]
	if first.Role != aiclient.RoleSystem {
		t.Fatalf("first message role = %q, want system", first.Role)
	}
	if !strings.Contains(first.Content, "知识库检索结果") || !strings.Contains(first.Content, "员工每年享有 15 天年假") {
		t.Fatalf("first system message lacks knowledge chunk: %q", first.Content)
	}
	// 原 user 消息必须原样保留在末尾
	last := chat.msgs[len(chat.msgs)-1]
	if last.Role != aiclient.RoleUser || last.Content != "今年我还能休几天年假？" {
		t.Fatalf("last message = %+v, want original user message preserved", last)
	}
	// 原 system 提示词不得被篡改
	var preserved bool
	for _, m := range chat.msgs[1:] {
		if m.Role == aiclient.RoleSystem && m.Content == "existing system prompt" {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("original system prompt should be preserved")
	}
}

// 策略关闭 enable_knowledge_rag 时，消息原样转发，不注入任何检索块。
func TestModelProxySkipsKnowledgeWhenPolicyDisabled(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	chat := &fakeProxyProvider{}
	// searcher 即便配了命中也不该被调用（策略关了开关）
	searcher := &fakeChunkSearcher{matches: []aidao.ChunkMatch{{
		DocumentID: 1, Title: "假期制度", ChunkIndex: 0, Content: "员工每年享有 15 天年假", Score: 0.9,
	}}}
	proxy := newRAGTestProxy(t, svc, store, chat, searcher)

	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:         "plain policy",
		DesiredState: localmodel.HermesDesiredStateActive,
		Toolsets:     localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		ModelName:    "model-plain",
		ModelBaseURL: "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("create plain policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)
	token, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	caller, scoped, err := proxy.Authorize(ctx, "Bearer "+token.Token)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}

	err = proxy.StreamChat(scoped, caller, HermesProxyRequest{
		Model: "model-plain",
		Messages: []HermesProxyMessage{
			{Role: "system", Content: "existing system prompt"},
			{Role: "user", Content: "普通问题"},
		},
	}, func(delta aiclient.ChatDelta) error { return nil })
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	if len(chat.msgs) != 2 {
		t.Fatalf("got %d messages, want original 2 without injection", len(chat.msgs))
	}
	if chat.msgs[0].Role != aiclient.RoleSystem || chat.msgs[0].Content != "existing system prompt" {
		t.Fatalf("first message = %+v, want untouched original system prompt", chat.msgs[0])
	}
}

// embed/检索失败是降级路径：RAG 是增强而非必需，员工对话不能被检索故障打断。
func TestModelProxyDegradesWhenKnowledgeRAGFails(t *testing.T) {
	svc, store, ctx, _ := setupHermesServiceTest(t, true)
	chat := &fakeProxyProvider{embedErr: context.Canceled}
	searcher := &fakeChunkSearcher{}
	proxy := newRAGTestProxy(t, svc, store, chat, searcher)

	policy, err := svc.CreatePolicy(ctx, 1, CreateHermesPolicyRequest{
		Name:               "rag-degrade policy",
		DesiredState:       localmodel.HermesDesiredStateActive,
		Toolsets:           localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
		ModelName:          "model-degrade",
		ModelBaseURL:       "https://control.example/api/v1/ai/hermes/proxy/v1",
		LeaseSeconds:       3600,
		EnableKnowledgeRAG: true,
	})
	if err != nil {
		t.Fatalf("create rag policy: %v", err)
	}
	createHermesTestEntitlement(t, svc, ctx, true, policy.ID)
	device := enrollHermesTestDevice(t, svc, store, ctx)
	token, err := proxy.IssueModelToken(ctx, device)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	caller, scoped, err := proxy.Authorize(ctx, "Bearer "+token.Token)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}

	err = proxy.StreamChat(scoped, caller, HermesProxyRequest{
		Model: "model-degrade",
		Messages: []HermesProxyMessage{
			{Role: "user", Content: "embed 挂了问题"},
		},
	}, func(delta aiclient.ChatDelta) error { return nil })
	if err != nil {
		t.Fatalf("stream chat must degrade, got error: %v", err)
	}
	if len(chat.msgs) != 1 {
		t.Fatalf("got %d messages, want only the original user message (no injection)", len(chat.msgs))
	}
	if chat.msgs[0].Role != aiclient.RoleUser || strings.Contains(chat.msgs[0].Content, "知识库检索结果") {
		t.Fatalf("degraded message = %+v, want original user message without knowledge injection", chat.msgs[0])
	}
}
