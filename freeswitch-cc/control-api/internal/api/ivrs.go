package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/extdir"
	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// IVR 菜单：DB → 菜单 XML + 接入号拨号计划物化 → reloadxml 生效。

var (
	ivrNameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
	ivrDigitsRe = regexp.MustCompile(`^[0-9*#]$`)
	ivrTargetRe = regexp.MustCompile(`^[0-9*#]{2,16}$`)
	// 放音路径或 tone_stream；禁 XML 特殊字符与空白注入
	ivrGreetingRe = regexp.MustCompile(`^[A-Za-z0-9_\-./:%(),]{0,200}$`)
	// 收号提示音：走冒号分隔编码，故不含 ':'（提示音库文件名/相对路径即可）
	ivrCollectPromptRe = regexp.MustCompile(`^[A-Za-z0-9_\-./%(),]{0,200}$`)
)

type ivrReq struct {
	Name         string            `json:"name" binding:"required"`
	AccessNumber string            `json:"access_number" binding:"required"`
	Greeting     string            `json:"greeting"`
	TimeoutMS    int               `json:"timeout_ms"`
	Entries      []extdir.IVREntry `json:"entries" binding:"required"`
}

func (s *Server) validateIVR(c *gin.Context, req *ivrReq) bool {
	switch {
	case !ivrNameRe.MatchString(req.Name):
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must be 2-32 lowercase letters/digits/_-"})
		return false
	case !numberRe.MatchString(req.AccessNumber):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid access_number"})
		return false
	case reservedExt(req.AccessNumber):
		c.JSON(http.StatusBadRequest, gin.H{"error": "access_number is reserved"})
		return false
	case !ivrGreetingRe.MatchString(req.Greeting):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid greeting"})
		return false
	case len(req.Entries) == 0 || len(req.Entries) > 12:
		c.JSON(http.StatusBadRequest, gin.H{"error": "entries must have 1-12 items"})
		return false
	}
	seen := map[string]bool{}
	for _, e := range req.Entries {
		if !ivrDigitsRe.MatchString(e.Digits) || seen[e.Digits] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "entry digits must be unique single [0-9*#]"})
			return false
		}
		seen[e.Digits] = true
		switch e.Action {
		case "transfer":
			if !ivrTargetRe.MatchString(e.Target) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transfer target"})
				return false
			}
		case "submenu":
			if !ivrNameRe.MatchString(e.Target) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submenu name"})
				return false
			}
			if e.Target != req.Name {
				if _, err := s.Store.GetIVRMenu(e.Target); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "submenu not found: " + e.Target})
					return false
				}
			}
		case "replay", "back", "hangup":
			// no target
		case "collect":
			if !s.validateCollect(c, e.Target) {
				return false
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "action must be transfer/submenu/replay/back/hangup/collect"})
			return false
		}
	}
	return true
}

// validateCollect 校验收号参数串 "min:max:timeout:prompt:next"：
// min/max 1-32 位且 min<=max；timeout 1000-60000ms；prompt 走放音路径白名单
// （可空）；next 是有效转接号码。
func (s *Server) validateCollect(c *gin.Context, target string) bool {
	p, okParse := extdir.ParseCollect(target)
	if !okParse {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collect target must be min:max:timeout:prompt:next"})
		return false
	}
	switch {
	case p.Min < 1 || p.Min > 32 || p.Max < 1 || p.Max > 32 || p.Min > p.Max:
		c.JSON(http.StatusBadRequest, gin.H{"error": "collect digits length must be 1-32 and min<=max"})
		return false
	case p.Timeout < 1000 || p.Timeout > 60000:
		c.JSON(http.StatusBadRequest, gin.H{"error": "collect timeout must be 1000-60000 ms"})
		return false
	case p.Prompt != "" && !ivrCollectPromptRe.MatchString(p.Prompt):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collect prompt path"})
		return false
	case !ivrTargetRe.MatchString(p.Next):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collect next number"})
		return false
	}
	return true
}

func (s *Server) ivrFromReq(req *ivrReq) *store.IVRMenu {
	raw, _ := json.Marshal(req.Entries)
	return &store.IVRMenu{
		Name:         req.Name,
		AccessNumber: req.AccessNumber,
		Greeting:     req.Greeting,
		TimeoutMS:    req.TimeoutMS,
		Entries:      string(raw),
	}
}

// listIVRs handles GET /v1/ivrs.
func (s *Server) listIVRs(c *gin.Context) {
	list, err := s.Store.ListIVRMenus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type view struct {
		store.IVRMenu
		EntryList []extdir.IVREntry `json:"entry_list"`
	}
	out := make([]view, 0, len(list))
	for _, m := range list {
		entries, _ := extdir.ParseIVREntries(m.Entries)
		out = append(out, view{IVRMenu: m, EntryList: entries})
	}
	c.JSON(http.StatusOK, gin.H{"list": out, "total": len(out)})
}

// createIVR handles POST /v1/ivrs.
func (s *Server) createIVR(c *gin.Context) {
	var req ivrReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/access_number/entries required"})
		return
	}
	if !s.validateIVR(c, &req) {
		return
	}
	m := s.ivrFromReq(&req)
	if err := s.Store.CreateIVRMenu(m); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "name or access_number already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.WriteIVR(s.Cfg.IVRDir, s.Cfg.DialplanDir, *m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": m.Name, "reloaded": s.reloadXML()})
}

// updateIVR handles PUT /v1/ivrs/:name.
func (s *Server) updateIVR(c *gin.Context) {
	name := c.Param("name")
	if !ivrNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	existing, err := s.Store.GetIVRMenu(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ivr not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req ivrReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name // 名称不可改（文件名/菜单名锚点）
	if !s.validateIVR(c, &req) {
		return
	}
	oldAccess := existing.AccessNumber
	m := s.ivrFromReq(&req)
	m.ID = existing.ID
	m.CreatedAt = existing.CreatedAt
	if err := s.Store.UpdateIVRMenu(m); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "access_number already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = oldAccess // 接入号变更由 WriteIVR 整文件覆盖，无需单独清理
	if err := extdir.WriteIVR(s.Cfg.IVRDir, s.Cfg.DialplanDir, *m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize xml: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// deleteIVR handles DELETE /v1/ivrs/:name.
func (s *Server) deleteIVR(c *gin.Context) {
	name := c.Param("name")
	if !ivrNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if err := s.Store.DeleteIVRMenu(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ivr not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := extdir.RemoveIVR(s.Cfg.IVRDir, s.Cfg.DialplanDir, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "remove xml: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}
