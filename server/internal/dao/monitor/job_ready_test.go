package monitor

import (
	"testing"

	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestJobDAOReadyReflectsDatabaseAvailability(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	if NewJobDAO().Ready() {
		t.Fatal("Ready() = true, want false when database is nil")
	}
}

func TestJobDAOReadyReturnsTrueWhenDatabaseIsConfigured(t *testing.T) {
	setupMonitorDAOTestDB(t)

	if !NewJobDAO().Ready() {
		t.Fatal("Ready() = false, want true when database is configured")
	}
}
