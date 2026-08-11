package auth

import (
	"context"
	"testing"
	"time"

	localmodel "github.com/go-admin-kit/services/auth/internal/model"
	"gorm.io/gorm"
)

// fakeResetDAO is an in-memory PasswordResetDAO implementing the same methods.
type fakeResetDAO struct {
	rows map[uint]*localmodel.PasswordReset
	next uint
}

func newFakeResetDAO() *fakeResetDAO {
	return &fakeResetDAO{rows: map[uint]*localmodel.PasswordReset{}, next: 1}
}

func (f *fakeResetDAO) CreateContext(_ context.Context, reset *localmodel.PasswordReset) error {
	reset.ID = f.next
	f.next++
	f.rows[reset.ID] = reset
	return nil
}

func (f *fakeResetDAO) GetByTokenHashContext(_ context.Context, tokenHash string) (*localmodel.PasswordReset, error) {
	for _, row := range f.rows {
		if row.TokenHash == tokenHash {
			return row, nil
		}
	}
	return &localmodel.PasswordReset{}, gorm.ErrRecordNotFound
}

func (f *fakeResetDAO) MarkUsedContext(_ context.Context, id uint) error {
	row, ok := f.rows[id]
	if !ok || row.UsedAt != nil || time.Now().After(row.ExpiresAt) {
		return gorm.ErrRecordNotFound
	}
	now := time.Now()
	row.UsedAt = &now
	return nil
}

func (f *fakeResetDAO) PruneExpiredContext(_ context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func TestResetTokenHashRoundTrip(t *testing.T) {
	token := "abc-123-xyz"
	h := hashResetToken(token)
	if h != hashResetToken(token) {
		t.Fatal("hash must be deterministic")
	}
	if len(h) != 64 {
		t.Fatalf("hash length = %d, want 64 (sha256 hex)", len(h))
	}
	if h == token {
		t.Fatal("plaintext token must never equal its hash")
	}
}

func TestResetTokenConsumption(t *testing.T) {
	dao := newFakeResetDAO()
	now := time.Now().UTC()
	future := now.Add(time.Hour)

	// 未用未过期 → 消费成功
	row := &localmodel.PasswordReset{UserID: 1, TokenHash: hashResetToken("t1"), ExpiresAt: future}
	_ = dao.CreateContext(context.Background(), row)
	if err := dao.MarkUsedContext(context.Background(), row.ID); err != nil {
		t.Fatalf("first consume error = %v, want nil", err)
	}
	// 二次消费 → 失败（防重放）
	if err := dao.MarkUsedContext(context.Background(), row.ID); err == nil {
		t.Fatal("second consume must fail")
	}

	// 过期 → 消费失败
	expired := &localmodel.PasswordReset{UserID: 2, TokenHash: hashResetToken("t2"), ExpiresAt: now.Add(-time.Minute)}
	_ = dao.CreateContext(context.Background(), expired)
	if err := dao.MarkUsedContext(context.Background(), expired.ID); err == nil {
		t.Fatal("expired token must not be consumable")
	}
}

func TestForgotPasswordUnknownEmailReturnsNil(t *testing.T) {
	svc := &PasswordResetService{now: time.Now}
	// 无 dao：未知邮箱语义上等同成功，不 panic、不报错。
	if err := svc.ForgotPasswordContext(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("unknown email error = %v, want nil (anti-enumeration)", err)
	}
}

func TestResetPasswordEmptyTokenRejected(t *testing.T) {
	svc := &PasswordResetService{now: time.Now}
	if err := svc.ResetPasswordContext(context.Background(), "", "NewPass123"); err != ErrResetTokenInvalid {
		t.Fatalf("empty token error = %v, want ErrResetTokenInvalid", err)
	}
}
