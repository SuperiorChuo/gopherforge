package ai

import (
	"context"
	"strings"
	"testing"

	aiclient "github.com/go-admin-kit/services/ai/internal/ai"
)

// stubProvider is a test double implementing the ai.Provider interface. It
// records the messages passed to Chat and replays a fixed reply, and captures
// image-generation calls.
type stubProvider struct {
	chatMsgs  []aiclient.ChatMessage
	chatReply string

	imagePrompt string
	imageOpts   aiclient.ImageOptions
	imageResult aiclient.ImageResult
}

func (p *stubProvider) Chat(ctx context.Context, msgs []aiclient.ChatMessage, onDelta func(aiclient.ChatDelta) error) error {
	p.chatMsgs = msgs
	if err := onDelta(aiclient.ChatDelta{Content: p.chatReply}); err != nil {
		return err
	}
	return onDelta(aiclient.ChatDelta{Done: true})
}

func (p *stubProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, nil
}

func (p *stubProvider) GenerateImage(ctx context.Context, prompt string, opts aiclient.ImageOptions) (aiclient.ImageResult, error) {
	p.imagePrompt = prompt
	p.imageOpts = opts
	return p.imageResult, nil
}

func (p *stubProvider) Name() string { return "stub" }

func newInsightServiceWithProvider(p aiclient.Provider) InsightService {
	return NewInsightServiceWithDB(nil, aiclient.Providers{Chat: p, Embed: p})
}

func TestComposeWechatArticleUsesStructuredJSONSystemPrompt(t *testing.T) {
	const reply = `{"title":"标题","digest":"摘要","content":"<p>正文</p>"}`
	stub := &stubProvider{chatReply: reply}
	svc := newInsightServiceWithProvider(stub)

	content, err := svc.Compose(context.Background(), ComposeRequest{
		Kind:   "wechat_article",
		Prompt: "写一篇关于秋天的公众号文章",
	})
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	if content != reply {
		t.Fatalf("Compose() content = %q, want %q", content, reply)
	}

	if len(stub.chatMsgs) < 1 || stub.chatMsgs[0].Role != aiclient.RoleSystem {
		t.Fatalf("Compose() first message = %+v, want system prompt", stub.chatMsgs)
	}
	system := stub.chatMsgs[0].Content
	// The wechat_article branch must ask for structured JSON with these keys.
	for _, want := range []string{"微信公众号", "JSON", `"title"`, `"digest"`, `"content"`, "<p>"} {
		if !strings.Contains(system, want) {
			t.Fatalf("Compose() wechat_article system prompt missing %q; got: %s", want, system)
		}
	}
}

func TestComposeNoticeUsesDifferentBranch(t *testing.T) {
	stub := &stubProvider{chatReply: "公告正文"}
	svc := newInsightServiceWithProvider(stub)

	if _, err := svc.Compose(context.Background(), ComposeRequest{Kind: "notice", Prompt: "放假通知"}); err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	system := stub.chatMsgs[0].Content
	// The notice branch must not leak the wechat_article JSON contract.
	if strings.Contains(system, "JSON") || strings.Contains(system, "微信公众号") {
		t.Fatalf("Compose() notice branch used wrong system prompt: %s", system)
	}
}

func TestInsightGenerateImageDelegatesToProvider(t *testing.T) {
	stub := &stubProvider{imageResult: aiclient.ImageResult{URLs: []string{"https://img/1.png"}}}
	svc := newInsightServiceWithProvider(stub)

	result, err := svc.GenerateImage(context.Background(), "a fox", aiclient.ImageOptions{Size: "512x512"})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if stub.imagePrompt != "a fox" {
		t.Fatalf("GenerateImage() prompt = %q, want a fox", stub.imagePrompt)
	}
	if stub.imageOpts.Size != "512x512" {
		t.Fatalf("GenerateImage() opts.Size = %q, want 512x512", stub.imageOpts.Size)
	}
	if len(result.URLs) != 1 || result.URLs[0] != "https://img/1.png" {
		t.Fatalf("GenerateImage() result = %+v", result)
	}
}
