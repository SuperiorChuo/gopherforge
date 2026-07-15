package database

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/ai/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCloseNoopsWhenDatabaseIsNil(t *testing.T) {
	oldDB := DB
	DB = nil
	t.Cleanup(func() {
		DB = oldDB
	})

	if err := Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestCloseClosesUnderlyingSQLDBAndClearsGlobal(t *testing.T) {
	oldDB := DB
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	mock.ExpectClose()
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}
	DB = gormDB
	t.Cleanup(func() {
		DB = oldDB
	})

	if err := Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if DB != nil {
		t.Fatal("Close() should clear global DB")
	}
	if err := sqlDB.Ping(); err == nil {
		t.Fatal("expected underlying sql DB to be closed")
	}
}

func TestApplyConnectionPoolConfigSetsMaxOpenConnections(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	defer sqlDB.Close()

	applyConnectionPoolConfig(sqlDB, config.DatabaseConfig{
		MaxIdleConns:           2,
		MaxOpenConns:           7,
		ConnMaxLifetimeSeconds: 30,
		ConnMaxIdleTimeSeconds: 15,
	})

	if got := sqlDB.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("max open connections = %d, want 7", got)
	}
}
