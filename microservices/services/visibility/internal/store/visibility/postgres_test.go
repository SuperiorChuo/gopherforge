package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The audit plugin uses public.outbox_events; a private schema in a business
// database is NOT sufficient isolation. Only opt-in, disposable databases.
func postgresFixture(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("VISIBILITY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires isolated VISIBILITY_TEST_POSTGRES_DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "visibility_regression_") {
		t.Fatal("VISIBILITY_TEST_POSTGRES_DSN must be a PostgreSQL URL for a visibility_regression_ database")
	}
	cfg := &gorm.Config{PrepareStmt: true, Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), cfg)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var database string
	if err := admin.Raw("SELECT current_database()").Scan(&database).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "visibility_regression_") {
		t.Fatal("refusing non-regression database")
	}
	schema := fmt.Sprintf("visibility_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), cfg)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	st, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := db.Raw("SELECT to_regclass('public.outbox_events') IS NOT NULL").Scan(&exists).Error; err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("refusing to take over an existing public.outbox_events table")
	}
	if err := outbox.EnsureTable(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP TABLE public.outbox_events").Error; err != nil {
			t.Error(err)
		}
	})
	enabled := outbox.TransactionalEnabled()
	t.Cleanup(func() {
		if !enabled {
			outbox.DisableTransactional()
		}
	})
	if err := sharedaudit.Register(db, sharedaudit.Config{Targets: []sharedaudit.Target{
		{Model: &model.Brand{}, Table: "vis_brands", TargetType: "vis_brand", TenantField: "tenant_id", SnapshotFields: []string{"ID", "TenantID", "Name", "Aliases", "Kind", "Website", "Note", "Status", "CreatedAt", "UpdatedAt"}},
		{Model: &model.Keyword{}, Table: "vis_keywords", TargetType: "vis_keyword", TenantField: "tenant_id", SnapshotFields: []string{"ID", "TenantID", "Keyword", "Locale", "TargetURL", "Status", "Note", "CreatedAt", "UpdatedAt"}},
		{Model: &model.GeoPrompt{}, Table: "vis_geo_prompts", TargetType: "vis_geo_prompt", TenantField: "tenant_id", SnapshotFields: []string{"ID", "TenantID", "Category", "Question", "Engine", "Status", "CreatedAt", "UpdatedAt"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if !outbox.TransactionalEnabled() {
		t.Fatal("production transactional audit not enabled")
	}
	return st
}

func TestPostgresBatchAtomicityAndAudit(t *testing.T) {
	if os.Getenv("VISIBILITY_TEST_POSTGRES_DSN") == "" {
		t.Skip("requires isolated VISIBILITY_TEST_POSTGRES_DSN")
	}
	for _, tc := range batchCases() {
		t.Run(tc.table, func(t *testing.T) {
			st := postgresFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			ctx = sharedaudit.WithTenantID(sharedaudit.WithActor(ctx, "operator", "20"), 2)
			assertBatchWriteRollback(t, st, tc, ctx)
			assertRows(t, st, "public.outbox_events", 0)
			// A real server-side column rejection also rolls back the first row/audit.
			var err error
			switch tc.table {
			case "vis_brands":
				_, _, err = st.BatchCreateBrands(ctx, 2, []model.Brand{{Name: "valid"}, {Name: strings.Repeat("工", 129)}})
			case "vis_keywords":
				_, _, err = st.BatchCreateKeywords(ctx, 2, []model.Keyword{{Keyword: "valid"}, {Keyword: strings.Repeat("词", 257)}})
			case "vis_geo_prompts":
				_, _, err = st.BatchCreateGeoPrompts(ctx, 2, []model.GeoPrompt{{Question: "valid"}, {Question: "invalid category", Category: strings.Repeat("类", 33)}})
			}
			if err == nil || !strings.Contains(err.Error(), "22001") {
				t.Fatalf("server column failure not exercised: %v", err)
			}
			assertRows(t, st, tc.table, 0)
			assertRows(t, st, "public.outbox_events", 0)
			sentinel := errors.New("injected second audit failure")
			hits := 0
			if err := st.db.Callback().Create().Before("gorm:create").Register("test:fail_audit", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.Table, "outbox_events") {
					hits++
					if hits == 2 {
						tx.AddError(sentinel)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			n, k, err := tc.call(st, ctx, 2)
			if !errors.Is(err, sentinel) || hits != 2 || n != 0 || k != 0 {
				t.Fatalf("audit failure not exercised: hits=%d created=%d skipped=%d err=%v", hits, n, k, err)
			}
			assertRows(t, st, tc.table, 0)
			assertRows(t, st, "public.outbox_events", 0)
			if err := st.db.Callback().Create().Remove("test:fail_audit"); err != nil {
				t.Fatal(err)
			}
			if n, k, err := tc.call(st, ctx, 2); err != nil || n != 2 || k != 0 {
				t.Fatalf("success created=%d skipped=%d err=%v", n, k, err)
			}
			if n, k, err := tc.call(st, ctx, 2); err != nil || n != 0 || k != 2 {
				t.Fatalf("repeat created=%d skipped=%d err=%v", n, k, err)
			}
			assertRows(t, st, tc.table, 2)
			assertRows(t, st, "public.outbox_events", 2)
			var jobs []outbox.Event
			if err := st.db.Table("public.outbox_events").Order("id ASC").Limit(3).Find(&jobs).Error; err != nil {
				t.Fatal(err)
			}
			target := map[string]string{"vis_brands": "vis_brand", "vis_keywords": "vis_keyword", "vis_geo_prompts": "vis_geo_prompt"}[tc.table]
			field := map[string]string{"vis_brands": "name", "vis_keywords": "keyword", "vis_geo_prompts": "question"}[tc.table]
			for i, job := range jobs {
				var event auditevents.AuditEvent
				if err := json.Unmarshal(job.Payload, &event); err != nil {
					t.Fatal(err)
				}
				if job.Subject != "audit.log.create" || event.TenantID != 2 || event.ActorID != "20" || event.TargetType != target || event.TargetID == "" || event.After[field] != []string{"Alpha", "Beta"}[i] || event.After["tenant_id"] != float64(2) {
					t.Fatalf("audit identity/snapshot=%+v", event)
				}
			}
			otherCtx := sharedaudit.WithTenantID(ctx, 3)
			if n, k, err := tc.call(st, otherCtx, 3); err != nil || n != 2 || k != 0 {
				t.Fatalf("cross-tenant dedupe: created=%d skipped=%d err=%v", n, k, err)
			}
			assertRows(t, st, tc.table, 4)
			assertRows(t, st, "public.outbox_events", 4)
		})
	}
}
