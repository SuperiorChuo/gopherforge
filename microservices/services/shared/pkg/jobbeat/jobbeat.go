// Package jobbeat reports distributed background-task heartbeats and explicit
// execution lifecycles to the task center. Report only updates the latest
// heartbeat, so high-frequency empty polling loops do not create unbounded
// history. Discrete work uses Start/Finish or Record to write ops_task_runs.
//
// All writes are soft failures: task execution must never fail merely because
// the observability database is unavailable or has not been migrated yet.
package jobbeat

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Run describes one task execution or heartbeat cycle.
type Run struct {
	RunID         string
	Key           string // globally unique task key: <service>.<task>
	Service       string
	Description   string
	Source        string // worker / scheduler / ops-cron
	Trigger       string // scheduled / manual / shell
	Attempt       int
	CorrelationID string
	IntervalSec   int64 // expected heartbeat interval; 0 disables stale detection
	StartedAt     time.Time
	Err           error
}

// Execution tracks one explicit task run. Finish is idempotent.
type Execution struct {
	db   *gorm.DB
	run  Run
	once sync.Once
}

// NewRunID returns a random 128-bit identifier suitable for public diagnostics.
func NewRunID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

// Start records a running execution and returns a tracker. Database errors are
// logged and swallowed; callers should always defer Finish.
func Start(db *gorm.DB, run Run) *Execution {
	run = normalizeRun(run)
	execution := &Execution{db: db, run: run}
	if db == nil || run.Key == "" {
		return execution
	}
	if err := db.Exec(`
INSERT INTO ops_task_runs
  (run_id, task_key, service, description, source, trigger_type, status, attempt,
   correlation_id, started_at, duration_ms, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		run.RunID, run.Key, run.Service, run.Description, run.Source, run.Trigger,
		StatusRunning, run.Attempt, run.CorrelationID, run.StartedAt, time.Now()).Error; err != nil {
		log.Printf("jobbeat: start %s failed (soft): %v", run.Key, err)
	}
	return execution
}

// Finish completes an explicit execution and also refreshes its heartbeat.
// message is operator-facing output and must not contain payloads or secrets.
func (e *Execution) Finish(err error, message string) {
	if e == nil {
		return
	}
	e.once.Do(func() {
		e.run.Err = err
		finishedAt := time.Now()
		status, errorMessage := StatusSucceeded, ""
		if err != nil {
			status, errorMessage = StatusFailed, truncate(err.Error(), 2048)
		}
		durationMS := finishedAt.Sub(e.run.StartedAt).Milliseconds()
		if durationMS < 0 {
			durationMS = 0
		}
		if e.db != nil && e.run.Key != "" {
			result := e.db.Exec(`
UPDATE ops_task_runs
SET status = ?, finished_at = ?, duration_ms = ?, message = ?, error_message = ?
WHERE run_id = ? AND status = ?`,
				status, finishedAt, durationMS, truncate(message, 2048), errorMessage,
				e.run.RunID, StatusRunning)
			if result.Error != nil {
				log.Printf("jobbeat: finish %s failed (soft): %v", e.run.Key, result.Error)
			} else if result.RowsAffected == 0 {
				log.Printf("jobbeat: finish %s found no running execution (soft)", e.run.Key)
			}
		}
		Report(e.db, e.run)
	})
}

// Record writes an already-finished discrete run while preserving the same
// Start/Finish lifecycle and heartbeat semantics.
func Record(db *gorm.DB, run Run, message string) {
	execution := Start(db, run)
	execution.Finish(run.Err, message)
}

// Report updates one latest-heartbeat row. It deliberately does not append to
// ops_task_runs; empty polling loops call Report frequently.
func Report(db *gorm.DB, run Run) {
	run = normalizeRun(run)
	if db == nil || run.Key == "" {
		return
	}
	status, lastErr := "ok", ""
	failInc := 0
	if run.Err != nil {
		status, lastErr = "error", truncate(run.Err.Error(), 2048)
		failInc = 1
	}
	finishedAt := time.Now()
	durationMS := finishedAt.Sub(run.StartedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	err := db.Exec(`
INSERT INTO ops_job_heartbeats
  (job_key, service, description, interval_sec, last_run_at, last_status, last_error, last_duration_ms, runs, fails, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT (job_key) DO UPDATE SET
  service = EXCLUDED.service,
  description = EXCLUDED.description,
  interval_sec = EXCLUDED.interval_sec,
  last_run_at = EXCLUDED.last_run_at,
  last_status = EXCLUDED.last_status,
  last_error = EXCLUDED.last_error,
  last_duration_ms = EXCLUDED.last_duration_ms,
  runs = ops_job_heartbeats.runs + 1,
  fails = ops_job_heartbeats.fails + ?,
  updated_at = EXCLUDED.updated_at`,
		run.Key, run.Service, run.Description, run.IntervalSec, finishedAt, status, lastErr,
		durationMS, failInc, finishedAt, failInc).Error
	if err != nil {
		log.Printf("jobbeat: report %s failed (soft): %v", run.Key, err)
	}
}

func normalizeRun(run Run) Run {
	if run.RunID == "" {
		run.RunID = NewRunID()
	}
	if run.Source == "" {
		run.Source = "worker"
	}
	if run.Trigger == "" {
		run.Trigger = "scheduled"
	}
	if run.Attempt <= 0 {
		run.Attempt = 1
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now()
	}
	run.RunID = truncate(run.RunID, 64)
	run.Key = truncate(strings.TrimSpace(run.Key), 100)
	run.Service = truncate(strings.TrimSpace(run.Service), 50)
	run.Description = truncate(strings.TrimSpace(run.Description), 255)
	run.Source = truncate(strings.TrimSpace(run.Source), 24)
	run.Trigger = truncate(strings.TrimSpace(run.Trigger), 24)
	run.CorrelationID = truncate(strings.TrimSpace(run.CorrelationID), 96)
	return run
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
