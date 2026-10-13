package store

import (
	"fmt"
	"testing"

	"github.com/go-admin-kit/services/crm/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
)

func TestCompleteSetDuplicateMembersPoolRulesAndMergeParticipants(t *testing.T) {
	db := testdb.Open(t, &model.Customer{}, &model.PoolRule{})
	s := &Store{db: db}
	const count = fullread.BatchSize*2 + 1
	customers := make([]model.Customer, 0, count)
	rules := make([]model.PoolRule, 0, count)
	for i := 1; i <= count; i++ {
		customers = append(customers, model.Customer{TenantID: 7, Name: fmt.Sprint(i), Phone: "13800001111", OwnerUserID: uint64(i)})
		// Persisted legacy levels have no four-value DB CHECK; do not truncate them.
		rules = append(rules, model.PoolRule{TenantID: 7, Level: fmt.Sprintf("Z%06d", i)})
	}
	for _, rows := range []any{&customers, &rules} {
		if err := db.CreateInBatches(rows, fullread.BatchSize).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.Customer{TenantID: 8, Name: "other", Phone: "13800001111"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PoolRule{TenantID: 8, Level: "A"}).Error; err != nil {
		t.Fatal(err)
	}
	testdb.Batches(t, db, func() {
		groups, err := s.FindDuplicateGroups(7, 1)
		if err != nil || len(groups) != 1 || len(groups[0].Customers) != count {
			t.Fatalf("groups=%d %v", len(groups), err)
		}
		for i, c := range groups[0].Customers {
			if c.ID != uint64(i+1) || c.TenantID != 7 || c.OwnerUserID != uint64(i+1) {
				t.Fatalf("customer %d: %+v", i, c)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := s.GetPoolRules(7)
		if err != nil || len(got) != count+1 || got[0].Level != "*" {
			t.Fatalf("rules=%d %v", len(got), err)
		}
		for i, r := range got[1:] {
			if r.Level != fmt.Sprintf("Z%06d", i+1) || r.TenantID != 7 {
				t.Fatalf("rule %d: %+v", i, r)
			}
		}
	})
	ids := make([]uint64, MaxMergeSourceIDs+1)
	for i := range ids {
		ids[i] = uint64(i + 1)
	}
	got, err := s.GetCustomersByIDs(7, ids)
	if err != nil || len(got) != len(ids) {
		t.Fatalf("merge participants=%d %v", len(got), err)
	}
	for i, c := range got {
		if c.ID != ids[i] || c.OwnerUserID != ids[i] {
			t.Fatalf("merge scope participant %d: %+v", i, c)
		}
	}
	duplicates, err := s.GetCustomersByIDs(7, []uint64{1, 1, uint64(count + 1), 99999})
	if err != nil || len(duplicates) != 1 || duplicates[0].ID != 1 {
		t.Fatalf("duplicate/missing/cross-tenant IDs: %+v %v", duplicates, err)
	}
}
