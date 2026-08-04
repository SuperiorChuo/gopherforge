package auth

import (
	"context"
	"errors"
	"testing"
)

// 空/空白邀请 token 在 DB 访问前即拒绝（无 token 的注册路径已封死）。
func TestConsumeInviteRejectsEmptyToken(t *testing.T) {
	svc := NewUserServiceWithDB(nil)
	if _, err := svc.consumeInvite(context.Background(), "   "); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("consumeInvite('') err = %v, want ErrInviteInvalid", err)
	}
}

// consumeInvite 对无效 token 也返回 ErrInviteInvalid（记录不存在路径）。
// nil db 下 GetByTokenHash 会 panic，此处仅验证空 token 短路逻辑；
// 完整消费链路（有效/已用/过期/撤销）由 109 E2E 覆盖。
