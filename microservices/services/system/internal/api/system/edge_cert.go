package system

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/edgecert"
	"github.com/go-admin-kit/services/system/internal/pkg/database"
)

// EdgeCertAPI 边缘免费证书管理。
type EdgeCertAPI struct {
	svc edgecert.Service
}

func NewEdgeCertAPI() *EdgeCertAPI {
	return &EdgeCertAPI{svc: edgecert.Service{DB: database.DB}}
}

func (a *EdgeCertAPI) List(c *gin.Context) {
	list, err := a.svc.List(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list edge certs failed: "+err.Error())
		return
	}
	response.Success(c, list)
}

func (a *EdgeCertAPI) Create(c *gin.Context) {
	var req edgecert.UpsertInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	v, err := a.svc.UpsertDraft(c.Request.Context(), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, v)
}

func (a *EdgeCertAPI) Issue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	v, err := a.svc.Issue(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, "issue failed: "+err.Error())
		return
	}
	response.Success(c, v)
}

func (a *EdgeCertAPI) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := a.svc.Delete(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithMessage(c, "deleted", nil)
}

func (a *EdgeCertAPI) Download(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid id")
		return
	}
	fullchain, key, domain, err := a.svc.Download(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"domain":          domain,
		"fullchain_pem":   fullchain,
		"private_key_pem": key,
	})
}

// ACMEChallenge 公开：Let's Encrypt HTTP-01（无鉴权）。
func ACMEChallenge(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.String(http.StatusNotFound, "not found")
		return
	}
	keyAuth, ok := edgecert.LookupChallenge(token)
	if !ok {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(keyAuth))
}
