package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFanSyncLeaseLost  = errors.New("fan sync lease lost")
	ErrFanSyncIncomplete = errors.New("fan sync result is incomplete")
)

func fanSyncActiveKey(tenantID, accountID uint64) string {
	return fmt.Sprintf("%d:%d", EnsureTenant(tenantID), accountID)
}

func newFanSyncLeaseToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// EnqueueFanSyncJob creates a new run or returns the already-active run for the
// same tenant/account. The nullable unique ActiveKey makes concurrent POSTs
// idempotent without serializing unrelated accounts.
func (s *Store) EnqueueFanSyncJob(ctx context.Context, tenantID, accountID, createdBy uint64) (*MpFanSyncJob, bool, error) {
	tenantID = EnsureTenant(tenantID)
	key := fanSyncActiveKey(tenantID, accountID)
	for attempt := 0; attempt < 4; attempt++ {
		now := time.Now()
		job := &MpFanSyncJob{
			TenantID: tenantID, AccountID: accountID, ActiveKey: &key,
			Status: "pending", NextAt: now, CreatedBy: createdBy,
		}
		res := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "active_key"}},
			DoNothing: true,
		}).Create(job)
		if res.Error != nil {
			return nil, false, res.Error
		}
		if res.RowsAffected == 1 {
			return job, false, nil
		}

		var active MpFanSyncJob
		findErr := s.db.WithContext(ctx).Where("active_key = ?", key).First(&active).Error
		if findErr == nil {
			return &active, true, nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return nil, false, findErr
		}
		// The conflicting job can become terminal between INSERT and SELECT.
		// Retry the insert rather than surfacing a transient unique race as 500.
	}
	return nil, false, errors.New("fan sync enqueue changed concurrently; retry request")
}

func (s *Store) LatestFanSyncJob(ctx context.Context, tenantID, accountID uint64) (*MpFanSyncJob, error) {
	var job MpFanSyncJob
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND account_id = ?", EnsureTenant(tenantID), accountID).
		Order("id DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) GetAccountContext(ctx context.Context, id, tenantID uint64) (*MpAccount, error) {
	var account MpAccount
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", EnsureTenant(tenantID), id).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// ClaimFanSyncJobs leases due jobs. Expired running jobs are recoverable, and
// PostgreSQL SKIP LOCKED lets both Swarm replicas work on different accounts.
func (s *Store) ClaimFanSyncJobs(ctx context.Context, limit int, lease time.Duration) ([]MpFanSyncJob, error) {
	if limit <= 0 {
		limit = 2
	}
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	var claimed []MpFanSyncJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		q := tx.Where(`active_key IS NOT NULL AND (
			(status = ? AND next_at <= ?) OR
			(status = ? AND lease_until IS NOT NULL AND lease_until <= ?)
		)`, "pending", now, "running", now).Order("id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&claimed).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(lease)
		for i := range claimed {
			token, err := newFanSyncLeaseToken()
			if err != nil {
				return err
			}
			res := tx.Model(&MpFanSyncJob{}).
				Where("id = ? AND active_key IS NOT NULL", claimed[i].ID).
				Updates(map[string]any{
					"status":      "running",
					"lease_until": leaseUntil,
					"lease_token": token,
					"started_at":  gorm.Expr("COALESCE(started_at, ?)", now),
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrFanSyncLeaseLost
			}
			claimed[i].Status = "running"
			claimed[i].LeaseUntil = &leaseUntil
			claimed[i].LeaseToken = token
			if claimed[i].StartedAt == nil {
				started := now
				claimed[i].StartedAt = &started
			}
		}
		return nil
	})
	return claimed, err
}

// StageFanSyncPage durably stores the upstream page before any profile detail
// call. A restart therefore resumes from PageOffset without losing the cursor.
func (s *Store) StageFanSyncPage(ctx context.Context, job *MpFanSyncJob, total int, openIDs []string, nextCursor string) error {
	if job == nil || job.LeaseToken == "" {
		return ErrFanSyncLeaseLost
	}
	raw, err := json.Marshal(openIDs)
	if err != nil {
		return err
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&MpFanSyncJob{}).
		Where("id = ? AND status = ? AND lease_token = ? AND lease_until > ?", job.ID, "running", job.LeaseToken, now).
		Updates(map[string]any{
			"status":        "pending",
			"page_open_ids": string(raw),
			"page_offset":   0,
			"next_cursor":   nextCursor,
			"total_count":   total,
			"listed_count":  gorm.Expr("listed_count + ?", len(openIDs)),
			"attempts":      0,
			"last_error":    "",
			"next_at":       now,
			"lease_until":   nil,
			"lease_token":   "",
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrFanSyncLeaseLost
	}
	return nil
}

// CommitFanSyncBatch atomically upserts at most 100 profiles and advances the
// persisted page offset. The lease-token CAS rolls back the profile writes if
// an expired worker tries to commit after another replica recovered the job.
func (s *Store) CommitFanSyncBatch(
	ctx context.Context,
	job *MpFanSyncJob,
	fans []MpFan,
	snapshotAt time.Time,
	pageLength int,
	nextDelay time.Duration,
) (bool, error) {
	if job == nil || job.LeaseToken == "" || len(fans) == 0 || pageLength <= 0 {
		return false, errors.New("invalid fan sync batch")
	}
	finished := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var current MpFanSyncJob
		q := tx.Where(
			"id = ? AND status = ? AND lease_token = ? AND page_offset = ? AND lease_until > ?",
			job.ID, "running", job.LeaseToken, job.PageOffset, now,
		)
		if tx.Dialector.Name() == "postgres" {
			// Keep ClaimFanSyncJobs(... SKIP LOCKED) from recovering this job
			// while its profile/reconciliation transaction is still committing.
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFanSyncLeaseLost
			}
			return err
		}
		if err := upsertFansTx(tx, fans, snapshotAt, job.ID); err != nil {
			return err
		}
		var syncedCount int64
		if err := tx.Model(&MpFan{}).
			Where("tenant_id = ? AND account_id = ? AND last_sync_job_id = ?", job.TenantID, job.AccountID, job.ID).
			Count(&syncedCount).Error; err != nil {
			return err
		}

		newOffset := job.PageOffset + len(fans)
		if newOffset > pageLength {
			return errors.New("fan sync batch exceeds staged page")
		}
		updates := map[string]any{
			"synced_count": int(syncedCount),
			"attempts":     0,
			"last_error":   "",
			"lease_until":  nil,
			"lease_token":  "",
		}
		switch {
		case newOffset < pageLength:
			updates["status"] = "pending"
			updates["page_offset"] = newOffset
			updates["next_at"] = now.Add(nextDelay)
		case job.NextCursor != "":
			updates["status"] = "pending"
			updates["cursor"] = job.NextCursor
			updates["next_cursor"] = ""
			updates["page_open_ids"] = ""
			updates["page_offset"] = 0
			updates["next_at"] = now.Add(nextDelay)
		default:
			if err := validateFanSyncCompletion(current.ListedCount, current.TotalCount, int(syncedCount)); err != nil {
				return err
			}
			reconcileAt := snapshotAt
			if job.StartedAt != nil && !job.StartedAt.IsZero() {
				reconcileAt = *job.StartedAt
			}
			if err := markMissingFansUnsubscribedTx(tx, job.TenantID, job.AccountID, job.ID, reconcileAt, now); err != nil {
				return err
			}
			finished = true
			updates["status"] = "succeeded"
			updates["active_key"] = nil
			updates["cursor"] = ""
			updates["next_cursor"] = ""
			updates["page_open_ids"] = ""
			updates["page_offset"] = 0
			updates["finished_at"] = now
		}
		res := tx.Model(&MpFanSyncJob{}).
			Where("id = ? AND status = ? AND lease_token = ? AND page_offset = ?", job.ID, "running", job.LeaseToken, job.PageOffset).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrFanSyncLeaseLost
		}
		return nil
	})
	return finished, err
}

func (s *Store) FinishEmptyFanSyncJob(ctx context.Context, job *MpFanSyncJob, total int) error {
	if job == nil || job.LeaseToken == "" {
		return ErrFanSyncLeaseLost
	}
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current MpFanSyncJob
		q := tx.Where(
			"id = ? AND status = ? AND lease_token = ? AND lease_until > ?",
			job.ID, "running", job.LeaseToken, now,
		)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFanSyncLeaseLost
			}
			return err
		}
		reconcileAt := now
		if current.StartedAt != nil && !current.StartedAt.IsZero() {
			reconcileAt = *current.StartedAt
		}
		if err := validateFanSyncCompletion(current.ListedCount, total, current.SyncedCount); err != nil {
			return err
		}
		if err := markMissingFansUnsubscribedTx(tx, current.TenantID, current.AccountID, current.ID, reconcileAt, now); err != nil {
			return err
		}
		res := tx.Model(&MpFanSyncJob{}).
			Where("id = ? AND status = ? AND lease_token = ?", job.ID, "running", job.LeaseToken).
			Updates(map[string]any{
				"status": "succeeded", "active_key": nil, "total_count": total,
				"cursor": "", "next_cursor": "", "page_open_ids": "", "page_offset": 0,
				"last_error": "", "lease_until": nil, "lease_token": "", "finished_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrFanSyncLeaseLost
		}
		return nil
	})
}

// validateFanSyncCompletion requires stable counts across the full scan. A
// changing upstream total can otherwise leave an early-page unsubscribe
// marked as present when its callback was missed. Failing the run is more
// honest than publishing a known-stale "succeeded" result.
func validateFanSyncCompletion(listed, total, synced int) error {
	if synced != listed {
		return fmt.Errorf("%w: listed=%d synced=%d", ErrFanSyncIncomplete, listed, synced)
	}
	if listed != total {
		return fmt.Errorf("%w: total=%d listed=%d", ErrFanSyncIncomplete, total, listed)
	}
	return nil
}

// markMissingFansUnsubscribedTx closes missed unsubscribe callbacks: every
// local fan absent from the completed upstream listing becomes unsubscribed.
// A callback at or after the job start keeps precedence through its state
// version, so completion cannot undo a concurrent subscribe event.
func markMissingFansUnsubscribedTx(
	tx *gorm.DB,
	tenantID, accountID, syncJobID uint64,
	reconcileAt, updatedAt time.Time,
) error {
	if reconcileAt.IsZero() {
		reconcileAt = updatedAt
	}
	reconcileAt = reconcileAt.Truncate(time.Microsecond)
	return tx.Model(&MpFan{}).
		Where("tenant_id = ? AND account_id = ? AND last_sync_job_id <> ?", EnsureTenant(tenantID), accountID, syncJobID).
		Where("subscribe_event_at IS NULL OR subscribe_event_at < ?", reconcileAt).
		Updates(map[string]any{
			"subscribed": false, "subscribe_event_at": reconcileAt, "updated_at": updatedAt,
		}).Error
}

func truncateFanSyncError(raw string) string {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) > 480 {
		runes = runes[:480]
	}
	return string(runes)
}

func (s *Store) RetryFanSyncJob(ctx context.Context, job *MpFanSyncJob, maxAttempts int, reason string) error {
	if job == nil || job.LeaseToken == "" {
		return ErrFanSyncLeaseLost
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	attempts := job.Attempts + 1
	now := time.Now()
	updates := map[string]any{
		"attempts": attempts, "last_error": truncateFanSyncError(reason),
		"lease_until": nil, "lease_token": "",
	}
	if attempts >= maxAttempts {
		updates["status"] = "failed"
		updates["active_key"] = nil
		updates["finished_at"] = now
	} else {
		shift := attempts
		if shift > 6 {
			shift = 6
		}
		updates["status"] = "pending"
		updates["next_at"] = now.Add(time.Duration(1<<uint(shift)) * time.Second)
	}
	res := s.db.WithContext(ctx).Model(&MpFanSyncJob{}).
		Where("id = ? AND status = ? AND lease_token = ?", job.ID, "running", job.LeaseToken).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrFanSyncLeaseLost
	}
	return nil
}

func (s *Store) FailFanSyncJob(ctx context.Context, job *MpFanSyncJob, reason string) error {
	if job == nil || job.LeaseToken == "" {
		return ErrFanSyncLeaseLost
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&MpFanSyncJob{}).
		Where("id = ? AND status = ? AND lease_token = ?", job.ID, "running", job.LeaseToken).
		Updates(map[string]any{
			"status": "failed", "active_key": nil, "last_error": truncateFanSyncError(reason),
			"lease_until": nil, "lease_token": "", "finished_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrFanSyncLeaseLost
	}
	return nil
}

// UpsertFan retains the single-row API used by callbacks/tests while using the
// same atomic conflict path as the batch worker.
func (s *Store) UpsertFan(f *MpFan) error {
	if f == nil {
		return errors.New("fan is nil")
	}
	return s.UpsertFansContext(context.Background(), []MpFan{*f}, time.Now(), f.LastSyncJobID)
}

func (s *Store) UpsertFansContext(ctx context.Context, fans []MpFan, snapshotAt time.Time, syncJobID uint64) error {
	return upsertFansTx(s.db.WithContext(ctx), fans, snapshotAt, syncJobID)
}

func upsertFansTx(db *gorm.DB, fans []MpFan, snapshotAt time.Time, syncJobID uint64) error {
	if len(fans) == 0 {
		return nil
	}
	if snapshotAt.IsZero() {
		snapshotAt = time.Now()
	}
	// PostgreSQL timestamps have microsecond precision. Truncation avoids a
	// near-boundary snapshot rounding into the next second and defeating the
	// conservative callback ordering rule.
	snapshotAt = snapshotAt.Truncate(time.Microsecond)
	for i := range fans {
		fans[i].TenantID = EnsureTenant(fans[i].TenantID)
		fans[i].OpenID = strings.TrimSpace(fans[i].OpenID)
		fans[i].LastSyncJobID = syncJobID
		fans[i].SubscribeEventAt = &snapshotAt
		if fans[i].AccountID == 0 || fans[i].OpenID == "" {
			return errors.New("fan account_id/open_id is required")
		}
	}
	now := time.Now()
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "open_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"nickname":         gorm.Expr("excluded.nickname"),
			"avatar":           gorm.Expr("excluded.avatar"),
			"sex":              gorm.Expr("excluded.sex"),
			"city":             gorm.Expr("excluded.city"),
			"province":         gorm.Expr("excluded.province"),
			"country":          gorm.Expr("excluded.country"),
			"tags":             gorm.Expr("excluded.tags"),
			"last_sync_job_id": gorm.Expr("excluded.last_sync_job_id"),
			"subscribed": gorm.Expr(
				"CASE WHEN mp_fans.subscribe_event_at IS NULL OR mp_fans.subscribe_event_at < excluded.subscribe_event_at THEN excluded.subscribed ELSE mp_fans.subscribed END",
			),
			"subscribe_time": gorm.Expr(
				"CASE WHEN mp_fans.subscribe_event_at IS NULL OR mp_fans.subscribe_event_at < excluded.subscribe_event_at THEN excluded.subscribe_time ELSE mp_fans.subscribe_time END",
			),
			"subscribe_event_at": gorm.Expr(
				"CASE WHEN mp_fans.subscribe_event_at IS NULL OR mp_fans.subscribe_event_at < excluded.subscribe_event_at THEN excluded.subscribe_event_at ELSE mp_fans.subscribe_event_at END",
			),
			"updated_at": now,
		}),
	}).Create(&fans).Error
}
