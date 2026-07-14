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
	return "16.3", nil
}

func (d *fakeMySQLDAO) GetCurrentDatabaseContext(ctx context.Context) (string, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return "go_admin", nil
}

func (d *fakeMySQLDAO) GetServerStatsContext(ctx context.Context) (monitordao.MySQLServerStats, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return monitordao.MySQLServerStats{
		UptimeSeconds: 100,
		Commits:       200,
		Rollbacks:     50,
		RowsReturned:  900,
		RowsInserted:  30,
		RowsUpdated:   20,
		RowsDeleted:   10,
		BlocksRead:    2,
		BlocksHit:     4,
	}, nil
}

func (d *fakeMySQLDAO) GetTableStatsContext(ctx context.Context, dbName string) (monitordao.MySQLTableStats, error) {
	d.contextMarker = ctx.Value(mysqlContextTestKey{})
	return monitordao.MySQLTableStats{TableCount: 2, DatabaseSize: 2048}, nil
}
