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

// 骚扰号黑名单：DB 事实源 + mod_blacklist 内存名单（运行时 add/del 即时生效）+
// 名单文件物化（重启自愈，blacklist.conf 指向该文件）。
// 拦截点在 public 入方向拨号计划：blacklist(check cc ${caller_id_number}) == true → 拒接。

var blNumberRe = regexp.MustCompile(`^\+?[0-9*#]{3,32}$`)

func (s *Server) syncBlacklist() error {
	list, err := s.Store.ListBlacklist()
	if err != nil {
		return err
	}
	numbers := make([]string, 0, len(list))
	for _, e := range list {
		numbers = append(numbers, e.Number)
	}
	return extdir.RenderBlacklist(s.Cfg.BlacklistDir, numbers)
}

// listBlacklist handles GET /v1/blacklist.
func (s *Server) listBlacklist(c *gin.Context) {
	list, err := s.Store.ListBlacklist()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"list": list, "total": len(list)})
}

type blacklistReq struct {
	Number string `json:"number" binding:"required"`
	Remark string `json:"remark"`
}

// addBlacklist handles POST /v1/blacklist.
func (s *Server) addBlacklist(c *gin.Context) {
	var req blacklistReq
	if err := c.ShouldBindJSON(&req); err != nil || !blNumberRe.MatchString(req.Number) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid number required"})
		return
	}
	if len(req.Remark) > 128 {
		req.Remark = req.Remark[:128]
	}
	e := &store.BlacklistEntry{Number: req.Number, Remark: req.Remark}
	if err := s.Store.CreateBlacklist(e); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "number already blacklisted"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncBlacklist(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	// 运行时即时生效
	applied := true
	if _, err := s.ESL.API("blacklist add cc " + e.Number); err != nil {
		applied = false
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}

// removeBlacklist handles DELETE /v1/blacklist/:number.
func (s *Server) removeBlacklist(c *gin.Context) {
	number := c.Param("number")
	if !blNumberRe.MatchString(number) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid number"})
		return
	}
	if err := s.Store.DeleteBlacklist(number); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not in blacklist"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.syncBlacklist(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "materialize: " + err.Error()})
		return
	}
	applied := true
	if _, err := s.ESL.API("blacklist del cc " + number); err != nil {
		applied = false
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "applied": applied})
}
