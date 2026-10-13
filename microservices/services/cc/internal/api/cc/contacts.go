package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/cc/internal/store/cc"
)

// 来显归属：联系人库 CRUD（鉴权组）+ 开放 cid 反查端点（mod_cidlookup 直连，
// 裸文本返回，不走 envelope/ForwardAuth）。

// ListContacts handles GET /api/v1/cc/contacts.
func (s *Server) ListContacts(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := s.Store.ListContacts(c.Request.Context(), agent.TenantID, c.Query("q"), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, "list contacts failed")
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

type contactReq struct {
	Number string `json:"number"`
	Name   string `json:"name"`
	Remark string `json:"remark"`
}

// UpsertContact handles POST /api/v1/cc/contacts — 话单页"备注姓名"按号码 upsert。
func (s *Server) UpsertContact(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	var req contactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "number and name required")
		return
	}
	saved, err := s.Store.UpsertContact(c.Request.Context(), &store.Contact{
		TenantID: agent.TenantID,
		Number:   req.Number,
		Name:     req.Name,
		Remark:   req.Remark,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, saved)
}

// DeleteContact handles DELETE /api/v1/cc/contacts/:number.
func (s *Server) DeleteContact(c *gin.Context) {
	agent, authorized := s.requireAgent(c)
	if !authorized {
		return
	}
	deleted, err := s.Store.DeleteContact(c.Request.Context(), agent.TenantID, c.Param("number"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "delete contact failed")
		return
	}
	if !deleted {
		fail(c, http.StatusNotFound, "contact not found")
		return
	}
	ok(c, gin.H{"deleted": true})
}

// CidLookup handles GET /api/v1/cc/cid?number=... — mod_cidlookup 直连的开放端点。
// ★ 返回契约与其它端点不同：命中返回 200 + 纯文本客户名（非 JSON——mod_cidlookup
// 不解 JSON，会把整段响应当名字）；未命中返回 404，模块不改名。cid 端点不带租户
// 上下文（FS 无鉴权头），默认租户 1 查询。
func (s *Server) CidLookup(c *gin.Context) {
	number := c.Query("number")
	name, err := s.Store.LookupContactName(c.Request.Context(), 1, number)
	if err != nil {
		c.String(http.StatusInternalServerError, "")
		return
	}
	if name == "" {
		c.String(http.StatusNotFound, "")
		return
	}
	c.String(http.StatusOK, name)
}
