// Package fullread reads complete configuration/dispatch sets without silently
// capping them. Fetches are bounded, but the returned slice still uses O(n) memory.
package fullread

import (
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BatchSize bounds each fetch, not the number of returned rows.
const BatchSize = 256

var cursorID atomic.Uint64

// All preserves the query's filters and ordering. PostgreSQL uses one NO SCROLL,
// WITHOUT HOLD cursor: all fetches share the original SELECT snapshot, including
// nullable/non-unique sort keys, without repeated OFFSET scans or ID assumptions.
// SQLite uses a single transaction and a primary-key tie breaker for pagination.
// Callers must pass a complete-set SELECT (no limit/offset, preloads or raw SQL).
// An error, including commit failure, returns no partial result. Cursor lifetime
// is confined to the transaction. The independent media module carries the same
// implementation; a release regression checks that both copies remain identical.
func All[T any](query *gorm.DB) ([]T, error) {
	if query == nil {
		return nil, errors.New("fullread: database is not initialized")
	}
	if query.Error != nil {
		return nil, query.Error
	}
	if query.DryRun || query.Statement.SQL.Len() != 0 || len(query.Statement.Preloads) != 0 {
		return nil, errors.New("fullread: requires a model query without dry-run, raw SQL or preloads")
	}
	if _, limited := query.Statement.Clauses["LIMIT"]; limited {
		return nil, errors.New("fullread: limit/offset would truncate a complete set")
	}
	dialect := query.Dialector.Name()
	if dialect != "postgres" && dialect != "sqlite" {
		return nil, fmt.Errorf("fullread: unsupported dialect %q", dialect)
	}
	// Compile only; values remain bound parameters (never interpolate SQL).
	//nolint:unbounded-find -- DryRun builds SQL only; actual reads use bounded FETCH or LIMIT below.
	compiled := query.Session(&gorm.Session{DryRun: true}).Find(new([]T))
	if compiled.Error != nil {
		return nil, compiled.Error
	}
	if _, limited := compiled.Statement.Clauses["LIMIT"]; limited || len(compiled.Statement.Preloads) != 0 {
		return nil, errors.New("fullread: scoped limit/offset or preload is not a complete-set query")
	}
	all := make([]T, 0)
	err := query.Session(&gorm.Session{NewDB: true}).Transaction(func(tx *gorm.DB) error {
		if dialect == "sqlite" {
			// Keep the caller's clauses but execute them on this transaction.
			q := query.Session(&gorm.Session{Initialized: true})
			q.Statement.ConnPool = tx.Statement.ConnPool
			if group, ok := compiled.Statement.Clauses["GROUP BY"].Expression.(clause.GroupBy); ok {
				// Group keys identify result rows; the source model's primary key
				// is not part of a grouped projection and must not order it.
				for _, column := range group.Columns {
					q = q.Order(clause.OrderByColumn{Column: column})
				}
			} else {
				q = q.Order(clause.OrderByColumn{Column: clause.Column{Name: clause.PrimaryKey}})
			}
			for offset := 0; ; offset += BatchSize {
				var page []T
				if err := q.Session(&gorm.Session{}).Offset(offset).Limit(BatchSize).Find(&page).Error; err != nil {
					return err
				}
				all = append(all, page...)
				if len(page) < BatchSize {
					return nil
				}
			}
		}

		// Production enables GORM's shared prepared-statement cache. Cursor
		// identifiers are single-use: caching their SQL retains useless entries.
		// Unwrap only this transaction-local read session, never the caller or
		// the transaction owner (which still commits/rolls back its own pool).
		if prepared, ok := tx.Statement.ConnPool.(*gorm.PreparedStmtTX); ok {
			tx = tx.Session(&gorm.Session{NewDB: true, Initialized: true})
			tx.Statement.ConnPool = prepared.Tx
		}
		name := fmt.Sprintf("gak_fullread_%d", cursorID.Add(1))
		ctx := tx.Statement.Context
		if _, err := tx.Statement.ConnPool.ExecContext(ctx,
			"DECLARE "+name+" NO SCROLL CURSOR WITHOUT HOLD FOR "+compiled.Statement.SQL.String(),
			compiled.Statement.Vars...); err != nil {
			return err
		}
		for {
			var page []T
			//nolint:unbounded-find -- This raw statement FETCHes at most BatchSize rows; Find retains GORM model decoding/hooks.
			if err := tx.Raw(fmt.Sprintf("FETCH FORWARD %d FROM %s", BatchSize, name)).Find(&page).Error; err != nil {
				return err
			}
			all = append(all, page...)
			if len(page) < BatchSize {
				_, err := tx.Statement.ConnPool.ExecContext(ctx, "CLOSE "+name)
				return err
			}
		}
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return all, nil
}
