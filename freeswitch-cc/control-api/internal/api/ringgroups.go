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

// 响铃组：DB → 45_rg-<name>.xml 拨号计划物化 + reloadxml。拨接入号同振/顺振
// 一组分机，全都没接转 FailoverTarget（空则挂断）。

var ringGroupNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

// rgExtRe 响铃组成员分机：2-16 位数字。
var rgExtRe = regexp.MustCompile(`^\d{2,16}$`)

var rgStrategies = map[string]bool{
	"simultaneous": true, "sequential": true,
}

func (s *Server) syncRingGroups() error {
	groups, err := s.Store.ListRingGroups()
	if err != nil {
		return err
	}
	return extdir.SyncRingGroups(s.Cfg.DialplanDir, groups)
}

type ringGroupReq struct {
	Name           string `json:"name"`
	AccessNumber   string `json:"access_number"`
	Strategy       string `json:"strategy"`
	Exts           string `json:"exts"`
	RingTimeout    int    `json:"ring_timeout"`
	FailoverTarget string `json:"failover_target"`
}

// validate 校验并转 store 模型；非法时已写响应并返回 nil。
func (r *ringGroupReq) validate(c *gin.Context) *store.CCRingGroup {
	bad := func(msg string) *store.CCRingGroup {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return nil
	}
	r.Name = strings.TrimSpace(r.Name)
	r.AccessNumber = strings.TrimSpace(r.AccessNumber)
	r.Strategy = strings.TrimSpace(r.Strategy)
	r.Exts = strings.ReplaceAll(strings.TrimSpace(r.Exts), " ", "")
	r.FailoverTarget = strings.TrimSpace(r.FailoverTarget)
	if r.Strategy == "" {
		r.Strategy = "simultaneous"
	}
	if r.RingTimeout == 0 {
		r.RingTimeout = 20
	}
	exts := splitCSV(r.Exts)
	switch {
	case !ringGroupNameRe.MatchString(r.Name):
		return bad("name must be 2-32 lowercase letters/digits/_-")
	case !ccAccessNumberRe.MatchString(r.AccessNumber) || reservedExt(r.AccessNumber):
		return bad("access_number must be 3-6 digits and not reserved")
	case !rgStrategies[r.Strategy]:
		return bad("strategy must be simultaneous or sequential")
	case len(exts) == 0:
		return bad("exts required (comma-separated extension list)")
	case r.RingTimeout < 5 || r.RingTimeout > 600:
		return bad("ring_timeout must be 5-600")
	case r.FailoverTarget != "" && !numberRe.MatchString(r.FailoverTarget):
		return bad("invalid failover_target")
	}
	for _, e := range exts {
		if !rgExtRe.MatchString(e) {
			return bad("each ext must be 2-16 digits")
		}
	}
	// 归一化：去空格后重新拼，保证物化用的字符串干净
	return &store.CCRingGroup{
		Name: r.Name, AccessNumber: r.AccessNumber, Strategy: r.Strategy,
		Exts: strings.Join(exts, ","), RingTimeout: r.RingTimeout,
		FailoverTarget: r.FailoverTarget,
	}
}

// splitCSV 拆逗号分隔并去空段。
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// listRingGroups handles GET /v1/ring-groups.
func (s *Server) listRingGroups(c *gin.Context) {
	groups, err := s.Store.ListRingGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"list": groups, "total": len(groups)})
}

// createRingGroup handles POST /v1/ring-groups.
func (s *Server) createRingGroup(c *gin.Context) {
	var req ringGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	g := req.validate(c)
	if g == nil {
		return
	}
	if err := s.Store.CreateRingGroup(g); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "ring-group name or access_number exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncRingGroups(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// updateRingGroup handles PUT /v1/ring-groups/:name.
func (s *Server) updateRingGroup(c *gin.Context) {
	name := c.Param("name")
	existing, err := s.Store.GetRingGroup(name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ring-group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req ringGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Name = name // 名字不可改（拨号计划文件名以名字命名）
	g := req.validate(c)
	if g == nil {
		return
	}
	g.ID = existing.ID
	g.CreatedAt = existing.CreatedAt
	if err := s.Store.UpdateRingGroup(g); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "access_number already used by another group"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncRingGroups(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}

// deleteRingGroup handles DELETE /v1/ring-groups/:name.
func (s *Server) deleteRingGroup(c *gin.Context) {
	name := c.Param("name")
	if !ringGroupNameRe.MatchString(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	if err := s.Store.DeleteRingGroup(name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ring-group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncRingGroups(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "reloaded": s.reloadXML()})
}
