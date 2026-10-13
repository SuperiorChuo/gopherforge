package fullread

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type record struct {
	ID       uint64 `gorm:"primaryKey"`
	TenantID uint64
	Sort     int
	Name     string
	Hooked   bool `gorm:"-"`
}

func (r *record) AfterFind(*gorm.DB) error { r.Hooked = true; return nil }

func mockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return db, mock
}

const declarePattern = `DECLARE gak_fullread_\d+ NO SCROLL CURSOR WITHOUT HOLD FOR SELECT \* FROM "records" WHERE name = \$1 ORDER BY sort DESC`
const fetchPattern = `FETCH FORWARD 256 FROM gak_fullread_\d+`
const closePattern = `CLOSE gak_fullread_\d+`

func pageRows(start, count int) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "sort", "name"})
	for i := start; i < start+count; i++ {
		rows.AddRow(i+1, -i, "bound ' value")
	}
	return rows
}

func TestPostgresCompleteSetBoundariesAndHooks(t *testing.T) {
	for _, count := range []int{0, 1, BatchSize - 1, BatchSize, BatchSize + 1, BatchSize * 2, BatchSize*2 + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, mock := mockDB(t)
			mock.ExpectBegin()
			mock.ExpectExec(declarePattern).WithArgs("bound ' value").WillReturnResult(sqlmock.NewResult(0, 0))
			for start := 0; ; start += BatchSize {
				n := min(BatchSize, count-start)
				mock.ExpectQuery(fetchPattern).WillReturnRows(pageRows(start, n))
				if n < BatchSize {
					break
				}
			}
			mock.ExpectExec(closePattern).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			got, err := All[*record](db.Where("name = ?", "bound ' value").Order("sort DESC"))
			if err != nil || got == nil || len(got) != count {
				t.Fatalf("len=%d nil=%t err=%v", len(got), got == nil, err)
			}
			for i, row := range got {
				if row.ID != uint64(i+1) || row.Sort != -i || !row.Hooked {
					t.Fatalf("row %d = %+v", i, row)
				}
			}
		})
	}
}

func TestPostgresOneShotCursorsDoNotEnterPreparedStatementCache(t *testing.T) {
	db, mock := mockDB(t)
	db = db.Session(&gorm.Session{PrepareStmt: true})
	cache, ok := db.Statement.ConnPool.(*gorm.PreparedStmtDB)
	if !ok {
		t.Fatal("prepared-statement fixture is not enabled")
	}
	defer cache.Close()
	for repeat := 0; repeat < 3; repeat++ {
		mock.ExpectBegin()
		// Cursor identifiers are unique per call. Preparing these statements
		// would retain useless SQL/cache entries in the application-wide cache.
		mock.ExpectExec(declarePattern).WithArgs("bound ' value").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(fetchPattern).WillReturnRows(pageRows(0, 1))
		mock.ExpectExec(closePattern).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		got, err := All[record](db.Where("name = ?", "bound ' value").Order("sort DESC"))
		if err != nil || len(got) != 1 || !got[0].Hooked {
			t.Fatalf("prepared database read %d: rows=%d err=%v", repeat, len(got), err)
		}
		if keys := cache.Stmts.Keys(); len(keys) != 0 {
			t.Fatalf("one-shot cursor polluted shared cache: %v", keys)
		}
		if db.Statement.ConnPool != cache {
			t.Fatal("caller prepared connection pool was replaced")
		}
	}
}

func TestPostgresNeverReturnsPartialResults(t *testing.T) {
	boom := errors.New("injected database failure")
	for _, stage := range []string{"begin", "declare", "fetch", "decode", "close", "commit"} {
		t.Run(stage, func(t *testing.T) {
			db, mock := mockDB(t)
			begin := mock.ExpectBegin()
			if stage == "begin" {
				begin.WillReturnError(boom)
			} else {
				decl := mock.ExpectExec(declarePattern).WithArgs("bound ' value")
				if stage == "declare" {
					decl.WillReturnError(boom)
				} else {
					decl.WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectQuery(fetchPattern).WillReturnRows(pageRows(0, BatchSize))
					next := mock.ExpectQuery(fetchPattern)
					switch stage {
					case "fetch":
						next.WillReturnError(boom)
					case "decode":
						next.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-an-id"))
					default:
						next.WillReturnRows(pageRows(BatchSize, 1))
						close := mock.ExpectExec(closePattern)
						if stage == "close" {
							close.WillReturnError(boom)
						} else {
							close.WillReturnResult(sqlmock.NewResult(0, 0))
						}
					}
				}
				if stage == "commit" {
					mock.ExpectCommit().WillReturnError(boom)
				} else {
					mock.ExpectRollback()
				}
			}
			got, err := All[record](db.Where("name = ?", "bound ' value").Order("sort DESC"))
			if got != nil || err == nil {
				t.Fatalf("returned partial/success: %d, %v", len(got), err)
			}
		})
	}
}

func TestRejectsTruncatingOrUnsupportedQueryShapesBeforeStartingTransaction(t *testing.T) {
	db, _ := mockDB(t)
	for _, q := range []*gorm.DB{nil, db.Limit(3), db.Offset(1), db.Preload("Other"), db.Raw("SELECT 1"), db.Session(&gorm.Session{DryRun: true}), db.Scopes(func(q *gorm.DB) *gorm.DB { return q.Limit(3) })} {
		got, err := All[record](q)
		if got != nil || err == nil {
			t.Fatalf("accepted incomplete query: %v, %v", got, err)
		}
	}
}

func TestSQLiteCompleteSetOrderingIsolationAndQueryReuse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.AutoMigrate(&record{}); err != nil {
		t.Fatal(err)
	}
	if err := tenant.Register(db); err != nil {
		t.Fatal(err)
	}
	const count = BatchSize*2 + 1
	for i := 1; i <= count; i++ {
		for _, tenantID := range []uint64{1, 2} {
			if err := db.Create(&record{TenantID: tenantID, Sort: count - i, Name: fmt.Sprint(i)}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	q := db.WithContext(tenant.WithContext(context.Background(), 2)).Order("sort ASC")
	for repeat := 0; repeat < 2; repeat++ {
		got, err := All[record](q)
		if err != nil || len(got) != count {
			t.Fatalf("len=%d err=%v", len(got), err)
		}
		for i, row := range got {
			if row.Sort != i || row.TenantID != 2 || !row.Hooked {
				t.Fatalf("row %d = %+v", i, row)
			}
		}
	}
	// Reusing the caller query must not retain a closed transaction or added limit.
	var first record
	if err := q.First(&first).Error; err != nil || first.Sort != 0 {
		t.Fatalf("caller query mutated: %+v %v", first, err)
	}
}
