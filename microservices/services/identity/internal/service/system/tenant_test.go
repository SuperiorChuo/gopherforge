package system

import (
	"context"
	"errors"
	"testing"

	"github.com/go-admin-kit/services/identity/internal/service/auth"
)

// Delete 的 id==1 守卫在 DB 访问前完成，nil db 即可测。
func TestTenantDeleteRejectsDefaultTenant(t *testing.T) {
	svc := NewTenantServiceWithDB(nil)
	if err := svc.Delete(context.Background(), 1); !errors.Is(err, ErrDefaultTenantLocked) {
		t.Fatalf("Delete(1) err = %v, want ErrDefaultTenantLocked", err)
	}
}

// 随机初始密码必须过密码强度校验（≥8 位、含大小写 + 数字）。
func TestRandomStrongPasswordPassesStrength(t *testing.T) {
	pwd, err := randomStrongPassword()
	if err != nil {
		t.Fatalf("randomStrongPassword: %v", err)
	}
	if len(pwd) < 8 {
		t.Fatalf("password too short: %q", pwd)
	}
	if err := auth.ValidatePasswordStrength(pwd); err != nil {
		t.Fatalf("password %q failed strength: %v", pwd, err)
	}
}
