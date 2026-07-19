package system

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/logger"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/model"
	"github.com/go-admin-kit/services/system/internal/service/system"
)

const noticeEmailTimeout = 5 * time.Second

type noticeEmailNotifier interface {
	SendNoticeEnabledContext(ctx context.Context, notice *model.Notice) error
}

// NoticeAPI handles notice endpoints.
type NoticeAPI struct {
	noticeService system.NoticeService
	broadcaster   *system.NotificationBroadcaster
	emailNotifier noticeEmailNotifier
}

// NewNoticeAPI creates a NoticeAPI instance.
func NewNoticeAPI() *NoticeAPI {
	return &NoticeAPI{
		noticeService: system.NoticeService{},
		broadcaster:   system.DefaultNotificationBroadcaster(),
		emailNotifier: system.DefaultNoticeEmailNotifier(),
	}
}

// NewNoticeAPIWithService creates a NoticeAPI instance from an injected
// service. Broadcaster and email notifier keep their default implementations.
func NewNoticeAPIWithService(noticeService system.NoticeService) *NoticeAPI {
	return &NoticeAPI{
		noticeService: noticeService,
		broadcaster:   system.DefaultNotificationBroadcaster(),
		emailNotifier: system.DefaultNoticeEmailNotifier(),
	}
}

// GetNoticeList returns paginated notices.
func (a *NoticeAPI) GetNoticeList(c *gin.Context) {
	var req system.NoticeListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	if typeStr := c.Query("type"); typeStr != "" {
		t, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			tInt8 := int8(t)
			req.Type = &tInt8
		}
	}

	if statusStr := c.Query("status"); statusStr != "" {
		s, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			sInt8 := int8(s)
			req.Status = &sInt8
		}
	}

	notices, total, err := a.noticeService.GetListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get notice list", err)
		return
	}

	response.Success(c, gin.H{
		"list":      notices,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetActiveNotices returns active notices for display.
func (a *NoticeAPI) GetActiveNotices(c *gin.Context) {
	var noticeType *int8
	if typeStr := c.Query("type"); typeStr != "" {
		t, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			tInt8 := int8(t)
			noticeType = &tInt8
		}
	}

	notices, err := a.noticeService.GetActiveListContext(c.Request.Context(), noticeType)
	if err != nil {
		internalServerError(c, "failed to get active notices", err)
		return
	}

	response.Success(c, notices)
}

// GetNotice returns a notice by id.
func (a *NoticeAPI) GetNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid notice id")
		return
	}

	notice, err := a.noticeService.GetByIDContext(c.Request.Context(), uint(id))
	if err != nil {
		writeSystemNoticeServiceError(c, "failed to get notice", err)
		return
	}

	response.Success(c, notice)
}

// CreateNotice creates a notice.
func (a *NoticeAPI) CreateNotice(c *gin.Context) {
	var req system.CreateNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	creatorID, creatorName := resolveNoticeCreator(c)

	notice, err := a.noticeService.CreateContext(c.Request.Context(), req, creatorID, creatorName)
	if err != nil {
		writeSystemNoticeServiceError(c, "failed to create notice", err)
		return
	}
	if notice.Status == 1 {
		a.notifyNoticeEnabled(context.WithoutCancel(c.Request.Context()), notice)
	}

	response.SuccessWithMessage(c, "notice created", notice)
}

// UpdateNotice updates a notice.
func (a *NoticeAPI) UpdateNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid notice id")
		return
	}

	var req system.UpdateNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	notice, err := a.noticeService.UpdateContext(c.Request.Context(), uint(id), req)
	if err != nil {
		writeSystemNoticeServiceError(c, "failed to update notice", err)
		return
	}
	if notice.Status == 1 {
		a.notifyNoticeEnabled(context.WithoutCancel(c.Request.Context()), notice)
	}

	response.SuccessWithMessage(c, "notice updated", notice)
}

// DeleteNotice deletes a notice.
func (a *NoticeAPI) DeleteNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid notice id")
		return
	}

	if err := a.noticeService.DeleteContext(c.Request.Context(), uint(id)); err != nil {
		writeSystemNoticeServiceError(c, "failed to delete notice", err)
		return
	}

	response.SuccessWithMessage(c, "notice deleted", nil)
}

// UpdateNoticeStatus updates a notice status.
func (a *NoticeAPI) UpdateNoticeStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid notice id")
		return
	}

	var req struct {
		Status int8 `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := a.noticeService.UpdateStatusContext(c.Request.Context(), uint(id), req.Status); err != nil {
		writeSystemNoticeServiceError(c, "failed to update notice status", err)
		return
	}
	if req.Status == 1 {
		if notice, err := a.noticeService.GetByIDContext(c.Request.Context(), uint(id)); err == nil {
			a.notifyNoticeEnabled(context.WithoutCancel(c.Request.Context()), notice)
		}
	}

	response.SuccessWithMessage(c, "notice status updated", nil)
}

func (a *NoticeAPI) notifyNoticeEnabled(ctx context.Context, notice *model.Notice) {
	a.publishNotice(ctx, notice)
	a.sendNoticeEmailAsync(ctx, notice)
}

func (a *NoticeAPI) publishNotice(ctx context.Context, notice *model.Notice) {
	broadcaster := a.broadcaster
	if broadcaster == nil {
		broadcaster = system.DefaultNotificationBroadcaster()
	}
	_ = broadcaster.PublishContext(ctx, system.NotificationMessageFromNotice(notice))
}

func (a *NoticeAPI) sendNoticeEmailAsync(ctx context.Context, notice *model.Notice) {
	if notice == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	noticeCopy := *notice
	go func() {
		emailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), noticeEmailTimeout)
		defer cancel()
		a.sendNoticeEmail(emailCtx, &noticeCopy)
	}()
}

func (a *NoticeAPI) sendNoticeEmail(ctx context.Context, notice *model.Notice) {
	notifier := a.emailNotifier
	if notifier == nil {
		notifier = system.DefaultNoticeEmailNotifier()
	}
	if err := notifier.SendNoticeEnabledContext(ctx, notice); err != nil && logger.Logger != nil {
		logger.Warn("notice email notification failed", logger.Uint("notice_id", notice.ID), logger.Err(err))
	}
}

func resolveNoticeCreator(c *gin.Context) (uint, string) {
	creatorID := uint(0)
	creatorName := "system"
	if userID, exists := c.Get("user_id"); exists {
		if typed, ok := userID.(uint); ok {
			creatorID = typed
		}
	}
	if username, exists := c.Get("username"); exists {
		if typed, ok := username.(string); ok && typed != "" {
			creatorName = typed
		}
	}
	return creatorID, creatorName
}
