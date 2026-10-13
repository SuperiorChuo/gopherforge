package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 营业时间路由：DB → 15_time-<name>.xml 拨号计划物化 + reloadxml。
// 拨接入号先命中时间路由（前缀 15 抢跑），营业转 OpenTarget、非营业转
// ClosedTarget（空则播放提示音后挂断）。

var timeRouteNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// ccAccessNumberRe 接入号：3-6 位数字（时间路由/响铃组共用）。
var ccAccessNumberRe = regexp.MustCompile(`^[0-9]{3,6}$`)

// wdayRe FS wday 语法：数字/逗号/连字符，如 "2-6" 或 "2,4,6"。
var wdayRe = regexp.MustCompile(`^[0-7,\-]+$`)

// timeRangesRe 逗号分隔的 HH:mm-HH:mm 多段。
var timeRangesRe = regexp.MustCompile(`^\d{2}:\d{2}-\d{2}:\d{2}(,\d{2}:\d{2}-\d{2}:\d{2})*$`)

func (s *Server) syncTimeRoutes() error {
	routes, err := s.Store.ListTimeRoutes()
	if err != nil {
		return err
	}
	return extdir.SyncTimeRoutes(s.Cfg.DialplanDir, routes)
}

type timeRouteReq struct {
	Name         string `json:"name"`
	AccessNumber string `json:"access_number"`
	Wdays        string `json:"wdays"`
	TimeRanges   string `json:"time_ranges"`
	OpenTarget   string `json:"open_target"`
	ClosedTarget string `json:"closed_target"`
	ClosedPrompt string `json:"closed_prompt"`
}

// validate 校验并转 store 模型；非法时已写响应并返回 nil。
func (r *timeRouteReq) validate(c *gin.Context) *store.CCTimeRoute {
	bad := func(msg string) *store.CCTimeRoute {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return nil
	}
	r.Name = strings.TrimSpace(r.Name)
	r.AccessNumber = strings.TrimSpace(r.AccessNumber)
	r.Wdays = strings.TrimSpace(r.Wdays)
	r.TimeRanges = strings.ReplaceAll(strings.TrimSpace(r.TimeRanges), " ", "")
	r.OpenTarget = strings.TrimSpace(r.OpenTarget)
	r.ClosedTarget = strings.TrimSpace(r.ClosedTarget)
	r.ClosedPrompt = strings.TrimSpace(r.ClosedPrompt)
	switch {
	case !timeRouteNameRe.MatchString(r.Name):
		return bad("name must be 2-32 lowercase letters/digits/_-")
	case !ccAccessNumberRe.MatchString(r.AccessNumber) || reservedExt(r.AccessNumber):
		return bad("access_number must be 3-6 digits and not reserved")
	case !wdayRe.MatchString(r.Wdays):
		return bad("wdays must be FS wday syntax like 2-6 or 2,4,6")
	case !timeRangesRe.MatchString(r.TimeRanges):
		return bad("time_ranges must be HH:MM-HH:MM[,HH:MM-HH:MM]")
	case !numberRe.MatchString(r.OpenTarget):
		return bad("open_target required (queue/IVR/ext/ring-group access number)")
	case r.ClosedTarget != "" && !numberRe.MatchString(r.ClosedTarget):
		return bad("invalid closed_target")
	case r.ClosedPrompt != "" && !ivrGreetingRe.MatchString(r.ClosedPrompt):
		return bad("invalid closed_prompt")
	}
	return &store.CCTimeRoute{
		Name: r.Name, AccessNumber: r.AccessNumber, Wdays: r.Wdays,
		TimeRanges: r.TimeRanges, OpenTarget: r.OpenTarget,
		ClosedTarget: r.ClosedTarget, ClosedPrompt: r.ClosedPrompt,
	}
}

// listTimeRoutes handles GET /v1/time-routes.
func (s *Server) listTimeRoutes(c *gin.Context) {
	routes, err := s.Store.ListTimeRoutes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"list": routes, "total": len(routes)})
}

// createTimeRoute handles POST /v1/time-routes.
func (s *Server) createTimeRoute(c *gin.Context) {
	var req timeRouteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	t := req.validate(c)
	if t == nil {
		return
	}
	if err := s.Store.CreateTimeRoute(t); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "time-route name or access_number exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncTimeRoutes(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// updateTimeRoute handles PUT /v1/time-routes/:name.
func (s *Server) updateTimeRoute(c *gin.Context) {
	name := c.Param("name")
	existing, err := s.Store.GetTimeRoute(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "time-route not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req timeRouteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name // 名字不可改（拨号计划文件名以名字命名）
	t := req.validate(c)
	if t == nil {
		return
	}
	t.ID = existing.ID
	t.CreatedAt = existing.CreatedAt
	if err := s.Store.UpdateTimeRoute(t); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "access_number already used by another route"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncTimeRoutes(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// deleteTimeRoute handles DELETE /v1/time-routes/:name.
func (s *Server) deleteTimeRoute(c *gin.Context) {
	name := c.Param("name")
	if !timeRouteNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if err := s.Store.DeleteTimeRoute(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "time-route not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncTimeRoutes(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}
