package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
	"github.com/go-admin-kit/services/mp/internal/wxclient"
	"gorm.io/gorm"
)

const (
	fanSyncJobLease       = 2 * time.Minute
	fanSyncJobMaxAttempts = 8
	fanSyncBatchDelay     = 200 * time.Millisecond
)

type fanSyncJobResponse struct {
	ID          uint64     `json:"id"`
	AccountID   uint64     `json:"account_id"`
	Status      string     `json:"status"`
	TotalCount  int        `json:"total_count"`
	ListedCount int        `json:"listed_count"`
	SyncedCount int        `json:"synced_count"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	NextAt      *time.Time `json:"next_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func fanSyncJobData(job *store.MpFanSyncJob) *fanSyncJobResponse {
	if job == nil {
		return nil
	}
	var nextAt *time.Time
	if job.Status == "pending" {
		next := job.NextAt
		nextAt = &next
	}
	return &fanSyncJobResponse{
		ID: job.ID, AccountID: job.AccountID, Status: job.Status,
		TotalCount: job.TotalCount, ListedCount: job.ListedCount, SyncedCount: job.SyncedCount,
		Attempts: job.Attempts, LastError: job.LastError, NextAt: nextAt,
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
}

// SyncFans now only enqueues durable work. Repeated requests for the same
// tenant/account return the active job instead of duplicating WeChat traffic.
func (s *Server) SyncFans(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.wxAccount(c, u.TenantID)
	if !ok2 {
		return
	}
	job, reused, err := s.Store.EnqueueFanSyncJob(c.Request.Context(), u.TenantID, acc.ID, u.UserID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "创建粉丝同步任务失败")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"code": 200, "message": "success",
		"data": gin.H{"job": fanSyncJobData(job), "reused": reused},
	})
}

// GetFanSync returns only the latest task owned by the requested tenant and
// account. Cursor/page buffers never leave the service.
func (s *Server) GetFanSync(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	acc, ok2 := s.account(c, u.TenantID)
	if !ok2 {
		return
	}
	job, err := s.Store.LatestFanSyncJob(c.Request.Context(), u.TenantID, acc.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "读取粉丝同步任务失败")
		return
	}
	ok(c, gin.H{"job": fanSyncJobData(job)})
}

type terminalFanSyncError struct{ message string }

func (e *terminalFanSyncError) Error() string { return e.message }

func terminalFanSync(message string) error { return &terminalFanSyncError{message: message} }

// ReconcileFanSyncJobs executes at most one upstream page/batch step per
// claimed job. Every step commits its cursor/progress before releasing the
// lease, so shutdown and multi-replica recovery are deterministic.
func (s *Server) ReconcileFanSyncJobs(ctx context.Context, limit int) error {
	jobs, err := s.Store.ClaimFanSyncJobs(ctx, limit, fanSyncJobLease)
	if err != nil {
		return err
	}
	var resultErr error
	for i := range jobs {
		job := &jobs[i]
		stepErr := s.processFanSyncJob(ctx, job)
		if stepErr == nil {
			continue
		}
		if errors.Is(stepErr, context.Canceled) || errors.Is(stepErr, context.DeadlineExceeded) {
			return errors.Join(resultErr, stepErr)
		}
		if errors.Is(stepErr, store.ErrFanSyncLeaseLost) {
			resultErr = errors.Join(resultErr, fmt.Errorf("fan sync job %d lease lost", job.ID))
			continue
		}
		if errors.Is(stepErr, store.ErrFanSyncIncomplete) {
			if failErr := s.Store.FailFanSyncJob(ctx, job, "微信粉丝列表在分页期间发生变化，请重新同步"); failErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("fail incomplete fan sync job %d: %w", job.ID, failErr))
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("fan sync job %d: %w", job.ID, stepErr))
			continue
		}
		var terminal *terminalFanSyncError
		if errors.As(stepErr, &terminal) {
			if failErr := s.Store.FailFanSyncJob(ctx, job, terminal.Error()); failErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("fail fan sync job %d: %w", job.ID, failErr))
			}
			continue
		}
		if retryErr := s.Store.RetryFanSyncJob(ctx, job, fanSyncJobMaxAttempts, stepErr.Error()); retryErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("retry fan sync job %d: %w", job.ID, retryErr))
		}
		resultErr = errors.Join(resultErr, fmt.Errorf("fan sync job %d: %w", job.ID, stepErr))
	}
	return resultErr
}

func (s *Server) processFanSyncJob(ctx context.Context, job *store.MpFanSyncJob) error {
	if job == nil {
		return terminalFanSync("粉丝同步任务无效")
	}
	account, err := s.Store.GetAccountContext(ctx, job.AccountID, job.TenantID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return terminalFanSync("公众号账号不存在")
	}
	if err != nil {
		return fmt.Errorf("读取公众号账号失败: %w", err)
	}
	if account.Status != "active" {
		return terminalFanSync("公众号账号已停用")
	}
	if s.WX == nil {
		return errors.New("微信客户端未就绪")
	}

	if job.PageOpenIDs == "" {
		return s.stageNextFanPage(ctx, job, account)
	}

	var openIDs []string
	if err := json.Unmarshal([]byte(job.PageOpenIDs), &openIDs); err != nil {
		return terminalFanSync("粉丝同步页缓冲损坏")
	}
	if len(openIDs) == 0 || job.PageOffset < 0 || job.PageOffset >= len(openIDs) {
		return terminalFanSync("粉丝同步页进度无效")
	}
	end := job.PageOffset + wxclient.MaxBatchUserInfo
	if end > len(openIDs) {
		end = len(openIDs)
	}
	batch := openIDs[job.PageOffset:end]
	snapshotAt := time.Now()
	profiles, err := s.WX.BatchGetUserInfo(ctx, account, batch)
	if err != nil {
		return fmt.Errorf("批量读取微信粉丝详情失败: %w", err)
	}
	profileByOpenID, err := validateFanProfiles(batch, profiles)
	if err != nil {
		return err
	}
	fans := make([]store.MpFan, 0, len(batch))
	for _, openID := range batch {
		info := profileByOpenID[openID]
		var subscribeTime *time.Time
		if info.SubscribeTime > 0 {
			t := time.Unix(info.SubscribeTime, 0)
			subscribeTime = &t
		}
		fans = append(fans, store.MpFan{
			TenantID: job.TenantID, AccountID: job.AccountID, OpenID: openID,
			Nickname: info.Nickname, Avatar: info.HeadImgURL, Sex: info.Sex,
			City: info.City, Province: info.Province, Country: info.Country,
			Tags: formatFanTags(info.TagIDList), Subscribed: info.Subscribe == 1,
			SubscribeTime: subscribeTime,
		})
	}
	_, err = s.Store.CommitFanSyncBatch(ctx, job, fans, snapshotAt, len(openIDs), fanSyncBatchDelay)
	if err != nil {
		return fmt.Errorf("提交粉丝同步批次失败: %w", err)
	}
	return nil
}

func (s *Server) stageNextFanPage(ctx context.Context, job *store.MpFanSyncJob, account *store.MpAccount) error {
	page, err := s.WX.ListFanPage(ctx, account, job.Cursor)
	if err != nil {
		return fmt.Errorf("读取微信粉丝列表失败: %w", err)
	}
	if page.Total < 0 || page.Count < 0 || page.Count != len(page.OpenIDs) {
		return terminalFanSync("微信粉丝列表计数无效")
	}
	if page.NextOpenID != "" && page.NextOpenID == job.Cursor {
		return terminalFanSync("微信粉丝列表游标未前进")
	}
	openIDs, err := validateFanOpenIDs(page.OpenIDs)
	if err != nil {
		return err
	}
	if len(openIDs) == 0 {
		if page.NextOpenID != "" {
			return terminalFanSync("微信粉丝列表空页仍返回下一游标")
		}
		return s.Store.FinishEmptyFanSyncJob(ctx, job, page.Total)
	}
	if page.Total < len(openIDs) {
		return terminalFanSync("微信粉丝总数小于当前页数量")
	}
	if err := s.Store.StageFanSyncPage(ctx, job, page.Total, openIDs, page.NextOpenID); err != nil {
		return fmt.Errorf("保存粉丝同步页失败: %w", err)
	}
	return nil
}

func validateFanOpenIDs(raw []string) ([]string, error) {
	openIDs := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		openID := strings.TrimSpace(value)
		if openID == "" {
			return nil, terminalFanSync("微信粉丝列表包含空 OpenID")
		}
		if _, exists := seen[openID]; exists {
			return nil, terminalFanSync("微信粉丝列表包含重复 OpenID")
		}
		seen[openID] = struct{}{}
		openIDs = append(openIDs, openID)
	}
	return openIDs, nil
}

func validateFanProfiles(requested []string, profiles []wxclient.WxUserInfo) (map[string]wxclient.WxUserInfo, error) {
	if len(profiles) != len(requested) {
		return nil, fmt.Errorf("微信批量粉丝详情数量不一致: requested=%d returned=%d", len(requested), len(profiles))
	}
	wanted := make(map[string]struct{}, len(requested))
	for _, openID := range requested {
		wanted[openID] = struct{}{}
	}
	result := make(map[string]wxclient.WxUserInfo, len(profiles))
	for _, profile := range profiles {
		openID := strings.TrimSpace(profile.OpenID)
		if _, exists := wanted[openID]; !exists {
			return nil, errors.New("微信批量粉丝详情返回未知 OpenID")
		}
		if _, duplicate := result[openID]; duplicate {
			return nil, errors.New("微信批量粉丝详情返回重复 OpenID")
		}
		result[openID] = profile
	}
	return result, nil
}

func formatFanTags(tagIDs []int64) string {
	if len(tagIDs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		parts = append(parts, strconv.FormatInt(tagID, 10))
	}
	return strings.Join(parts, ",")
}
