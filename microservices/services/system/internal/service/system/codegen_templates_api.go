package system

var tplLayeredAPI = mustTpl("layered-api", `package system

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	"github.com/go-admin-kit/services/system/internal/middleware"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
	"gorm.io/gorm"
)

type {{.ModuleType}}API struct { service *systemsvc.{{.ModuleType}}Service }

func New{{.ModuleType}}API(db *gorm.DB) *{{.ModuleType}}API {
	if db == nil { return nil }
	return &{{.ModuleType}}API{service: systemsvc.New{{.ModuleType}}Service(db)}
}

func (a *{{.ModuleType}}API) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/{{.Module}}", middleware.PermissionMiddleware("system:{{.Module}}:list"), a.List)
{{- if .IsTree}}
	router.GET("/{{.Module}}/tree", middleware.PermissionMiddleware("system:{{.Module}}:list"), a.Tree)
{{- end}}
	router.GET("/{{.Module}}/relations/:name/options", middleware.PermissionMiddleware("system:{{.Module}}:list"), a.RelationOptions)
	router.GET("/{{.Module}}/:id", middleware.PermissionMiddleware("system:{{.Module}}:list"), a.Get)
	router.POST("/{{.Module}}", middleware.PermissionMiddleware("system:{{.Module}}:create"), a.Create)
	router.PUT("/{{.Module}}/:id", middleware.PermissionMiddleware("system:{{.Module}}:update"), a.Update)
	router.DELETE("/{{.Module}}/:id", middleware.PermissionMiddleware("system:{{.Module}}:delete"), a.Delete)
}
{{- if .IsTree}}

func (a *{{.ModuleType}}API) Tree(c *gin.Context) {
	rows, err := a.service.Tree(c.Request.Context())
	if err != nil { response.InternalServerError(c, "查询{{.Title}}树失败"); return }
	response.Success(c, rows)
}
{{- end}}

func (a *{{.ModuleType}}API) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := a.service.List(c.Request.Context(), c.Query("keyword"), page, pageSize)
	if err != nil { response.InternalServerError(c, "查询{{.Title}}失败"); return }
	response.Success(c, gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize})
}

func (a *{{.ModuleType}}API) Get(c *gin.Context) {
	id, ok := parse{{.ModuleType}}ID(c); if !ok { return }
	row, err := a.service.Get(c.Request.Context(), id)
	if err != nil { write{{.ModuleType}}Error(c, err); return }
	response.Success(c, row)
}

func (a *{{.ModuleType}}API) Create(c *gin.Context) {
	var input systemsvc.{{.ModuleType}}Input
	if err := c.ShouldBindJSON(&input); err != nil { response.BadRequest(c, "请求参数无效"); return }
	row, err := a.service.Create(c.Request.Context(), input)
	if err != nil { write{{.ModuleType}}Error(c, err); return }
	response.SuccessWithMessage(c, "创建成功", row)
}

func (a *{{.ModuleType}}API) Update(c *gin.Context) {
	id, ok := parse{{.ModuleType}}ID(c); if !ok { return }
	var input systemsvc.{{.ModuleType}}Input
	if err := c.ShouldBindJSON(&input); err != nil { response.BadRequest(c, "请求参数无效"); return }
	row, err := a.service.Update(c.Request.Context(), id, input)
	if err != nil { write{{.ModuleType}}Error(c, err); return }
	response.SuccessWithMessage(c, "更新成功", row)
}

func (a *{{.ModuleType}}API) Delete(c *gin.Context) {
	id, ok := parse{{.ModuleType}}ID(c); if !ok { return }
	if err := a.service.Delete(c.Request.Context(), id); err != nil { write{{.ModuleType}}Error(c, err); return }
	response.SuccessWithMessage(c, "删除成功", nil)
}

func (a *{{.ModuleType}}API) RelationOptions(c *gin.Context) {
	options, err := a.service.RelationOptions(c.Request.Context(), c.Param("name"))
	if err != nil { response.BadRequest(c, "未知关联"); return }
	response.Success(c, options)
}

func parse{{.ModuleType}}ID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil { response.BadRequest(c, "ID 无效"); return 0, false }
	return id, true
}

func write{{.ModuleType}}Error(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) { response.NotFound(c, "记录不存在"); return }
	response.InternalServerError(c, "操作{{.Title}}失败")
}
`)
