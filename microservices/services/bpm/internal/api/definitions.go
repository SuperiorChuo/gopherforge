package api

// 流程定义管理端 handler：CRUD / 发布 / 版本化 / 停用 / 按 key 取 active。

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/store"
	"gorm.io/gorm"
)

func notFoundOr(c *gin.Context, err error, msg string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, http.StatusNotFound, msg)
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}

func (s *Server) ListDefinitions(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	list, total, err := s.Store.ListDefinitions(u.TenantID, store.DefinitionFilter{
		Keyword: c.Query("keyword"),
		BizType: c.Query("biz_type"),
		Page:    pageOf(c),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total})
}

type definitionReq struct {
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	BizType    string          `json:"biz_type"`
	NodeTree   json.RawMessage `json:"node_tree"`
	FormSchema json.RawMessage `json:"form_schema"`
	Remark     string          `json:"remark"`
}

func (s *Server) CreateDefinition(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req definitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	d, err := s.Store.CreateDefinition(u.TenantID, store.CreateDefinitionInput{
		Key: req.Key, Name: req.Name, BizType: req.BizType,
		NodeTree: req.NodeTree, FormSchema: req.FormSchema,
		Remark: req.Remark, CreatedBy: u.UserID,
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, d)
}

func (s *Server) GetDefinition(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	d, err := s.Store.GetDefinition(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "流程定义不存在")
		return
	}
	ok(c, d)
}

type definitionUpdateReq struct {
	Name       *string         `json:"name"`
	BizType    *string         `json:"biz_type"`
	NodeTree   json.RawMessage `json:"node_tree"`
	FormSchema json.RawMessage `json:"form_schema"`
	Remark     *string         `json:"remark"`
}

func (s *Server) UpdateDefinition(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	var req definitionUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	d, err := s.Store.UpdateDefinition(id, u.TenantID, store.UpdateDefinitionInput{
		Name: req.Name, BizType: req.BizType, NodeTree: req.NodeTree,
		FormSchema: req.FormSchema, Remark: req.Remark,
	})
	if err != nil {
		notFoundOr(c, err, "流程定义不存在")
		return
	}
	ok(c, d)
}

func (s *Server) PublishDefinition(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	d, err := s.Store.Publish(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "流程定义不存在")
		return
	}
	ok(c, d)
}

func (s *Server) NewDefinitionVersion(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	d, err := s.Store.NewVersion(id, u.TenantID, u.UserID)
	if err != nil {
		notFoundOr(c, err, "流程定义不存在")
		return
	}
	ok(c, d)
}

func (s *Server) SuspendDefinition(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	d, err := s.Store.Suspend(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "流程定义不存在")
		return
	}
	ok(c, d)
}

func (s *Server) ActiveDefinitionByKey(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	d, err := s.Store.ActiveByKey(c.Param("key"), u.TenantID)
	if err != nil {
		notFoundOr(c, err, "流程没有已发布版本")
		return
	}
	ok(c, d)
}
