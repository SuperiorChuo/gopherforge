package store

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/im/internal/model"
	"gorm.io/gorm"
)

// DeleteAgentSkill 必须拒绝删除其技能组属于其他租户的绑定
// （此前是裸主键删除）。
func TestDeleteAgentSkillTenantScoped(t *testing.T) {
	s := newTestStore(t)

	gA, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 1, Name: "A组", Code: "grp-a"})
	if err != nil {
		t.Fatal(err)
	}
	gB, err := s.CreateSkillGroup(context.Background(), SkillGroupInput{TenantID: 2, Name: "B组", Code: "grp-b"})
	if err != nil {
		t.Fatal(err)
	}
	skA, err := s.UpsertAgentSkill(context.Background(), 101, gA.ID, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	skB, err := s.UpsertAgentSkill(context.Background(), 202, gB.ID, 5, 1)
	if err != nil {
		t.Fatal(err)
	}

	// tenant 1 尝试删除 tenant 2 的绑定：应找不到，行保留
	if err := s.DeleteAgentSkill(context.Background(), skB.ID, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-tenant delete: want ErrRecordNotFound, got %v", err)
	}
	var n int64
	if err := s.db.Model(&model.AgentSkill{}).Where("id = ?", skB.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cross-tenant delete removed the row (count=%d)", n)
	}

	// 同租户删除正常
	if err := s.DeleteAgentSkill(context.Background(), skA.ID, 1); err != nil {
		t.Fatalf("own-tenant delete failed: %v", err)
	}
	if err := s.db.Model(&model.AgentSkill{}).Where("id = ?", skA.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("own-tenant delete left the row behind")
	}

	// 未知 id：找不到
	if err := s.DeleteAgentSkill(context.Background(), 999999, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing id: want ErrRecordNotFound, got %v", err)
	}
}
