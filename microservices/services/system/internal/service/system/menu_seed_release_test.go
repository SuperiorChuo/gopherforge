//go:build release_integration

package system

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	model "github.com/go-admin-kit/services/shared/pkg/model"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This is an explicit isolated-database probe, not a replacement for starting
// system-service and the remaining services. The release harness supplies a
// network-none PostgreSQL container, never an existing deployment database.
func TestReleaseMenuBootstrapPostgres(t *testing.T) {
	dsn := os.Getenv("GAK_RELEASE_TEST_DSN")
	const prefix = "host=127.0.0.1 port=5432 user=postgres password=isolated-test-only dbname="
	if !strings.HasPrefix(dsn, prefix) || os.Getenv("GAK_RELEASE_TEST_CONFIRM") != "isolated-release-database" {
		t.Fatal("requires the explicit isolated release harness")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db = db.WithContext(ctx)
	exec := func(query string) {
		t.Helper()
		if err := db.Exec(query).Error; err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	var menuOID uint
	if err := db.Raw("SELECT 'public.menus'::regclass::oid").Scan(&menuOID).Error; err != nil {
		t.Fatal(err)
	}
	// A source seed ID occupied by an unrelated administrator menu must not
	// suppress the BPM parent or steal this record's ID/associations.
	exec("INSERT INTO menus(id,name,title,path) VALUES (122,'release-custom-id','Preserve custom ID','/release-custom-id')")
	exec("UPDATE menus SET title='Preserve administrator edit', status=0 WHERE name='bpm' AND path='/bpm'")
	exec("CREATE SCHEMA IF NOT EXISTS release_probe")
	exec("CREATE TABLE release_probe.menu_link(menu_id BIGINT REFERENCES menus(id))")
	exec("INSERT INTO release_probe.menu_link VALUES (122)")
	exec("INSERT INTO menu_permissions(menu_id,permission_id) VALUES (122,1) ON CONFLICT DO NOTHING")
	// Exercise the actual menu model's AutoMigrate against the populated public
	// schema; other system models and business AutoMigrate remain runtime gates.
	if err := db.AutoMigrate(&model.Menu{}, &model.MenuPermission{}); err != nil {
		t.Fatal(err)
	}
	var afterOID uint
	if err := db.Raw("SELECT 'public.menus'::regclass::oid").Scan(&afterOID).Error; err != nil || afterOID != menuOID {
		t.Fatalf("menu table identity changed: %d -> %d, %v", menuOID, afterOID, err)
	}
	var existing []model.Menu
	if err := db.Order("id").Find(&existing).Error; err != nil {
		t.Fatal(err)
	}
	var existingIDs []uint
	for _, menu := range existing {
		existingIDs = append(existingIDs, menu.ID)
	}
	originalRows := func() string {
		t.Helper()
		var value string
		if err := db.Raw("SELECT jsonb_agg(m ORDER BY id)::text FROM menus m WHERE id IN ?", existingIDs).Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := originalRows()
	exec("SELECT setval(pg_get_serial_sequence('menus','id'), 300000, true)")
	exec("INSERT INTO menus(name,title,path) VALUES ('release-deleted-bootstrap','deleted','/release-deleted-bootstrap')")
	exec("DELETE FROM menus WHERE name='release-deleted-bootstrap'")

	sequenceValue := func() int64 {
		t.Helper()
		var value int64
		if err := db.Raw("SELECT pg_sequence_last_value(pg_get_serial_sequence('menus','id')::regclass)").Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	// Reproduce an allowed administrator rename before any CRM seed exists:
	// BPM matches the path while CRM matches the name of the same old row.
	var crmCount int64
	if err := db.Model(&model.Menu{}).Where("name = 'crm' OR path = '/crm'").Count(&crmCount).Error; err != nil || crmCount != 0 {
		t.Fatalf("cross-seed fixture already has CRM: %d, %v", crmCount, err)
	}
	// Fresh SQL alone does not contain the old Go BPM seed. Add it only for
	// this negative fixture, then remove only that newly inserted test row.
	fixture := db.Exec("INSERT INTO menus(id,name,title,path) VALUES (35,'bpm','Public BPM fixture','/bpm') ON CONFLICT (id) DO NOTHING")
	if fixture.Error != nil {
		t.Fatal(fixture.Error)
	}
	if result := db.Exec("UPDATE menus SET name='crm' WHERE id=35 AND name='bpm' AND path='/bpm'"); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("cross-seed fixture missing public BPM: rows=%d, %v", result.RowsAffected, result.Error)
	}
	allMenuState := func() string {
		t.Helper()
		var value string
		if err := db.Raw(`SELECT jsonb_build_object(
			'menus', (SELECT jsonb_agg(m ORDER BY id) FROM menus m),
			'permissions', (SELECT jsonb_agg(p ORDER BY menu_id,permission_id) FROM menu_permissions p),
			'links', (SELECT jsonb_agg(l ORDER BY menu_id) FROM release_probe.menu_link l))::text`).Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	renameState, renameSequence := allMenuState(), sequenceValue()
	if n, err := BootstrapDefaultMenusContext(ctx, db); n.Menus != 0 || err == nil || !strings.Contains(err.Error(), "share persisted ID 35") {
		t.Fatalf("cross-seed bootstrap: created=%d err=%v", n.Menus, err)
	}
	if allMenuState() != renameState || sequenceValue() != renameSequence {
		t.Fatal("cross-seed refusal changed menus, associations or sequence")
	}
	if fixture.RowsAffected == 1 {
		exec("DELETE FROM menus WHERE id=35 AND name='crm' AND path='/bpm'")
	} else {
		exec("UPDATE menus SET name='bpm' WHERE id=35") // Restore only the test's deliberate rename.
	}
	if originalRows() != before {
		t.Fatal("cross-seed fixture restoration changed original rows")
	}

	type result struct {
		created int
		err     error
	}
	results := make(chan result, 4)
	for range 4 {
		go func() {
			n, err := BootstrapDefaultMenusContext(ctx, db)
			results <- result{n.Menus, err}
		}()
	}
	totalCreated := 0
	for range 4 {
		r := <-results
		if r.err != nil {
			t.Errorf("concurrent bootstrap failed: %v", r.err)
		}
		totalCreated += r.created
	}
	if t.Failed() {
		t.FailNow()
	}
	if totalCreated == 0 {
		t.Fatal("fixture did not exercise inserting new full-product menus")
	}
	if after := originalRows(); after != before {
		t.Fatal("bootstrap rewrote existing menu data or administrator edits")
	}
	var all []model.Menu
	if err := db.Find(&all).Error; err != nil {
		t.Fatal(err)
	}
	persisted := make(map[uint]model.Menu)
	claimed := make(map[uint]uint)
	oldIDs := make(map[uint]bool)
	for _, menu := range existing {
		oldIDs[menu.ID] = true
	}
	for _, seed := range DefaultMenus() {
		var matches []model.Menu
		for _, menu := range all {
			if menu.Name == seed.Name && menu.Path == seed.Path {
				matches = append(matches, menu)
			}
		}
		if len(matches) == 0 {
			for _, menu := range all {
				if menu.Name == seed.Name || menu.Path == seed.Path {
					matches = append(matches, menu)
				}
			}
		}
		if seed.Name == "setting" && len(matches) == 2 &&
			matches[0].ID == 22 && matches[1].ID == 23 && oldIDs[22] && oldIDs[23] {
			// The immutable public SQL and old Go seed created this known pair.
			// Both old rows were checked byte-for-byte above; no deduplication.
			matches = matches[1:]
		}
		if len(matches) != 1 {
			t.Fatalf("menu %s has %d equally strong persisted identities", seed.Name, len(matches))
		}
		menu := matches[0]
		if previous, duplicate := claimed[menu.ID]; duplicate {
			t.Fatalf("seeds %d and %d share persisted menu %d", previous, seed.ID, menu.ID)
		}
		claimed[menu.ID] = seed.ID
		persisted[seed.ID] = menu
		if !oldIDs[menu.ID] && menu.ID <= 300001 {
			t.Fatalf("reused a consumed menu ID: %d", menu.ID)
		}
	}
	for _, seed := range DefaultMenus() {
		menu := persisted[seed.ID]
		if !oldIDs[menu.ID] && seed.ParentID != 0 && menu.ParentID != persisted[seed.ParentID].ID {
			t.Fatalf("wrong persisted parent for %s: %+v", seed.Name, menu)
		}
	}
	if len(all)-len(existing) != totalCreated {
		t.Fatalf("created count %d does not match actual new rows %d", totalCreated, len(all)-len(existing))
	}
	exec(`DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM menu_permissions WHERE menu_id=122 AND permission_id=1)
		OR NOT EXISTS (SELECT 1 FROM release_probe.menu_link WHERE menu_id=122) THEN
			RAISE EXCEPTION 'menu association lost';
		END IF;
	END $$`)
	var foreignKeyError *pgconn.PgError
	if err := db.Exec("INSERT INTO release_probe.menu_link VALUES (-1)").Error; !errors.As(err, &foreignKeyError) || foreignKeyError.Code != "23503" {
		t.Fatalf("expected menu foreign key violation, got %v", err)
	}
	sequenceBefore := sequenceValue()
	if n, err := BootstrapDefaultMenusContext(ctx, db); err != nil || n.Menus != 0 {
		t.Fatalf("repeated bootstrap: created=%d err=%v", n.Menus, err)
	}
	if sequenceValue() != sequenceBefore || originalRows() != before {
		t.Fatal("idempotent bootstrap changed existing rows or sequence")
	}
	// Identity ambiguity must fail before any insert or sequence adjustment.
	exec("INSERT INTO menus(name,title,path) VALUES ('ai','Ambiguous','/ai')")
	sequenceBefore = sequenceValue()
	if n, err := BootstrapDefaultMenusContext(ctx, db); n.Menus != 0 || err == nil || !strings.Contains(err.Error(), "ambiguous menu identity") {
		t.Fatalf("ambiguous bootstrap: created=%d err=%v", n.Menus, err)
	}
	if sequenceValue() != sequenceBefore || originalRows() != before {
		t.Fatal("refused bootstrap changed existing rows or sequence")
	}
	t.Logf("PASS: %d created, 4 concurrent bootstraps, stable public/custom IDs and associations, OID/FK, parent remap, high-water, idempotence, ambiguity and cross-seed guards", totalCreated)
}
