package ai

import (
	"context"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"testing"
)

func TestCompleteSetDocumentChunks(t *testing.T) {
	db := testdb.Open(t, &localmodel.AIDocumentChunk{})
	const n = fullread.BatchSize*2 + 1
	rows := make([]localmodel.AIDocumentChunk, n)
	for i := range rows {
		rows[i] = localmodel.AIDocumentChunk{DocumentID: 7, ChunkIndex: n - i, Content: fmt.Sprint(n - i)}
	}
	if err := db.CreateInBatches(&rows, 128).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&localmodel.AIDocumentChunk{DocumentID: 8, Content: "excluded"}).Error; err != nil {
		t.Fatal(err)
	}
	d := NewDocumentDAO(db)
	testdb.Batches(t, db, func() {
		got, err := d.ListChunksContext(context.Background(), 7)
		if err != nil || len(got) != n {
			t.Fatalf("chunks=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.DocumentID != 7 || r.ChunkIndex != i+1 || r.Content != fmt.Sprint(i+1) {
				t.Fatalf("chunk %d: %+v", i, r)
			}
		}
	})
	testdb.FailLater(t, db, func() {
		got, err := d.ListChunksContext(context.Background(), 7)
		if err == nil || got != nil {
			t.Fatalf("partial chunks=%d err=%v", len(got), err)
		}
	})
}
