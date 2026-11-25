package authdao

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	model "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/gorm"
)

func TestBaseUserDAOAndAuthUserOps(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.PasswordHistory{}, &model.TOTPRecoveryCode{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	base := NewBaseUserDAO(db)
	ops := NewAuthUserOps[model.User](db)

	u := &model.User{Username: "alice", Email: "alice@example.com", Phone: "13800001111", Password: "hash-v1"}
	if err := base.CreateUserContext(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if got, err := base.GetUserByUsernameContext(context.Background(), "alice"); err != nil || got.ID != u.ID {
		t.Fatalf("GetUserByUsernameContext = %+v err=%v", got, err)
	}
	if got, err := ops.GetUserByPhoneContext(context.Background(), "13800001111"); err != nil || got.ID != u.ID {
		t.Fatalf("GetUserByPhoneContext = %+v err=%v", got, err)
	}

	now := time.Now()
	if err := ops.UpdatePasswordWithHistoryContext(context.Background(), u.ID, "hash-v1", "hash-v2", now, 5); err != nil {
		t.Fatalf("UpdatePasswordWithHistoryContext: %v", err)
	}
	hist, err := ops.ListRecentPasswordHistoryContext(context.Background(), u.ID, 5)
	if err != nil || len(hist) != 1 || hist[0].PasswordHash != "hash-v1" {
		t.Fatalf("password history = %+v err=%v", hist, err)
	}

	if err := ops.EnableTOTPWithRecoveryCodesContext(context.Background(), u.ID, []string{"c1", "c2"}, now); err != nil {
		t.Fatalf("EnableTOTPWithRecoveryCodesContext: %v", err)
	}
	codes, err := ops.ListUnusedTOTPRecoveryCodesContext(context.Background(), u.ID)
	if err != nil || len(codes) != 2 {
		t.Fatalf("unused codes = %+v err=%v", codes, err)
	}
	if err := ops.MarkTOTPRecoveryCodeUsedContext(context.Background(), u.ID, codes[0].ID, now); err != nil {
		t.Fatalf("MarkTOTPRecoveryCodeUsedContext: %v", err)
	}
	if err := ops.DisableTOTPContext(context.Background(), u.ID); err != nil {
		t.Fatalf("DisableTOTPContext: %v", err)
	}
	codesAfter, _ := ops.ListUnusedTOTPRecoveryCodesContext(context.Background(), u.ID)
	if len(codesAfter) != 0 {
		t.Fatalf("expected 0 recovery codes after disable, got %d", len(codesAfter))
	}
}
