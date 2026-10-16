package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

func TestAdviceMalformedStructuredOutputIsNotMarkdown(t *testing.T) {
	for _, text := range []string{`{"markdown":"truncated`, `{"markdown":7,"actions":[]}`, `[{"markdown":"wrong root"}]`, "```json\n{\"markdown\":broken}\n```", "建议如下：\n{\"markdown\":\"truncated", "建议如下：\n[{\"markdown\":\"wrong root\"}]", "null", "true", `"not an object"`, "42", `建议如下：{ "markdown": "truncated`, "建议如下：{\t\"markdown\":\"truncated"} {
		t.Run(text, func(t *testing.T) {
			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}}}})
			}))
			defer server.Close()
			bundle, err := GenerateGeoAdvice(context.Background(), server.URL, "test-key", "test-model", &model.GeoRun{}, []model.GeoResult{{Question: "q", Engine: "selfhost"}}, nil)
			if hits != 1 || err == nil || bundle.Markdown != "" {
				t.Fatalf("malformed JSON accepted as advice: hits=%d bundle=%+v err=%v", hits, bundle, err)
			}
		})
	}
}
func TestAdviceMarkdownCompatibilityAndJSONNormalization(t *testing.T) {
	for _, text := range []string{"## 立即可做\n- 补充 FAQ 与对比文档", "[官网](https://example.com)\n## 立即可做\n- 补充 FAQ 与对比文档", "[官网]: https://example.com\n## 立即可做\n- 补充 FAQ", "[官网][site]\n- 补充 FAQ\n[site]: https://example.com", `{"markdown":"诊断正文","actions":[{"title":"修复内容","priority":"HIGH"}]}`, "[FAQ [入门]](https://example.com)\n- 补充 FAQ", "[\"FAQ\"](https://example.com)\n- 补充 FAQ", "[FAQ \\] 入门](https://example.com)\n- 补充 FAQ"} {
		t.Run(text, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}}}})
			}))
			defer server.Close()
			bundle, err := GenerateGeoAdvice(context.Background(), server.URL, "test-key", "test-model", &model.GeoRun{}, []model.GeoResult{{Question: "q", Engine: "selfhost"}}, nil)
			if err != nil || bundle.Markdown == "" || len(bundle.Actions) == 0 || bundle.Actions[0].ID == "" {
				t.Fatalf("bundle=%+v err=%v", bundle, err)
			}
		})
	}
}
