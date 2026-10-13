package store

import (
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

func TestListGeoResultsDecodesSources(t *testing.T) {
	s := newTestStore(t)
	const tenant, run uint64 = 1, 1
	res := &model.GeoResult{
		TenantID: tenant, RunID: run, PromptID: 1, Question: "q", Answer: "a",
		SourcesJSON: `[{"position":1,"title":"t","url":"https://example.com","cited":true,"own_domain":false}]`,
	}
	if err := s.CreateGeoResult(res); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListGeoResults(tenant, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}
	got := list[0]
	if len(got.Sources) != 1 {
		t.Fatalf("sources = %+v", got.Sources)
	}
	src := got.Sources[0]
	if src.URL != "https://example.com" || !src.Cited || src.OwnDomain {
		t.Fatalf("source = %+v", src)
	}
}

func TestListGeoResultsEmptySources(t *testing.T) {
	s := newTestStore(t)
	const tenant, run uint64 = 1, 2
	if err := s.CreateGeoResult(&model.GeoResult{TenantID: tenant, RunID: run, PromptID: 1, Question: "q"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListGeoResults(tenant, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || len(list[0].Sources) != 0 {
		t.Fatalf("list = %+v", list)
	}
}

// TestOverviewDropStubWithRealEngineRows 验证：存在 chatgpt 等真实引擎行（即使没有
// 历史 llm 行）时，demo 模式的 stub 演示行也要被剔除，不能漏进总览。
func TestOverviewDropStubWithRealEngineRows(t *testing.T) {
	s := newTestStore(t)
	const tenant uint64 = 1
	if err := s.CreateGeoResult(&model.GeoResult{
		TenantID: tenant, RunID: 1, PromptID: 1, Question: "q1", Answer: "a",
		MentionBrand: true, Engine: "chatgpt",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateGeoResult(&model.GeoResult{
		TenantID: tenant, RunID: 2, PromptID: 1, Question: "q2", Answer: "b",
		MentionBrand: true, Engine: "stub",
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Overview(tenant, 30, true)
	if err != nil {
		t.Fatal(err)
	}
	if stats.GeoResultCount != 1 || stats.GeoMentionCount != 1 {
		t.Fatalf("geo result=%d mention=%d, want 1/1（stub 行应被剔除）", stats.GeoResultCount, stats.GeoMentionCount)
	}
}
