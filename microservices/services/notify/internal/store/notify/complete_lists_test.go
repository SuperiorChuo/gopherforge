package store

import (
	"fmt"
	"testing"

	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/notify/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
)

func TestCompleteSetTenantAndSystemTemplates(t *testing.T) {
	db := testdb.Open(t, &model.Template{})
	s := &Store{db: db}
	const count = fullread.BatchSize*2 + 1
	templates := make([]model.Template, 0, count)
	for i := 1; i <= count; i++ {
		tid := uint64(7)
		if i%2 == 0 {
			tid = 1
		}
		templates = append(templates, model.Template{TenantID: tid, Code: fmt.Sprint(i), TitleTpl: "title", BodyTpl: "body"})
	}
	if err := db.CreateInBatches(&templates, fullread.BatchSize).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Template{TenantID: 8, Code: "other", TitleTpl: "title", BodyTpl: "body"}).Error; err != nil {
		t.Fatal(err)
	}
	testdb.Batches(t, db, func() {
		got, err := s.ListTemplates(7)
		if err != nil || len(got) != count {
			t.Fatalf("templates=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(i+1) || (row.TenantID != 7 && row.TenantID != 1) {
				t.Fatalf("template %d: %+v", i, row)
			}
		}
	})
	got, err := s.ListTemplates(0)
	if err != nil || len(got) != count/2 {
		t.Fatalf("system templates=%d %v", len(got), err)
	}
	for _, row := range got {
		if row.TenantID != 1 {
			t.Fatalf("system template leaked: %+v", row)
		}
	}
}
