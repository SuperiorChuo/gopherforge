package testdb

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-admin-kit/services/shared/pkg/fullread"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FailLater injects a read error only after a complete-set query has returned
// its first batch. The caller must assert no partial business result escapes.
func FailLater(t testing.TB, db *gorm.DB, read func()) {
	t.Helper()
	name := fmt.Sprintf("test:late_failure_%d", callbackID.Add(1))
	fetches, failed := 0, false
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(q *gorm.DB) {
		if q.DryRun {
			return
		}
		later := false
		if strings.HasPrefix(q.Statement.SQL.String(), "FETCH FORWARD ") {
			fetches++
			later = fetches == 2
		} else if limit, ok := q.Statement.Clauses["LIMIT"].Expression.(clause.Limit); ok {
			later = limit.Limit != nil && *limit.Limit == fullread.BatchSize && limit.Offset >= fullread.BatchSize
		}
		if later {
			failed = true
			q.AddError(errors.New("injected later complete-set read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(name)
	read()
	if !failed {
		t.Fatal("late-fetch negative path was not reached")
	}
}
