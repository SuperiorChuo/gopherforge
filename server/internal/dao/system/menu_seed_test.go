package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
	"github.com/go-admin-kit/server/internal/pkg/database"
)

func TestMenuSeedDAOInsertDefaultMenusWhenTableIsEmpty(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `menus`").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `menus`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO `menus`").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	count, err := (&MenuSeedDAO{}).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "dashboard", Title: "Dashboard"},
		{ID: 2, Name: "system", Title: "System"},
	}, now)
	if err != nil {
		t.Fatalf("BootstrapDefaultMenusContext() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestMenuSeedDAOBootstrapDefaultMenusContextHonorsCanceledContext(t *testing.T) {
	setupSystemDAOTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&MenuSeedDAO{}).BootstrapDefaultMenusContext(ctx, []model.Menu{
		{ID: 1, Name: "dashboard", Title: "Dashboard"},
	}, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BootstrapDefaultMenusContext() error = %v, want context.Canceled", err)
	}
}

func TestMenuSeedDAOSkipsWhenMenusExist(t *testing.T) {
	mock := setupSystemDAOTestDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `menus`").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(3))
	mock.ExpectCommit()

	count, err := (&MenuSeedDAO{}).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "dashboard", Title: "Dashboard"},
	}, time.Now())
	if err != nil {
		t.Fatalf("BootstrapDefaultMenusContext() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestMenuSeedDAOUsesInjectedDB(t *testing.T) {
	db, mock := newInjectedDictNoticeSeedDAOTestDB(t)
	oldDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = oldDB
	})

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `menus`").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))
	mock.ExpectExec("INSERT INTO `menus`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	count, err := NewMenuSeedDAO(db).BootstrapDefaultMenusContext(context.Background(), []model.Menu{
		{ID: 1, Name: "dashboard", Title: "Dashboard"},
	}, time.Now())
	if err != nil {
		t.Fatalf("BootstrapDefaultMenusContext() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
