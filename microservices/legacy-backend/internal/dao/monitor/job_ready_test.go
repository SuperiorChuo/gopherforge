package monitor

import (
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestJobDAOReadyIsFalseWithoutInjectedDatabase(t *testing.T) {
	// Ready() still falls back to the global database.DB until the fallback
	// removal lands, so asserting false requires clearing the global. This is
	// the only test allowed to keep a database.DB reference during the
	// transition.
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	if (&JobDAO{}).Ready() {
		t.Fatal("Ready() = true, want false when no database is injected")
	}
}

func TestJobDAOReadyReturnsTrueWithInjectedDatabase(t *testing.T) {
	db, _ := newMonitorDAOTestDB(t)

	if !NewJobDAO(db).Ready() {
		t.Fatal("Ready() = false, want true when a database is injected")
	}
}
