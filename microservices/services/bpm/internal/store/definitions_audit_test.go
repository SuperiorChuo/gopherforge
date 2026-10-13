package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/audittrail"
	"gorm.io/gorm"
)

type publishAuditRecord struct {
	ID         uint `gorm:"primaryKey"`
	TenantID   uint
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	BeforeJSON map[string]any `gorm:"serializer:json"`
	AfterJSON  map[string]any `gorm:"serializer:json"`
	Summary    string
	CreatedAt  time.Time
}

func (publishAuditRecord) TableName() string { return "audit_logs" }

func publishAuditFixture(t *testing.T, active int) (*Store, context.Context, model.ProcessDefinition, []model.ProcessDefinition) {
	t.Helper()
	db := testdb.Open(t, &publishAuditRecord{})
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	tree := model.JSONB(`{"version":1,"start":{"id":"start","name":"Start","type":"start","next":{"id":"approve","name":"Approve","type":"approval","multiMode":"OR","assignee":{"type":"users","userIds":[1]}}}}`)
	rows := make([]model.ProcessDefinition, 0, active+5)
	for i := 0; i < active; i++ {
		rows = append(rows, model.ProcessDefinition{TenantID: 1, Key: "release", Name: "Prior", Version: i + 1, Status: model.DefActive, NodeTree: tree})
	}
	rows = append(rows,
		model.ProcessDefinition{TenantID: 1, Key: "release", Name: "Publish", Version: active + 1, Status: model.DefDraft, NodeTree: tree},
		model.ProcessDefinition{TenantID: 2, Key: "release", Name: "Other tenant", Version: 1, Status: model.DefActive, NodeTree: tree},
		model.ProcessDefinition{TenantID: 1, Key: "other", Name: "Other key", Version: 1, Status: model.DefActive, NodeTree: tree},
		model.ProcessDefinition{TenantID: 1, Key: "release", Name: "Archived", Version: active + 2, Status: model.DefArchived, NodeTree: tree},
		model.ProcessDefinition{TenantID: 1, Key: "release", Name: "Other draft", Version: active + 3, Status: model.DefDraft, NodeTree: tree},
	)
	if err := db.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	// The same single-row target and default MaxRows=1 used by bpm/cmd/main.go.
	if err := audittrail.Register(db, audittrail.Config{Targets: []audittrail.Target{
		{Model: &model.ProcessDefinition{}, Table: "bpm_process_definition", TargetType: "bpm_definition", TenantField: "tenant_id",
			SnapshotFields: []string{"ID", "TenantID", "Key", "Name", "Version", "Status", "BizType", "Remark"}},
	}}); err != nil {
		t.Fatal(err)
	}
	ctx := audittrail.WithTenantID(audittrail.WithActor(context.Background(), "user", "71"), 1)
	return s, ctx, rows[active], rows
}

func assertPublishState(t *testing.T, s *Store, original []model.ProcessDefinition, active int, published bool) {
	t.Helper()
	for i, before := range original {
		var got model.ProcessDefinition
		if err := s.DB().First(&got, before.ID).Error; err != nil {
			t.Fatal(err)
		}
		want := before.Status
		if published && i < active {
			want = model.DefArchived
		} else if published && i == active {
			want = model.DefActive
		}
		if got.Status != want {
			t.Fatalf("id=%d tenant=%d key=%s status=%s, want %s", got.ID, got.TenantID, got.Key, got.Status, want)
		}
	}
}

func TestPublishWithSingleRowAudit(t *testing.T) {
	for _, active := range []int{0, 1, 513} {
		t.Run(fmt.Sprintf("active_%d", active), func(t *testing.T) {
			s, ctx, draft, original := publishAuditFixture(t, active)
			publish := func() {
				got, err := s.Publish(ctx, draft.ID, 1)
				if err != nil {
					t.Fatal(err)
				}
				if got.ID != draft.ID || got.Status != model.DefActive {
					t.Fatalf("wrong published definition: %+v", got)
				}
			}
			if active > 256 {
				testdb.Batches(t, s.DB(), publish)
			} else {
				publish()
			}
			assertPublishState(t, s, original, active, true)
			var count int64
			if err := s.DB().Model(&publishAuditRecord{}).Count(&count).Error; err != nil || count != int64(active+1) {
				t.Fatalf("audit count=%d, want %d: %v", count, active+1, err)
			}
			for i := 0; i <= active; i++ {
				var record publishAuditRecord
				if err := s.DB().Where("target_id = ?", strconv.FormatUint(original[i].ID, 10)).First(&record).Error; err != nil {
					t.Fatal(err)
				}
				from, to := model.DefActive, model.DefArchived
				if i == active {
					from, to = model.DefDraft, model.DefActive
				}
				if record.TenantID != 1 || record.ActorID != "71" || record.ActorType != "user" || record.Action != "update" || record.TargetType != "bpm_definition" || record.BeforeJSON["status"] != from || record.AfterJSON["status"] != to {
					t.Fatalf("bad audit attribution/snapshot: %+v", record)
				}
			}
		})
	}
}

func TestPublishAuditRollsBackAllChanges(t *testing.T) {
	for _, failAt := range []int{2, 4} { // Later archive, then final draft activation.
		t.Run(fmt.Sprintf("write_%d", failAt), func(t *testing.T) {
			s, ctx, draft, original := publishAuditFixture(t, 3)
			injected := errors.New("injected publish write failure")
			updates := 0
			const name = "test:publish_write_failure"
			if err := s.DB().Callback().Update().Before("gorm:update").Register(name, func(q *gorm.DB) {
				if q.Statement.Table == "bpm_process_definition" {
					updates++
					if updates == failAt {
						q.AddError(injected)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer s.DB().Callback().Update().Remove(name)
			got, err := s.Publish(ctx, draft.ID, 1)
			if got != nil || !errors.Is(err, injected) || updates != failAt {
				t.Fatalf("got=%v err=%v writes=%d, want rollback at %d", got, err, updates, failAt)
			}
			assertPublishState(t, s, original, 3, false)
			var count int64
			if err := s.DB().Model(&publishAuditRecord{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial audit records=%d: %v", count, err)
			}
		})
	}
}

func TestPublishLaterReadFailureDoesNotArchive(t *testing.T) {
	s, ctx, draft, original := publishAuditFixture(t, 513)
	testdb.FailLater(t, s.DB(), func() {
		got, err := s.Publish(ctx, draft.ID, 1)
		if got != nil || err == nil {
			t.Fatalf("partial publish: got=%v err=%v", got, err)
		}
	})
	assertPublishState(t, s, original, 513, false)
	var count int64
	if err := s.DB().Model(&publishAuditRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial audit records=%d: %v", count, err)
	}
}
