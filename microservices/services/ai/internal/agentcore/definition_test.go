package agentcore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/shared/pkg/tenant"
)

// 编辑/启停只能修改逻辑定义；已发布版本和已提交运行不能被管理操作悄悄重写。
func TestDefinitionEditingPreservesVersionAndExistingRun(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 71, nil, 4)
	versionBefore, err := repository.GetVersion(ctx, *definition.LatestVersionID)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "已有运行", IdempotencyKey: "before-disable"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateDefinition(ctx, definition.ID, UpdateDefinitionInput{Name: "  新名称  ", Description: "", Status: DefinitionDisabled})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "新名称" || updated.Status != DefinitionDisabled || updated.AgentKey != definition.AgentKey || *updated.LatestVersionID != *definition.LatestVersionID {
		t.Fatalf("更新后定义字段不符合预期: %+v", updated)
	}
	if _, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "禁用期间", IdempotencyKey: "during-disable"}); !errors.Is(err, ErrDefinitionDisabled) {
		t.Fatalf("禁用定义新建Run错误=%v", err)
	}
	run, err := repository.GetRun(ctx, existing.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunQueued {
		t.Fatalf("禁用错误地取消了已提交Run: %s", run.Status)
	}
	versionAfter, err := repository.GetVersion(ctx, *definition.LatestVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if versionAfter.SpecSHA256 != versionBefore.SpecSHA256 || string(versionAfter.Spec) != string(versionBefore.Spec) {
		t.Fatal("编辑逻辑定义修改了已发布版本快照")
	}
	if _, err := service.UpdateDefinition(ctx, definition.ID, UpdateDefinitionInput{Name: "新名称", Description: "重新启用", Status: DefinitionActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateRun(ctx, definition.ID, 99, CreateRunInput{Input: "启用后", IdempotencyKey: "after-enable"}); err != nil {
		t.Fatalf("重新启用仍不能创建Run: %v", err)
	}
}

// 跨租户ID、缺失租户与非法字段都不能改变原定义；两个租户真实上下文而非固定租户mock。
func TestDefinitionEditingRejectsForeignTenantAndInvalidFields(t *testing.T) {
	repository := newTestRepository(t)
	service := NewService(repository, &fakeModel{}, NewToolRegistry())
	ctx, definition := createPublishedAgent(t, service, 72, nil, 4)
	valid := UpdateDefinitionInput{Name: "越权名称", Description: "", Status: DefinitionDisabled}
	if _, err := service.UpdateDefinition(tenant.WithContext(context.Background(), 73), definition.ID, valid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("跨租户编辑错误=%v", err)
	}
	if _, err := service.UpdateDefinition(context.Background(), definition.ID, valid); err == nil {
		t.Fatal("缺失租户被放行")
	}
	for _, input := range []UpdateDefinitionInput{
		{Name: " ", Status: DefinitionActive},
		{Name: strings.Repeat("😀", 101), Status: DefinitionActive},
		{Name: "合法", Description: strings.Repeat("𠮷", 501), Status: DefinitionActive},
		{Name: "合法", Status: "running"},
	} {
		if _, err := service.UpdateDefinition(ctx, definition.ID, input); !errors.Is(err, ErrValidation) {
			t.Fatalf("非法输入未拒绝: %v", err)
		}
	}
	unchanged, err := service.GetDefinition(ctx, definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != definition.Name || unchanged.Status != DefinitionActive {
		t.Fatal("拒绝的请求修改了定义")
	}
	if _, err := service.UpdateDefinition(ctx, definition.ID, UpdateDefinitionInput{Name: strings.Repeat("😀", 100), Description: strings.Repeat("𠮷", 500), Status: DefinitionActive}); err != nil {
		t.Fatalf("合法Unicode边界未放行: %v", err)
	}
}
