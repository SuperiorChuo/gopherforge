package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/auditevents"
	sharedaudit "github.com/go-admin-kit/services/shared/pkg/audittrail"
	"github.com/go-admin-kit/services/shared/pkg/outbox"
	"github.com/go-admin-kit/services/visibility/internal/model"
	"gorm.io/gorm"
)

func TestBatchHTTPAuditIdentityAndRollback(t *testing.T) {
	for _, tc := range []struct {
		path, table, target, field, body string
		model                            any
		fields                           []string
	}{
		{"/brands/batch", "vis_brands", "vis_brand", "name", `{"items":[{"name":"Alpha"},{"name":"Beta"}]}`, &model.Brand{}, []string{"ID", "TenantID", "Name", "Aliases", "Kind", "Website", "Note", "Status", "CreatedAt", "UpdatedAt"}},
		{"/keywords/batch", "vis_keywords", "vis_keyword", "keyword", `{"items":[{"keyword":"Alpha"},{"keyword":"Beta"}]}`, &model.Keyword{}, []string{"ID", "TenantID", "Keyword", "Locale", "TargetURL", "Status", "Note", "CreatedAt", "UpdatedAt"}},
		{"/geo/prompts/batch", "vis_geo_prompts", "vis_geo_prompt", "question", `{"items":[{"question":"Alpha"},{"question":"Beta"}]}`, &model.GeoPrompt{}, []string{"ID", "TenantID", "Category", "Question", "Engine", "Status", "CreatedAt", "UpdatedAt"}},
	} {
		for _, stage := range []string{"success", "audit failure"} {
			t.Run(tc.table+"/"+stage, func(t *testing.T) {
				s, _ := coreServer(t)
				db := s.Store.DB()
				if err := outbox.EnsureTable(db); err != nil {
					t.Fatal(err)
				}
				enabled := outbox.TransactionalEnabled()
				t.Cleanup(func() {
					if !enabled {
						outbox.DisableTransactional()
					}
				})
				if err := sharedaudit.Register(db, sharedaudit.Config{Targets: []sharedaudit.Target{{Model: tc.model, Table: tc.table, TargetType: tc.target, TenantField: "tenant_id", SnapshotFields: tc.fields}}}); err != nil {
					t.Fatal(err)
				}
				r := gin.New()
				r.Use(sharedaudit.AuditHeaderMiddleware())
				s.RegisterRoutes(r)
				hits := 0
				if stage == "audit failure" {
					if err := db.Callback().Create().Before("gorm:create").Register("test:http_audit", func(tx *gorm.DB) {
						if strings.Contains(tx.Statement.Table, "outbox_events") {
							hits++
							if hits == 2 {
								tx.AddError(errors.New("second HTTP audit failed"))
							}
						}
					}); err != nil {
						t.Fatal(err)
					}
				}
				want := 200
				if stage == "audit failure" {
					want = 500
				}
				coreRequest(t, r, "POST", tc.path, tc.body, 2, "visibility:write", want)
				var count int64
				if err := db.Table(tc.table).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				var events []outbox.Event
				if err := db.Order("id ASC").Limit(3).Find(&events).Error; err != nil {
					t.Fatal(err)
				}
				if stage == "audit failure" {
					if hits != 2 || count != 0 || len(events) != 0 {
						t.Fatalf("HTTP partial audit: hits=%d rows=%d events=%d", hits, count, len(events))
					}
					return
				}
				if count != 2 || len(events) != 2 {
					t.Fatalf("HTTP audit missing: rows=%d events=%d", count, len(events))
				}
				for i, event := range events {
					var audit auditevents.AuditEvent
					if err := json.Unmarshal(event.Payload, &audit); err != nil {
						t.Fatal(err)
					}
					if audit.ActorID != "10" || audit.TenantID != 2 || audit.TargetType != tc.target || audit.TargetID == "" || audit.After[tc.field] != []string{"Alpha", "Beta"}[i] || audit.After["tenant_id"] != float64(2) {
						t.Fatalf("HTTP identity/snapshot: %+v", audit)
					}
				}
			})
		}
	}
}
