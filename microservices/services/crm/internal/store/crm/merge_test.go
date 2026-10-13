package store

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
)

func TestFindDuplicateGroupsUsesBoundedCandidatesAndPreservesPrecedence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithDB(db)
	if err != nil {
		t.Fatal(err)
	}
	customers := []model.Customer{
		{TenantID: 1, Name: "宏 图", Phone: "+86 138-0000-1111"},
		{TenantID: 1, Name: "宏图", Phone: "13800001111"},
		{TenantID: 1, Name: "宏图", Phone: "13900002222"},
		{TenantID: 1, Name: "宏图", Phone: "13700003333"},
		{TenantID: 1, Name: "", Phone: "12345"},
		{TenantID: 2, Name: "宏图", Phone: "13800001111"},
	}
	if err := db.Create(&customers).Error; err != nil {
		t.Fatal(err)
	}

	groups, err := s.FindDuplicateGroups(1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %#v, want phone + name", groups)
	}
	if groups[0].Kind != "phone" || groups[0].Key != "phone:13800001111" || len(groups[0].Customers) != 2 {
		t.Fatalf("phone group = %#v", groups[0])
	}
	if groups[1].Kind != "name" || groups[1].Key != "name:宏图" || len(groups[1].Customers) != 4 {
		t.Fatalf("name group = %#v", groups[1])
	}
	for _, group := range groups {
		for _, customer := range group.Customers {
			if customer.TenantID != 1 {
				t.Fatalf("cross-tenant customer leaked: %#v", customer)
			}
		}
	}

	limited, err := s.FindDuplicateGroups(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].Kind != "phone" {
		t.Fatalf("limited groups = %#v", limited)
	}
}

func TestNormalizeMergeSourceIDs(t *testing.T) {
	got, err := NormalizeMergeSourceIDs(1, []uint64{2, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("normalized = %v", got)
	}
	if _, err := NormalizeMergeSourceIDs(1, []uint64{1}); err == nil {
		t.Fatal("primary id in source ids must be rejected")
	}
	if _, err := NormalizeMergeSourceIDs(1, []uint64{0}); err == nil {
		t.Fatal("zero source id must be rejected")
	}
	tooMany := make([]uint64, MaxMergeSourceIDs+1)
	if _, err := NormalizeMergeSourceIDs(1, tooMany); err == nil {
		t.Fatal("oversized source ids must be rejected")
	}
}
