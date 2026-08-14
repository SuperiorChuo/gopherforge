package system

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newRetentionTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	// 清理器会打运行日志；全局 logger 在测试进程里默认未初始化（nil），
	// 这里指到临时目录，避免日志路径依赖。
	logger.InitLogger(filepath.Join(t.TempDir(), "test.log"), "error", 1, 1, 1)

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm sqlmock db: %v", err)
	}

	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
		_ = sqlDB.Close()
	})
	return db, mock
}

// 保留天数未配置（<=0）时必须完全不启动——不起协程、不发 SQL、不上报心跳。
func TestStartLogRetentionCleanerDisabledByDefault(t *testing.T) {
	db, _ := newRetentionTestDB(t)
	opSvc := NewOperationLogServiceWithDB(db)
	loginSvc := NewLoginLogServiceWithDB(db)

	for _, days := range []int{0, -1} {
		if StartLogRetentionCleaner(context.Background(), db, &opSvc, &loginSvc, LogRetentionOptions{RetentionDays: days}) {
			t.Fatalf("StartLogRetentionCleaner(days=%d) = true, want false", days)
		}
	}
}

// 单轮清理必须跨全部租户删除（DELETE 不得带 tenant_id 过滤），且心跳照报。
func TestRunLogRetentionOnceDeletesAcrossTenants(t *testing.T) {
	db, mock := newRetentionTestDB(t)
	opSvc := NewOperationLogServiceWithDB(db)
	loginSvc := NewLoginLogServiceWithDB(db)

	mock.ExpectExec(`INSERT INTO ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`^DELETE FROM "operation_logs" WHERE created_at < \$1$`).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`^DELETE FROM "login_logs" WHERE created_at < \$1$`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	runLogRetentionOnce(context.Background(), db, &opSvc, &loginSvc, LogRetentionOptions{
		RetentionDays: 90,
		ScanInterval:  24 * time.Hour,
	})
}

// 删除报错不 panic，执行账本与心跳都要记录失败。
func TestRunLogRetentionOnceReportsHeartbeatOnError(t *testing.T) {
	db, mock := newRetentionTestDB(t)
	opSvc := NewOperationLogServiceWithDB(db)
	loginSvc := NewLoginLogServiceWithDB(db)

	mock.ExpectExec(`INSERT INTO ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`^DELETE FROM "operation_logs" WHERE created_at < \$1$`).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectExec(`^DELETE FROM "login_logs" WHERE created_at < \$1$`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE ops_task_runs`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO ops_job_heartbeats`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	runLogRetentionOnce(context.Background(), db, &opSvc, &loginSvc, LogRetentionOptions{
		RetentionDays: 30,
		ScanInterval:  time.Hour,
	})
}
