package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"gorm.io/gorm"
)

func TestPostgresAuditCancellationRollsBack(t *testing.T) {
	if os.Getenv("VISIBILITY_TEST_POSTGRES_DSN") == "" {
		t.Skip("requires isolated VISIBILITY_TEST_POSTGRES_DSN")
	}
	for _, tc := range batchCases() {
		for _, stage := range []string{"snapshot", "outbox"} {
			t.Run(tc.table+"/"+stage, func(t *testing.T) {
				st := postgresFixture(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx = sharedaudit.WithTenantID(sharedaudit.WithActor(ctx, "operator", "20"), 2)
				businessWrites, auditWrites, hits := 0, 0, 0
				var targetErr error
				if err := st.db.Callback().Create().Before("gorm:create").Register("test:cancel_write", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table {
						businessWrites++
					}
					if strings.Contains(tx.Statement.Table, "outbox_events") {
						auditWrites++
						if stage == "outbox" && auditWrites == 2 {
							hits++
							cancel()
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := st.db.Callback().Create().After("gorm:create").Register("test:observe_write", func(tx *gorm.DB) {
					if stage == "outbox" && hits == 1 && strings.Contains(tx.Statement.Table, "outbox_events") {
						targetErr = tx.Error
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := st.db.Callback().Query().Before("gorm:query").Register("test:cancel_snapshot", func(tx *gorm.DB) {
					if stage == "snapshot" && businessWrites == 2 && tx.Statement.Table == tc.table {
						hits++
						cancel()
					}
				}); err != nil {
					t.Fatal(err)
				}
				if err := st.db.Callback().Query().After("gorm:query").Register("test:observe_snapshot", func(tx *gorm.DB) {
					if stage == "snapshot" && hits == 1 && tx.Statement.Table == tc.table {
						targetErr = tx.Error
					}
				}); err != nil {
					t.Fatal(err)
				}
				n, k, err := tc.call(st, ctx, 2)
				if !errors.Is(err, context.Canceled) || !errors.Is(targetErr, context.Canceled) || hits != 1 || businessWrites != 2 || n != 0 || k != 0 {
					t.Fatalf("audit cancellation missed target: stage=%s writes=%d hits=%d created=%d skipped=%d err=%v target=%v", stage, businessWrites, hits, n, k, err, targetErr)
				}
				for _, name := range []string{"test:cancel_write", "test:observe_write"} {
					if err := st.db.Callback().Create().Remove(name); err != nil {
						t.Fatal(err)
					}
				}
				for _, name := range []string{"test:cancel_snapshot", "test:observe_snapshot"} {
					if err := st.db.Callback().Query().Remove(name); err != nil {
						t.Fatal(err)
					}
				}
				assertRows(t, st, tc.table, 0)
				assertRows(t, st, "public.outbox_events", 0)
			})
		}
	}
}
