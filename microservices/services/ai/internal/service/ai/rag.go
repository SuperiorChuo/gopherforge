package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-admin-kit/services/ai/internal/ai"
	aidao "github.com/go-admin-kit/services/ai/internal/dao/ai"
)

// M4 企业知识库 RAG：共享检索→注入 helper。console /ai/chat 与 Hermes 模型代理
// 都调用它，区别只在 persona preamble。

// knowledgeBaseTopK 是 RAG 检索返回的块数。
const knowledgeBaseTopK = 5

// ChunkSearcher 检索与 embedding 最近的 topK 块（租户隔离）。
// *aidao.DocumentDAO 实现它。
type ChunkSearcher interface {
	SearchChunksContext(ctx context.Context, embedding []float32, topK int) ([]aidao.ChunkMatch, error)
}

// BuildKnowledgePrompt embeds query，检索 topK 块，折进带 preamble 的 system
// prompt。无匹配或 embed 不可用时返回空串。错误向上抛，由调用方决定降级或中断。
func BuildKnowledgePrompt(ctx context.Context, embed ai.Provider, searcher ChunkSearcher, query, preamble string) (string, error) {
	if embed == nil || searcher == nil {
		return "", nil
	}
	vectors, err := embed.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return "", nil
	}
	matches, err := searcher.SearchChunksContext(ctx, vectors[0], knowledgeBaseTopK)
	if err != nil {
		return "", fmt.Errorf("search knowledge base: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(preamble)
	b.WriteString("\n\n知识库检索结果:\n")
	for i, match := range matches {
		fmt.Fprintf(&b, "\n[%d] 来源文档:%s\n%s\n", i+1, match.Title, match.Content)
	}
	return b.String(), nil
}
