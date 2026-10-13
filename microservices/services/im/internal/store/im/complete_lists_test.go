package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-admin-kit/services/im/internal/model"
	"github.com/go-admin-kit/services/im/internal/sensitive"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
)

func TestCompleteSetSensitiveDictionaryProtectsBeyondBatchBoundary(t *testing.T) {
	db := testdb.Open(t, &model.SensitiveWord{})
	s := &Store{db: db}
	const count = fullread.BatchSize*2 + 1
	words := make([]model.SensitiveWord, 0, count)
	for i := 1; i <= count; i++ {
		words = append(words, model.SensitiveWord{TenantID: 7, Word: fmt.Sprintf("词%06d尾", i), Status: 1, Action: "mask"})
	}
	words[count-1].Action = "block"
	if err := db.CreateInBatches(&words, fullread.BatchSize).Error; err != nil {
		t.Fatal(err)
	}
	for _, word := range []model.SensitiveWord{
		{TenantID: 8, Word: "other-only", Status: 1, Action: "block"},
		{TenantID: 7, Word: "disabled-only", Status: 2, Action: "block"},
	} {
		if err := db.Create(&word).Error; err != nil {
			t.Fatal(err)
		}
	}
	cache := sensitive.NewCache(s)
	testdb.Batches(t, db, func() {
		_, blocked, hits := cache.Apply(7, words[count-1].Word)
		if !blocked || len(hits) != 1 {
			t.Fatalf("tail block word lost: %t %+v", blocked, hits)
		}
	})
	if text, blocked, hits := cache.Apply(7, words[fullread.BatchSize].Word); blocked || len(hits) != 1 || text == words[fullread.BatchSize].Word {
		t.Fatalf("tail mask word lost: %q %t %+v", text, blocked, hits)
	}
	for _, text := range []string{"other-only", "disabled-only"} {
		if _, blocked, hits := cache.Apply(7, text); blocked || len(hits) != 0 {
			t.Fatalf("unexpected protected set member: %s", text)
		}
	}
	// A later batch failure must not return/cache only the first page of words.
	queries := 0
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_second_batch", func(q *gorm.DB) {
		if !q.DryRun {
			queries++
			if queries == 2 {
				q.AddError(errors.New("injected second-batch failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("test:fail_second_batch")
	got, err := s.ListEnabledSensitiveWords(7)
	if err == nil || got != nil {
		t.Fatalf("returned partial dictionary: %d %v", len(got), err)
	}
	// Existing valid cache remains intact; this change introduces no size-limit error.
	if _, blocked, _ := cache.Apply(7, words[count-1].Word); !blocked {
		t.Fatal("valid cached tail protection lost")
	}
}
