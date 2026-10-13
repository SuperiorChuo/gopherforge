package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"github.com/go-admin-kit/services/visibility/internal/model"
)

func TestCompleteSetTrackingGeoResultsAndRankHistory(t *testing.T) {
	db := testdb.Open(t, &model.Keyword{}, &model.GeoResult{}, &model.RankSnapshot{})
	s := &Store{db: db}
	const count = fullread.BatchSize*2 + 1
	keywords := make([]model.Keyword, 0, count)
	results := make([]model.GeoResult, 0, count)
	ranks := make([]model.RankSnapshot, 0, count)
	for i := 1; i <= count; i++ {
		keywords = append(keywords, model.Keyword{TenantID: 7, Keyword: fmt.Sprint(i), Status: "tracking"})
		results = append(results, model.GeoResult{TenantID: 7, RunID: 42, Question: "q", SourcesJSON: `[{"position":1,"url":"https://example.com","cited":true}]`})
		ranks = append(ranks, model.RankSnapshot{TenantID: 7, KeywordID: 42, Engine: "bing", Rank: i, CreatedAt: time.Now().Add(-time.Hour)})
	}
	for _, rows := range []any{&keywords, &results, &ranks} {
		if err := db.CreateInBatches(rows, fullread.BatchSize).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []any{
		&model.Keyword{TenantID: 8, Keyword: "other", Status: "tracking"}, &model.Keyword{TenantID: 7, Keyword: "paused", Status: "paused"},
		&model.GeoResult{TenantID: 8, RunID: 42, Question: "other"}, &model.GeoResult{TenantID: 7, RunID: 43, Question: "other-run"},
		&model.RankSnapshot{TenantID: 8, KeywordID: 42, Engine: "bing"}, &model.RankSnapshot{TenantID: 7, KeywordID: 43, Engine: "bing"},
		&model.RankSnapshot{TenantID: 7, KeywordID: 42, Engine: "google"}, &model.RankSnapshot{TenantID: 7, KeywordID: 42, Engine: "bing", CreatedAt: time.Now().AddDate(0, 0, -40)},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	testdb.Batches(t, db, func() {
		got, err := s.ListTrackingKeywords(7, 0)
		if err != nil || len(got) != count {
			t.Fatalf("keywords=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(i+1) || row.TenantID != 7 || row.Status != "tracking" {
				t.Fatalf("keyword %d: %+v", i, row)
			}
		}
	})
	one, err := s.ListTrackingKeywords(7, uint64(count))
	if err != nil || len(one) != 1 || one[0].ID != uint64(count) {
		t.Fatalf("single keyword: %+v %v", one, err)
	}
	testdb.Batches(t, db, func() {
		got, err := s.ListGeoResults(7, 42)
		if err != nil || len(got) != count {
			t.Fatalf("results=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(i+1) || row.TenantID != 7 || row.RunID != 42 || len(row.Sources) != 1 || !row.Sources[0].Cited || row.Sources[0].URL != "https://example.com" {
				t.Fatalf("result %d: %+v", i, row)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := s.ListRankSnapshots(7, 42, "bing", 30)
		if err != nil || len(got) != count {
			t.Fatalf("history=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(i+1) || row.TenantID != 7 || row.KeywordID != 42 || row.Engine != "bing" {
				t.Fatalf("snapshot %d: %+v", i, row)
			}
		}
	})
}
