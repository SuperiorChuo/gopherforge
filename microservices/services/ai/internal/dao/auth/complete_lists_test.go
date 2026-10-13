package auth

import (
	"context"
	"fmt"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/shared/pkg/model"
	"testing"
	"time"
)

func TestCompleteSetUnusedRecoveryCodes(t *testing.T) {
	db := testdb.Open(t, &model.TOTPRecoveryCode{})
	const n = fullread.BatchSize*2 + 1
	rows := make([]model.TOTPRecoveryCode, n)
	for i := range rows {
		rows[i] = model.TOTPRecoveryCode{UserID: 7, CodeHash: fmt.Sprintf("fixture-hash-%d", i)}
	}
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, r := range []model.TOTPRecoveryCode{{UserID: 8, CodeHash: "other"}, {UserID: 7, CodeHash: "used", UsedAt: &now}} {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	d := NewUserDAO(db)
	testdb.Batches(t, db, func() {
		got, err := d.ListUnusedTOTPRecoveryCodesContext(context.Background(), 7)
		if err != nil || len(got) != n {
			t.Fatalf("codes=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.ID != rows[i].ID || r.CodeHash != rows[i].CodeHash || r.UsedAt != nil || r.UserID != 7 {
				t.Fatalf("code %d: %+v", i, r)
			}
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := d.ListUnusedTOTPRecoveryCodesContext(context.Background(), 7)
		if err == nil || got != nil {
			t.Fatalf("partial codes=%d err=%v", len(got), err)
		}
	})
}
