package system

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// NoticeAPI 通知公告 API
type NoticeAPI struct {
	noticeService system.NoticeService
}

// NewNoticeAPI 创建 NoticeAPI 实例
func NewNoticeAPI() *NoticeAPI {
	return &NoticeAPI{
		noticeService: system.NoticeService{},
	}
}

// GetNoticeList 获取公告列表
func (a *NoticeAPI) GetNoticeList(c *gin.Context) {
	var req system.NoticeListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	// 解析类型参数
	if typeStr := c.Query("type"); typeStr != "" {
		t, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			tInt8 := int8(t)
			req.Type = &tInt8
		}
	}

	// 解析状态参数
	if statusStr := c.Query("status"); statusStr != "" {
		s, err := strconv.ParseInt(statusStr, 10, 8)
		if err == nil {
			sInt8 := int8(s)
			req.Status = &sInt8
		}
	}

	notices, total, err := a.noticeService.GetList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, gin.H{
		"list":      notices,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// GetActiveNotices 获取有效的公告列表（前台展示用）
func (a *NoticeAPI) GetActiveNotices(c *gin.Context) {
	var noticeType *int8
	if typeStr := c.Query("type"); typeStr != "" {
		t, err := strconv.ParseInt(typeStr, 10, 8)
		if err == nil {
			tInt8 := int8(t)
			noticeType = &tInt8
		}
	}

	notices, err := a.noticeService.GetActiveList(noticeType)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, notices)
}

// GetNotice 获取公告详情
func (a *NoticeAPI) GetNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的公告ID")
		return
	}

	notice, err := a.noticeService.GetByID(uint(id))
	if err != nil {
		response.NotFound(c, "公告不存在")
		return
	}

	response.Success(c, notice)
}

// CreateNotice 创建公告
func (a *NoticeAPI) CreateNotice(c *gin.Context) {
	var req system.CreateNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 获取当前用户信息
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	creatorID := uint(0)
	creatorName := "系统"
	if userID != nil {
		creatorID = userID.(uint)
	}
	if username != nil {
		creatorName = username.(string)
	}

	notice, err := a.noticeService.Create(req, creatorID, creatorName)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "创建成功", notice)
}

// UpdateNotice 更新公告
func (a *NoticeAPI) UpdateNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的公告ID")
		return
	}

	var req system.UpdateNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	notice, err := a.noticeService.Update(uint(id), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "更新成功", notice)
}

// DeleteNotice 删除公告
func (a *NoticeAPI) DeleteNotice(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的公告ID")
		return
	}

	if err := a.noticeService.Delete(uint(id)); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "删除成功", nil)
}

// UpdateNoticeStatus 更新公告状态
func (a *NoticeAPI) UpdateNoticeStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "无效的公告ID")
		return
	}

	var req struct {
		Status int8 `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := a.noticeService.UpdateStatus(uint(id), req.Status); err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "状态更新成功", nil)
}
