package store

import (
	"fmt"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/ticket/internal/model"
	"testing"
)

func TestCompleteSetTicketEvents(t *testing.T) {
	db := testdb.Open(t, &model.TicketEvent{})
	const n = fullread.BatchSize*2 + 1
	rows := make([]model.TicketEvent, n)
	for i := range rows {
		rows[i] = model.TicketEvent{TenantID: 7, TicketID: 9, Kind: "comment", Content: fmt.Sprint(i)}
	}
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range []model.TicketEvent{{TenantID: 8, TicketID: 9, Kind: "comment"}, {TenantID: 7, TicketID: 10, Kind: "comment"}} {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Store{db: db}
	testdb.Batches(t, db, func() {
		got, err := s.ListEvents(9, 7)
		if err != nil || len(got) != n {
			t.Fatalf("events=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.ID != rows[i].ID || r.Content != fmt.Sprint(i) || r.TenantID != 7 || r.TicketID != 9 {
				t.Fatalf("event %d: %+v", i, r)
			}
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := s.ListEvents(9, 7)
		if err == nil || got != nil {
			t.Fatalf("partial events=%d err=%v", len(got), err)
		}
	})
}
