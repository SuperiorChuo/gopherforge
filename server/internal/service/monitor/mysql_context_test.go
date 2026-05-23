package monitor

import (
	"context"
	"database/sql"
	"testing"

	monitordao "github.com/go-admin-kit/server/internal/dao/monitor"
)

type mysqlContextTestKey struct{}

func TestMySQLServiceGetMySQLInfoContextPropagatesContext(t *testing.T) {
	dao := &fakeMySQLDAO{}
	service := &MySQLService{dao: dao}
	ctx := context.WithValue(context.Background(), mysqlContextTestKey{}, "mysql-request")

	_, err := service.GetMySQLInfoContext(ctx)
	if err != nil {
		t.Fatalf("GetMySQLInfoContext() error = %v", err)
	}
	if dao.contextMarker != "mysql-request" {
		t.Fatalf("context marker = %#v, want mysql-request", dao.contextMarker)
	}
}

type fakeMySQLDAO struct {
	contextMarker any
}

func (d *fakeMySQLDAO) ConnectionStatsContext(ctx context.Context) (sql.DBStats, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return sql.DBStats{}, nil
}

func (d *fakeMySQLDAO) GetVersionContext(ctx context.Context) (string, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return "8.0.36", nil
}

func (d *fakeMySQLDAO) GetCurrentDatabaseContext(ctx context.Context) (string, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return "go_admin", nil
}

func (d *fakeMySQLDAO) GetNameValuesContext(ctx context.Context, query string) (map[string]string, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return map[string]string{
		"Uptime":                 "100",
		"Questions":              "250",
		"character_set_database": "utf8mb4",
		"collation_database":     "utf8mb4_unicode_ci",
	}, nil
}

func (d *fakeMySQLDAO) GetTableStatsContext(ctx context.Context, dbName string) (monitordao.MySQLTableStats, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return monitordao.MySQLTableStats{TableCount: 2, DatabaseSize: 2048}, nil
}
