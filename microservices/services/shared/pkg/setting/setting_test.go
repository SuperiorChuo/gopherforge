package setting

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/shared/pkg/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCreateIfAbsentContextReportsInsertResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer db.Close()

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	dao := NewSettingDAO(gormDB)
	insert := regexp.QuoteMeta(`INSERT INTO "system_settings"`)

	mock.ExpectBegin()
	mock.ExpectExec(insert).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	created, err := dao.CreateIfAbsentContext(context.Background(), &model.SystemSetting{SettingKey: "oidc.signing_key"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if !created {
		t.Fatal("first create = false, want true")
	}

	mock.ExpectBegin()
	mock.ExpectExec(insert).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	created, err = dao.CreateIfAbsentContext(context.Background(), &model.SystemSetting{SettingKey: "oidc.signing_key"})
	if err != nil {
		t.Fatalf("conflicting create: %v", err)
	}
	if created {
		t.Fatal("conflicting create = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
