package system

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/shared/pkg/response"
	systemsvc "github.com/go-admin-kit/services/system/internal/service/system"
)

type CodegenAPIOptions struct {
	RepositorySource systemsvc.RepositorySource
	RepositoryWriter *systemsvc.RepositoryWriter
	WriteEnabled     bool
}

type CodegenAPI struct {
	svc     systemsvc.CodegenService
	options CodegenAPIOptions
}

type CodegenCapabilities struct {
	PreviewEnabled  bool `json:"preview_enabled"`
	DownloadEnabled bool `json:"download_enabled"`
	WriteEnabled    bool `json:"write_enabled"`
}

type codegenOutputRequest struct {
	Request        systemsvc.GenerateRequest `json:"request" binding:"required"`
	ExpectedDigest string                    `json:"expected_digest" binding:"required"`
	Confirmation   string                    `json:"confirmation,omitempty"`
}

func NewCodegenAPIWithService(svc systemsvc.CodegenService) *CodegenAPI {
	return NewCodegenAPIWithOptions(svc, CodegenAPIOptions{})
}

func NewCodegenAPIWithOptions(svc systemsvc.CodegenService, options CodegenAPIOptions) *CodegenAPI {
	return &CodegenAPI{svc: svc, options: options}
}

func (a *CodegenAPI) Capabilities(c *gin.Context) {
	available := a.options.RepositorySource != nil
	response.Success(c, CodegenCapabilities{
		PreviewEnabled:  available,
		DownloadEnabled: available,
		WriteEnabled:    available && a.options.WriteEnabled && a.options.RepositoryWriter != nil,
	})
}

// GetTables handles GET /api/v1/codegen/tables.
func (a *CodegenAPI) GetTables(c *gin.Context) {
	tables, err := a.svc.ListTables()
	if err != nil {
		response.InternalServerError(c, "查询代码生成数据表失败")
		return
	}
	response.Success(c, gin.H{"list": tables, "total": len(tables)})
}

// GetColumns preserves the original columns endpoint during frontend migration.
func (a *CodegenAPI) GetColumns(c *gin.Context) {
	columns, err := a.svc.TableColumns(c.Param("name"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"list": columns, "total": len(columns)})
}

func (a *CodegenAPI) GetSchema(c *gin.Context) {
	schema, err := systemsvc.NewSchemaInspector(a.svc.DB).InspectTable(c.Param("name"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, schema)
}

func (a *CodegenAPI) Preview(c *gin.Context) {
	var request systemsvc.GenerateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	plan, err := a.buildPlan(request)
	if err != nil {
		writeCodegenPlanError(c, err)
		return
	}
	response.Success(c, plan)
}

func (a *CodegenAPI) Download(c *gin.Context) {
	var output codegenOutputRequest
	if err := c.ShouldBindJSON(&output); err != nil || output.ExpectedDigest == "" {
		response.BadRequest(c, "请求参数无效")
		return
	}
	plan, err := a.buildPlan(output.Request)
	if err != nil {
		writeCodegenPlanError(c, err)
		return
	}
	if plan.Digest != output.ExpectedDigest {
		response.Error(c, http.StatusConflict, "生成计划已变化，请重新预检")
		return
	}
	payload, err := systemsvc.ExportZIP(plan)
	if err != nil {
		writeCodegenOutputError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=codegen-%s.zip", plan.Request.Module))
	c.Data(http.StatusOK, "application/zip", payload)
}

func (a *CodegenAPI) Write(c *gin.Context) {
	if !a.options.WriteEnabled || a.options.RepositoryWriter == nil {
		response.Forbidden(c, "仓库写入未启用")
		return
	}
	var output codegenOutputRequest
	if err := c.ShouldBindJSON(&output); err != nil || output.ExpectedDigest == "" {
		response.BadRequest(c, "请求参数无效")
		return
	}
	plan, err := a.buildPlan(output.Request)
	if err != nil {
		writeCodegenPlanError(c, err)
		return
	}
	if plan.Digest != output.ExpectedDigest {
		response.Error(c, http.StatusConflict, "生成计划已变化，请重新预检")
		return
	}
	if output.Confirmation != plan.Request.Module {
		response.BadRequest(c, "确认文本必须与模块名一致")
		return
	}
	result, err := a.options.RepositoryWriter.Write(plan)
	if err != nil {
		writeCodegenOutputError(c, err)
		return
	}
	response.SuccessWithMessage(c, "已写入仓库", result)
}

func (a *CodegenAPI) buildPlan(request systemsvc.GenerateRequest) (systemsvc.GenerationPlan, error) {
	if a.options.RepositorySource == nil {
		return systemsvc.GenerationPlan{}, errCodegenRepositoryUnavailable
	}
	if _, err := a.svc.ValidateRequest(request); err != nil {
		return systemsvc.GenerationPlan{}, codegenRequestError{err: err}
	}
	plan, err := a.svc.BuildPlan(request, a.options.RepositorySource)
	if err != nil {
		return systemsvc.GenerationPlan{}, fmt.Errorf("build generation plan: %w", err)
	}
	return plan, nil
}

var errCodegenRepositoryUnavailable = errors.New("codegen repository snapshot unavailable")

type codegenRequestError struct{ err error }

func (e codegenRequestError) Error() string { return e.err.Error() }
func (e codegenRequestError) Unwrap() error { return e.err }

func writeCodegenPlanError(c *gin.Context, err error) {
	var requestError codegenRequestError
	switch {
	case errors.As(err, &requestError):
		response.BadRequest(c, requestError.Error())
	case errors.Is(err, errCodegenRepositoryUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "仓库快照不可用")
	default:
		response.Error(c, http.StatusUnprocessableEntity, "无法根据当前仓库状态建立生成计划")
	}
}

func writeCodegenOutputError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, systemsvc.ErrRepositoryConflict), errors.Is(err, systemsvc.ErrRepositoryLocked):
		response.Error(c, http.StatusConflict, "仓库状态已变化，请重新预检")
	case errors.Is(err, systemsvc.ErrInvalidGenerationPlan), errors.Is(err, systemsvc.ErrPathOutsideRoot):
		response.Error(c, http.StatusUnprocessableEntity, "生成计划包含冲突或无效产物")
	default:
		response.InternalServerError(c, "代码生成输出失败")
	}
}
