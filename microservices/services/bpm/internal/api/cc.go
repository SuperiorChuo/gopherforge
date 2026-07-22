package api

// 抄送端 handler（M2）：抄送我的列表 / 标记已读。
// 抄送记录本人天然可见，不设权限码（同任务动作口径，设计文档 Q6）。

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/store"
)

// MyCc handles GET /api/v1/bpm/cc/my?unread_only=&page=&page_size=
func (s *Server) MyCc(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	unreadOnly := c.Query("unread_only") == "true" || c.Query("unread_only") == "1"
	p := pageOf(c)
	list, total, err := s.Store.ListMyCc(u.TenantID, u.UserID, unreadOnly, p)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": p.Page, "page_size": p.PageSize})
}

// ReadCc handles POST /api/v1/bpm/cc/:id/read — 仅本人可标已读，幂等。
func (s *Server) ReadCc(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	if err := s.Store.MarkCcRead(id, u.TenantID, u.UserID); err != nil {
		if errors.Is(err, store.ErrNotCcOwner) {
			fail(c, http.StatusForbidden, err.Error())
			return
		}
		notFoundOr(c, err, "抄送记录不存在")
		return
	}
	ok(c, gin.H{"id": id, "read": true})
}
