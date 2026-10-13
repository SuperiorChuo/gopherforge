package api

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

const (
	publishResultJobLease       = 10 * time.Second
	publishResultJobMaxAttempts = 20
)

// applyPublishResultJob settles both legacy MpArticle and content-platform
// publications. The scoped UPDATE calls are idempotent, so retries are safe.
func (s *Server) applyPublishResultJob(job *store.MpPublishResultJob) (bool, error) {
	if job == nil || job.PublishID == "" || job.PublishStatus == "" {
		return false, errors.New("invalid publish result job")
	}
	status := "failed"
	if job.PublishStatus == "0" {
		status = "published"
	}

	articleMatched, articleErr := s.Store.UpdateArticleStatusByPublishID(
		job.TenantID, job.AccountID, job.PublishID, status,
	)
	publicationMatched := false
	var publicationErr error
	if s.Content != nil {
		publicationMatched, publicationErr = s.Content.UpdatePublicationStatusByExternalID(
			job.TenantID, "wechat", job.PublishID, job.AccountID, status,
		)
	}
	return articleMatched || publicationMatched, errors.Join(articleErr, publicationErr)
}

// ReconcilePublishResultJobs replays verified callbacks that arrived before
// their local article/publication record. Claiming uses SKIP LOCKED on
// PostgreSQL, allowing all Swarm replicas to run this loop safely.
func (s *Server) ReconcilePublishResultJobs(limit int) error {
	jobs, err := s.Store.ClaimPublishResultJobs(limit, publishResultJobLease)
	if err != nil {
		return err
	}
	var resultErr error
	for i := range jobs {
		job := &jobs[i]
		matched, applyErr := s.applyPublishResultJob(job)
		if applyErr == nil && matched {
			if finishErr := s.Store.FinishPublishResultJob(job.ID); finishErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("finish publish result job %d: %w", job.ID, finishErr))
			}
			continue
		}

		reason := "local publish record not found"
		if applyErr != nil {
			reason = applyErr.Error()
			resultErr = errors.Join(resultErr, fmt.Errorf("apply publish result job %d: %w", job.ID, applyErr))
		}
		nextAttempt := job.Attempts + 1
		if retryErr := s.Store.RetryPublishResultJob(
			job.ID, nextAttempt, publishResultJobMaxAttempts, reason,
		); retryErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("retry publish result job %d: %w", job.ID, retryErr))
			continue
		}
		if nextAttempt >= publishResultJobMaxAttempts {
			resultErr = errors.Join(resultErr, fmt.Errorf("publish result job %d exhausted retries: %s", job.ID, reason))
		}
	}
	return resultErr
}
