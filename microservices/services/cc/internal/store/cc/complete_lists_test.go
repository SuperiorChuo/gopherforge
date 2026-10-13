package store

import (
	"context"
	"fmt"
	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"testing"
	"time"
)

func TestCompleteSetAgentsRatesAndReportInputs(t *testing.T) {
	db := testdb.Open(t, &Agent{}, &CCRate{}, &Call{})
	const n = fullread.BatchSize*2 + 1
	agents := make([]Agent, n)
	rates := make([]CCRate, n)
	calls := make([]Call, n)
	start := time.Now().UTC().Truncate(time.Hour)
	answered := start.Add(5 * time.Second)
	for i := range agents {
		agents[i] = Agent{TenantID: 7, UserID: uint64(i + 1), Extension: fmt.Sprintf("%04d", n-i)}
		rates[i] = CCRate{TenantID: 7, Prefix: fmt.Sprint(i + 1)}
		calls[i] = Call{TenantID: 7, CallID: fmt.Sprint(i), Direction: "inbound", CreatedAt: start, StartedAt: &start, AnsweredAt: &answered, DurationSec: 60, AgentExt: "1001", Queue: "fixture"}
	}
	// Null started_at must still enter the same report via created_at.
	calls[n-1].StartedAt = nil
	for _, r := range []any{&agents, &rates, &calls} {
		if err := db.CreateInBatches(r, 128).Error; err != nil {
			t.Fatal(err)
		}
	}
	excludedTime := start.Add(2 * time.Hour)
	for _, r := range []any{&Agent{TenantID: 8, UserID: 1, Extension: "foreign"}, &CCRate{TenantID: 8, Prefix: "foreign"}, &Call{TenantID: 8, CallID: "foreign", CreatedAt: start}, &Call{TenantID: 7, CallID: "outside", CreatedAt: excludedTime}} {
		if err := db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Store{db: db}
	ctx := context.Background()
	testdb.Batches(t, db, func() {
		got, err := s.ListAgents(ctx, 7)
		if err != nil || len(got) != n {
			t.Fatalf("agents=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.Extension != fmt.Sprintf("%04d", i+1) || r.TenantID != 7 {
				t.Fatalf("agent %d: %+v", i, r)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := s.ListRates(ctx, 7)
		if err != nil || len(got) != n {
			t.Fatalf("rates=%d err=%v", len(got), err)
		}
		for i, r := range got {
			if r.TenantID != 7 {
				t.Fatal("foreign rate")
			}
			if i > 0 {
				p := got[i-1].Prefix
				if len(p) < len(r.Prefix) || (len(p) == len(r.Prefix) && p > r.Prefix) {
					t.Fatalf("wrong longest-prefix order %q %q", p, r.Prefix)
				}
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := s.ReportOverview(ctx, 7, start, start.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != n || got.Answered != n || got.TotalInbound != n || got.AvgTalkSec != 60 || got.AvgWaitSec != 5 || len(got.AgentLeaderboard) != 1 || got.AgentLeaderboard[0].Answered != n || len(got.QueueLeaderboard) != 1 || got.QueueLeaderboard[0].Entered != n {
			t.Fatalf("incomplete report: %+v", got)
		}
	})
	for _, read := range []func(){
		func() {
			got, err := s.ListAgents(ctx, 7)
			if err == nil || got != nil {
				t.Fatalf("partial agents=%d err=%v", len(got), err)
			}
		},
		func() {
			got, err := s.ListRates(ctx, 7)
			if err == nil || got != nil {
				t.Fatalf("partial rates=%d err=%v", len(got), err)
			}
		},
		func() {
			got, err := s.ReportOverview(ctx, 7, start, start.Add(time.Hour))
			if err == nil || got != nil {
				t.Fatalf("partial report=%+v err=%v", got, err)
			}
		},
	} {
		testdb.FailLater(t, db, read)
	}
}
