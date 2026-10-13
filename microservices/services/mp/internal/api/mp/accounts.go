package api

// 公众号账号 CRUD。密钥合规：appsecret/aeskey 仅以引用名（SecretRef/AESKeyRef）
// 落库，Token 是 json:"-" 不回显——返回体不含任何密钥明文。

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/mp/internal/store/mp"
)

// accountReq 建/改账号入参。app_id 仅在建时生效，改时忽略（不可改）。
type accountReq struct {
	AppID       string `json:"app_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	GhID        string `json:"gh_id"`
	SecretRef   string `json:"secret_ref"`
	Token       string `json:"token"`
	AESKeyRef   string `json:"aes_key_ref"`
	EncryptMode string `json:"encrypt_mode"`
	AutoPublish bool   `json:"auto_publish"`
	Status      string `json:"status"`
}

func (s *Server) ListAccounts(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	page, pageSize := pageOf(c)
	list, total, err := s.Store.ListAccounts(u.TenantID, c.Query("keyword"), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (s *Server) CreateAccount(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	var req accountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	a := store.MpAccount{
		TenantID: u.TenantID, AppID: req.AppID, Name: req.Name,
		Type: req.Type, GhID: req.GhID,
		SecretRef: req.SecretRef, Token: req.Token, AESKeyRef: req.AESKeyRef,
		EncryptMode: req.EncryptMode, AutoPublish: req.AutoPublish,
		Status: req.Status,
	}
	if err := s.Store.CreateAccount(c.Request.Context(), &a); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, a)
}

func (s *Server) GetAccount(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	a, err := s.Store.GetAccount(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "公众号账号不存在")
		return
	}
	ok(c, a)
}

func (s *Server) UpdateAccount(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	a, err := s.Store.GetAccount(id, u.TenantID)
	if err != nil {
		notFoundOr(c, err, "公众号账号不存在")
		return
	}
	var req accountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	// app_id 不可改；其余字段按传入覆盖。
	a.Name = req.Name
	if req.Type != "" {
		a.Type = req.Type
	}
	a.GhID = req.GhID
	a.SecretRef = req.SecretRef
	if req.Token != "" {
		a.Token = req.Token
	}
	a.AESKeyRef = req.AESKeyRef
	if req.EncryptMode != "" {
		a.EncryptMode = req.EncryptMode
	}
	a.AutoPublish = req.AutoPublish
	if req.Status != "" {
		a.Status = req.Status
	}
	if err := s.Store.UpdateAccount(c.Request.Context(), a); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, a)
}

func (s *Server) DeleteAccount(c *gin.Context) {
	u, authed := s.requireUser(c)
	if !authed {
		return
	}
	id, valid := pathID(c, "id")
	if !valid {
		return
	}
	if err := s.Store.DeleteAccount(c.Request.Context(), id, u.TenantID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "公众号账号不存在")
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"deleted": id})
}
