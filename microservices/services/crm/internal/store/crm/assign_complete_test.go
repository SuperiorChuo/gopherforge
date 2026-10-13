package store

import (
	"context"
	"fmt"
	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"testing"
)

func TestCompleteSetAssignLeadsInputCardinality(t *testing.T) {
	db := testdb.Open(t, &model.Lead{}, &model.AssignLog{})
	const n = fullread.BatchSize*2 + 1
	rows := make([]model.Lead, n)
	ids := make([]uint64, 0, n+4)
	for i := range rows {
		rows[i] = model.Lead{TenantID: 7, Name: fmt.Sprint(i), Status: "new", OwnerUserID: 2}
	}
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	excluded := []model.Lead{{TenantID: 8, Name: "foreign", Status: "new"}, {TenantID: 7, Name: "converted", Status: "converted"}, {TenantID: 7, Name: "not-selected", Status: "new"}}
	if err := db.Create(&excluded).Error; err != nil {
		t.Fatal(err)
	}
	ids = append(ids, rows[0].ID, excluded[0].ID, excluded[1].ID, 9999999)
	s := &Store{db: db}
	got, err := s.AssignLeads(context.Background(), 7, ids, 3, 9, 11)
	if err != nil || got != n {
		t.Fatalf("assigned=%d err=%v", got, err)
	}
	for _, r := range rows {
		var l model.Lead
		if err := db.First(&l, r.ID).Error; err != nil {
			t.Fatal(err)
		}
		if l.OwnerUserID != 9 || l.DepartmentID != 3 || l.TenantID != 7 {
			t.Fatalf("lead=%+v", l)
		}
	}
	for _, r := range excluded {
		var l model.Lead
		if err := db.First(&l, r.ID).Error; err != nil {
			t.Fatal(err)
		}
		if l.OwnerUserID != 0 || l.DepartmentID != 0 || l.Status != r.Status {
			t.Fatalf("excluded lead changed: %+v", l)
		}
	}
	var logs int64
	if err := db.Model(&model.AssignLog{}).Where("tenant_id = ? AND target_type = ? AND from_owner = ? AND to_owner = ? AND to_dept = ? AND by_user_id = ?", 7, "lead", 2, 9, 3, 11).Count(&logs).Error; err != nil || logs != n {
		t.Fatalf("assignment logs=%d err=%v", logs, err)
	}
	if _, err := s.AssignLeads(context.Background(), 7, nil, 3, 9, 11); err == nil {
		t.Fatal("empty input accepted")
	}
}
