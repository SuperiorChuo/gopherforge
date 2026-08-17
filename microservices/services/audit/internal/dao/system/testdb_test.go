package system

import (
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	localmodel "github.com/go-admin-kit/services/audit/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/authz"
	sharedmodel "github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupSystemDAOTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	if err := authz.RegisterDataScopePlugin(db); err != nil {
		t.Fatalf("register data scope plugin: %v", err)
	}
	authz.RegisterScopedModel(reflect.TypeOf(sharedmodel.File{}), authz.ScopeByOwner)
	authz.RegisterScopedModel(reflect.TypeOf(localmodel.LoginLog{}), authz.ScopeByOwner)
	authz.RegisterScopedModel(reflect.TypeOf(localmodel.OperationLog{}), authz.ScopeByOwner)

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})

	return db, mock
}
