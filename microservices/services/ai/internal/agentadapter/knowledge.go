package agentadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/go-admin-kit/services/ai/internal/agentcore"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

type KnowledgeSearchFunc func(context.Context, string, int) (any, error)

type knowledgeSearchInput struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

// RegisterKnowledgeSearch exposes the existing tenant-aware Knowledge Service
// through one strict, read-only Agent tool contract.
func RegisterKnowledgeSearch(registry *agentcore.ToolRegistry, search KnowledgeSearchFunc) error {
	if registry == nil || search == nil {
		return fmt.Errorf("knowledge search adapter is not configured")
	}
	return registry.Register(agentcore.ToolDefinition{
		Schema: agentcore.ToolSpecSchema,
		Name:   "knowledge_search", Description: "在当前租户知识库中检索与问题相关的片段", ReadOnly: true, TimeoutMS: 15_000,
		InputSchema: agentcore.MustJSONValue(map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"query"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000},
				"top_k": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			},
		}),
	}, func(ctx context.Context, arguments agentcore.JSONValue) (agentcore.JSONValue, error) {
		if _, err := tenant.Require(ctx); err != nil {
			return nil, fmt.Errorf("knowledge search requires tenant context")
		}
		decoder := json.NewDecoder(bytes.NewReader(arguments))
		decoder.DisallowUnknownFields()
		var input knowledgeSearchInput
		if err := decoder.Decode(&input); err != nil {
			return nil, fmt.Errorf("invalid knowledge_search arguments: %w", err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, err
		}
		input.Query = strings.TrimSpace(input.Query)
		if input.TopK == 0 {
			input.TopK = 5
		}
		if input.Query == "" || utf8.RuneCountInString(input.Query) > 2000 || input.TopK < 1 || input.TopK > 10 {
			return nil, fmt.Errorf("invalid knowledge_search arguments")
		}
		matches, err := search(ctx, input.Query, input.TopK)
		if err != nil {
			return nil, err
		}
		return agentcore.NewJSONValue(map[string]any{"matches": matches})
	})
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid knowledge_search arguments: multiple JSON values")
		}
		return fmt.Errorf("invalid knowledge_search arguments: %w", err)
	}
	return nil
}
