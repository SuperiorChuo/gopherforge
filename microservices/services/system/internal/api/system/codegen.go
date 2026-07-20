package system

import (
	"archive/zip"
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

type CodegenAPI struct {
	svc systemsvc.CodegenService
}

func NewCodegenAPIWithService(svc systemsvc.CodegenService) *CodegenAPI {
	return &CodegenAPI{svc: svc}
}

// GetTables handles GET /api/v1/codegen/tables.
func (a *CodegenAPI) GetTables(c *gin.Context) {
	tables, err := a.svc.ListTables()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"list": tables, "total": len(tables)}})
}

// GetColumns handles GET /api/v1/codegen/tables/:name/columns.
func (a *CodegenAPI) GetColumns(c *gin.Context) {
	table := c.Param("name")
	cols, err := a.svc.TableColumns(table)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"list": cols, "total": len(cols)}})
}

// Preview handles POST /api/v1/codegen/preview.
func (a *CodegenAPI) Preview(c *gin.Context) {
	var req systemsvc.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	files, err := a.svc.Generate(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"files": files}})
}

// Download handles POST /api/v1/codegen/download (returns a zip blob).
func (a *CodegenAPI) Download(c *gin.Context) {
	var req systemsvc.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	files, err := a.svc.Generate(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.Path)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
		if _, err := w.Write([]byte(f.Content)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
	}
	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=codegen-"+req.Module+".zip")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
