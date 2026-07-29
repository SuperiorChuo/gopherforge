package system

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/server/internal/model"
)

func TestOperationLogDAOCreateLogsContextUsesSingleMultiRowInsert(t *testing.T) {
	db, mock := newInjectedSystemDAOTestDB(t)

	logs := []*model.OperationLog{
		{Path: "/batch/1", Module: "system"},
		{Path: "/batch/2", Module: "system"},
		{Path: "/batch/3", Module: "system"},
	}

	// One statement for the whole batch: three VALUES tuples, not three INSERTs.
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "operation_logs" .*VALUES \(.+\),\(.+\),\(.+\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2).AddRow(3))
	mock.ExpectCommit()

	if err := NewOperationLogDAO(db).CreateLogsContext(context.Background(), logs); err != nil {
		t.Fatalf("CreateLogsContext() error = %v", err)
	}
}

func TestOperationLogDAOCreateLogsContextIgnoresEmptyBatch(t *testing.T) {
	db, _ := newInjectedSystemDAOTestDB(t)

	// No expectations registered: any SQL at all would fail the cleanup check.
	if err := NewOperationLogDAO(db).CreateLogsContext(context.Background(), nil); err != nil {
		t.Fatalf("CreateLogsContext(nil) error = %v", err)
	}
	if err := NewOperationLogDAO(db).CreateLogsContext(context.Background(), []*model.OperationLog{}); err != nil {
		t.Fatalf("CreateLogsContext(empty) error = %v", err)
	}
}

func TestOperationLogDAOCreateLogsContextSingleEntrySkipsBatchWrapper(t *testing.T) {
	db, mock := newInjectedSystemDAOTestDB(t)

	// A one-element batch goes through the plain Create path.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "operation_logs"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := NewOperationLogDAO(db).CreateLogsContext(context.Background(), []*model.OperationLog{
		{Path: "/batch/solo", Module: "system"},
	})
	if err != nil {
		t.Fatalf("CreateLogsContext() error = %v", err)
	}
}
