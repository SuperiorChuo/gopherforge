//go:build release_integration

package store

import (
	"crypto/rand"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/fullread"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const mediaTestDSN = "host=127.0.0.1 port=5432 user=postgres password=isolated-test-only dbname=release_fullread sslmode=disable"

func TestReleaseFullreadMediaConfigurationAndTargets(t *testing.T) {
	if os.Getenv("GAK_FULLREAD_TEST_DSN") != mediaTestDSN || os.Getenv("GAK_RELEASE_TEST_CONFIRM") != "isolated-release-database" {
		t.Fatal("requires the explicit isolated fullread database harness")
	}
	admin, err := gorm.Open(postgres.Open(mediaTestDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := admin.DB()
	t.Cleanup(func() { pool.Close() })
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("release_fullread_media_%x", nonce)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	s, err := Open(mediaTestDSN + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	mediaPool, _ := s.db.DB()
	t.Cleanup(func() { mediaPool.Close() })
	key := func(n int) string { return fmt.Sprintf("%06d", n) }
	verifyMediaList(t, s.db, func(n int) Extension { return Extension{Ext: key(n)} }, s.ListExtensions, func(r Extension) string { return r.Ext })
	verifyMediaList(t, s.db, func(n int) Gateway { return Gateway{Name: key(n), TenantID: 7} }, s.ListGateways, func(r Gateway) string { return r.Name })
	verifyMediaList(t, s.db, func(n int) CCQueue { return CCQueue{Name: key(n), AccessNumber: key(n)} }, s.ListCCQueues, func(r CCQueue) string { return r.Name })
	verifyMediaList(t, s.db, func(n int) CCQueueAgent {
		return CCQueueAgent{QueueName: "queue", Ext: key(n), Level: n % 3, Position: n}
	}, func() ([]CCQueueAgent, error) { return s.ListQueueAgents("queue") }, func(r CCQueueAgent) string { return key(r.Level) + "/" + key(r.Position) })
	// IDs are assigned in reverse number order, so id DESC yields number ASC.
	verifyMediaList(t, s.db, func(n int) BlacklistEntry { return BlacklistEntry{Number: key(n)} }, s.ListBlacklist, func(r BlacklistEntry) string { return r.Number })
	verifyMediaList(t, s.db, func(n int) IVRMenu {
		return IVRMenu{Name: key(n), AccessNumber: key(n), Entries: `[{"digits":"1","action":"menu-exec-app","target":"transfer 1000"}]`}
	}, s.ListIVRMenus, func(r IVRMenu) string { return r.AccessNumber })
	verifyMediaList(t, s.db, func(n int) CCConference { return CCConference{Name: key(n), Number: key(n)} }, s.ListConferences, func(r CCConference) string { return r.Number })
	verifyMediaList(t, s.db, func(n int) CCTimeRoute { return CCTimeRoute{Name: key(n), AccessNumber: key(n)} }, s.ListTimeRoutes, func(r CCTimeRoute) string { return r.AccessNumber })
	verifyMediaList(t, s.db, func(n int) CCRingGroup { return CCRingGroup{Name: key(n), AccessNumber: key(n)} }, s.ListRingGroups, func(r CCRingGroup) string { return r.AccessNumber })
	if err := s.db.Create(&CCQueueAgent{QueueName: "other", Ext: "other"}).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListQueueAgents("queue"); err != nil || len(got) != fullread.BatchSize*2+1 {
		t.Fatalf("queue filter=%d %v", len(got), err)
	}
	if err := s.CreateGateway(&Gateway{Name: "other", TenantID: 8}); err != nil {
		t.Fatal(err)
	}
	for _, disabled := range []bool{true, false} {
		names, err := s.SetGatewaysDisabledByTenant(7, disabled)
		if err != nil || len(names) != fullread.BatchSize*2+1 {
			t.Fatalf("gateway targets=%d %v", len(names), err)
		}
		seen := make(map[string]bool)
		for _, name := range names {
			if seen[name] || name == "other" {
				t.Fatalf("duplicate/cross-tenant target %q", name)
			}
			seen[name] = true
		}
		var count int64
		if err := s.db.Model(&Gateway{}).Where("tenant_id = ? AND disabled = ?", 7, disabled).Count(&count).Error; err != nil || count != int64(len(names)) {
			t.Fatalf("gateway state count=%d %v", count, err)
		}
		if other, err := s.GetGateway("other"); err != nil || other.Disabled {
			t.Fatalf("other tenant changed: %+v %v", other, err)
		}
	}
	fetches := 0
	if err := s.db.Callback().Query().Before("gorm:query").Register("test:media_fetch_failure", func(q *gorm.DB) {
		if strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
			fetches++
			if fetches == 2 {
				q.Statement.SQL.Reset()
				q.Statement.SQL.WriteString("FETCH FORWARD 256 FROM test_missing_cursor")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	names, err := s.SetGatewaysDisabledByTenant(7, true)
	s.db.Callback().Query().Remove("test:media_fetch_failure")
	if err == nil || names != nil || fetches != 2 {
		t.Fatalf("partial gateway update targets: %d %v", len(names), err)
	}
	var disabled int64
	if err := s.db.Model(&Gateway{}).Where("disabled = ?", true).Count(&disabled).Error; err != nil || disabled != 0 {
		t.Fatalf("failed fetch changed gateways: %d %v", disabled, err)
	}
	t.Log("PASS: nine complete ordered media lists, tenant-wide disable/restore target completeness, queue filter and late-fetch refusal before writes; independent GORM version")
}

func verifyMediaList[T any](t *testing.T, db *gorm.DB, record func(int) T, list func() ([]T, error), key func(T) string) {
	t.Helper()
	const count = fullread.BatchSize*2 + 1
	rows := make([]T, 0, count)
	expected := make([]string, 0, count)
	for n := count; n > 0; n-- {
		row := record(n)
		rows = append(rows, row)
		expected = append(expected, key(row))
	}
	if err := db.CreateInBatches(&rows, fullread.BatchSize).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(expected)
	fetches := 0
	if err := db.Callback().Query().After("gorm:after_query").Register("test:media_batches", func(q *gorm.DB) {
		if q.DryRun {
			return
		}
		if q.RowsAffected > fullread.BatchSize {
			t.Errorf("unbounded media read: %d", q.RowsAffected)
		}
		if strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
			fetches++
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := list()
	db.Callback().Query().Remove("test:media_batches")
	if err != nil || len(got) != count || fetches != 3 {
		t.Fatalf("%T list=%d fetches=%d %v", new(T), len(got), fetches, err)
	}
	for i, row := range got {
		if key(row) != expected[i] {
			t.Fatalf("%T order/member %d: %s want %s", new(T), i, key(row), expected[i])
		}
	}
}
