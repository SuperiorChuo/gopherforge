package api

// 粉丝列表：mp_fans（tenant 隔离，按 id desc）。本阶段数据可能为空，
// M4 才从微信用户接口拉取粉丝档案。

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) ListFans(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	page, pageSize := pageOf(c)
	list, total, err := s.Store.ListFans(
		u.TenantID,
		queryUint64(c, "account_id"),
		c.Query("keyword"),
		page, pageSize,
	)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}
