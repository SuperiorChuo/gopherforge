package agentadapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-admin-kit/services/ai/internal/agentcore"
	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

type plainProviderStub struct{}

func (plainProviderStub) Chat(_ context.Context, _ []aiclient.ChatMessage, onDelta func(aiclient.ChatDelta) error) error {
	if err := onDelta(aiclient.ChatDelta{Content: "plain"}); err != nil {
		return err
	}
	return onDelta(aiclient.ChatDelta{Done: true, Usage: &aiclient.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}})
}
func (plainProviderStub) Embed(context.Context, []string) ([][]float32, error) { return nil, nil }
func (plainProviderStub) GenerateImage(context.Context, string, aiclient.ImageOptions) (aiclient.ImageResult, error) {
	return aiclient.ImageResult{}, nil
}
func (plainProviderStub) Name() string { return "plain-stub" }

func TestCompletePlainChatUsesNativeProviderDeltas(t *testing.T) {
	result, err := completePlainChat(context.Background(), plainProviderStub{}, agentcore.ModelRequest{
		Messages: []agentcore.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("complete plain chat: %v", err)
	}
	if result.Content != "plain" || result.Usage.PromptTokens != 2 || result.Usage.CompletionTokens != 3 || result.StopReason != "stop" {
		t.Fatalf("plain result = %#v", result)
	}
}

func TestModelPortRequiresTenantBeforeLoadingModel(t *testing.T) {
	var loads atomic.Int32
	port := NewModelPort(func(context.Context, uint64) (*ModelConfig, error) {
		loads.Add(1)
		return &ModelConfig{ID: 2, Name: "model-a", Provider: "openai", APIKey: "key", Enabled: true}, nil
	}, nil, nil, nil)
	if _, err := port.Complete(context.Background(), agentcore.ModelRequest{ModelID: 2}); err == nil {
		t.Fatal("Complete without tenant succeeded")
	}
	if err := port.Validate(context.Background(), 2, false); err == nil {
		t.Fatal("Validate without tenant succeeded")
	}
	if loads.Load() != 0 {
		t.Fatalf("model loads without tenant = %d, want 0", loads.Load())
	}
}

func TestModelPortMapsKernelContractAndRecordsUsage(t *testing.T) {
	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["stream"] != false {
			t.Errorf("stream = %#v, want false", request["stream"])
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
	}))
	defer server.Close()

	var beforeCalls atomic.Int32
	var recorded UsageRecord
	port := NewModelPort(
		func(ctx context.Context, id uint64) (*ModelConfig, error) {
			return &ModelConfig{ID: id, Name: "model-a", Provider: "openai", BaseURL: server.URL, APIKey: "key", Enabled: true, CostPer1KInput: 0.01, CostPer1KOutput: 0.03}, nil
		},
		func(ctx context.Context, actor uint64, tenantID uint) error {
			beforeCalls.Add(1)
			return nil
		},
		func(ctx context.Context, record UsageRecord) error {
			recorded = record
			return nil
		}, nil,
	)
	ctx := tenant.WithContext(context.Background(), 7)
	response, err := port.Complete(ctx, agentcore.ModelRequest{
		ModelID: 2, ActorID: 9, RunID: 11,
		Messages: []agentcore.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.Content != "done" || response.Usage.InputTokens != 100 || response.Usage.OutputTokens != 50 {
		t.Fatalf("response = %#v", response)
	}
	if providerCalls.Load() != 1 || beforeCalls.Load() != 1 {
		t.Fatalf("provider=%d before=%d", providerCalls.Load(), beforeCalls.Load())
	}
	if recorded.ModelID != 2 || recorded.ActorID != 9 || recorded.TenantID != 7 || recorded.RunID != 11 || recorded.TotalTokens != 150 || recorded.Cost != 0.0025 || recorded.Status != "success" {
		t.Fatalf("usage = %#v", recorded)
	}
}

func TestModelPortSanitizesProviderErrorForKernel(t *testing.T) {
	const upstreamBody = "SECRET-UPSTREAM-BODY internal-gateway-host"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, upstreamBody)
	}))
	defer server.Close()

	// 把共享 logger 指到临时文件：本用例要同时证明"对外脱敏"与"诊断没丢"。
	logPath := filepath.Join(t.TempDir(), "agent.log")
	logger.InitLogger(logPath, "warn", 1, 1, 1)
	defer func() { logger.Logger = nil }()

	var recorded UsageRecord
	port := NewModelPort(
		func(ctx context.Context, id uint64) (*ModelConfig, error) {
			return &ModelConfig{ID: id, Name: "model-a", Provider: "openai", BaseURL: server.URL, APIKey: "key", Enabled: true}, nil
		},
		nil,
		func(ctx context.Context, record UsageRecord) error {
			recorded = record
			return nil
		},
		nil,
	)
	ctx := tenant.WithContext(context.Background(), 7)
	_, err := port.Complete(ctx, agentcore.ModelRequest{
		ModelID: 2, ActorID: 9, RunID: 11,
		Messages: []agentcore.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("provider failure was reported as success")
	}
	// Kernel 会把该错误写进 run.error_message，而它随 API 与 SSE 外发给持 ai:agent:list 的用户。
	if strings.Contains(err.Error(), upstreamBody) {
		t.Fatalf("kernel-facing error leaked the upstream body: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("kernel-facing error lost the upstream status code: %v", err)
	}
	// 脱敏不能把诊断也丢掉：上游原文的落点是服务日志。
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read service log: %v", readErr)
	}
	if !strings.Contains(string(logged), upstreamBody) {
		t.Fatalf("service log lost the upstream diagnostics: %s", string(logged))
	}
	if recorded.Status != "error" {
		t.Fatalf("usage status = %q, want error", recorded.Status)
	}
}

// 本地 provider 错误（不含上游正文）必须原样透传：它们正是可诊断的原因，
// 一律收敛成 "model provider call failed" 会把 run.error_message 变成一句废话。
func TestModelPortKeepsLocalProviderErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// HTTP 200 但零个 choice：属 provider 层本地校验失败，不是上游错误体。
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	port := NewModelPort(
		func(ctx context.Context, id uint64) (*ModelConfig, error) {
			return &ModelConfig{ID: id, Name: "model-a", Provider: "openai", BaseURL: server.URL, APIKey: "key", Enabled: true}, nil
		},
		nil, nil, nil,
	)
	ctx := tenant.WithContext(context.Background(), 7)
	_, err := port.Complete(ctx, agentcore.ModelRequest{
		ModelID: 2, ActorID: 9, RunID: 11,
		Messages: []agentcore.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("zero-choice provider response was reported as success")
	}
	if !strings.Contains(err.Error(), "choices = 0") {
		t.Fatalf("local provider detail was flattened away: %v", err)
	}
}

func TestKnowledgeSearchRejectsUnknownFieldsBeforeCallingBackend(t *testing.T) {
	registry := agentcore.NewToolRegistry()
	var calls atomic.Int32
	if err := RegisterKnowledgeSearch(registry, func(ctx context.Context, query string, topK int) (any, error) {
		calls.Add(1)
		return []string{"ok"}, nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx := tenant.WithContext(context.Background(), 8)
	if _, err := registry.Execute(ctx, "knowledge_search", agentcore.JSONValue(`{"query":"x","tenant_id":1}`)); err == nil {
		t.Fatal("unknown tenant_id field was accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls = %d, want 0", calls.Load())
	}
	result, err := registry.Execute(ctx, "knowledge_search", agentcore.JSONValue(`{"query":"x","top_k":3}`))
	if err != nil {
		t.Fatalf("valid search: %v", err)
	}
	if calls.Load() != 1 || string(result) != `{"matches":["ok"]}` {
		t.Fatalf("result=%s calls=%d", result, calls.Load())
	}
}
