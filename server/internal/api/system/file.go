package system

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/server/internal/config"
	"github.com/go-admin-kit/server/internal/pkg/authz"
	"github.com/go-admin-kit/server/internal/pkg/response"
	"github.com/go-admin-kit/server/internal/service/system"
)

// FileAPI 文件管理 API
type FileAPI struct {
	fileService *system.FileService
}

// NewFileAPI 创建 FileAPI 实例
func NewFileAPI() *FileAPI {
	return &FileAPI{
		fileService: system.NewFileService(),
	}
}

// Upload 上传单个文件
func (a *FileAPI) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "no file uploaded")
		return
	}

	userID, _ := c.Get("user_id")

	fileRecord, err := a.fileService.Upload(file, userID.(uint))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "file uploaded successfully", fileRecord)
}

// UploadMultiple 批量上传文件
func (a *FileAPI) UploadMultiple(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		response.BadRequest(c, "failed to parse form")
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		response.BadRequest(c, "no files uploaded")
		return
	}

	userID, _ := c.Get("user_id")

	results, errs := a.fileService.UploadMultiple(files, userID.(uint))

	// 构建响应
	var errMsgs []string
	for _, err := range errs {
		errMsgs = append(errMsgs, err.Error())
	}

	response.Success(c, gin.H{
		"uploaded": results,
		"errors":   errMsgs,
		"success":  len(results),
		"failed":   len(errs),
	})
}

// GetFileList 获取文件列表
func (a *FileAPI) GetFileList(c *gin.Context) {
	var req system.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}
	req.DataScope = dataScope

	files, total, err := a.fileService.GetFileList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, files, total, req.Page, req.PageSize)
}

// GetFile 获取文件详情
func (a *FileAPI) GetFile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	file, err := a.fileService.GetFileByIDInScope(uint(id), dataScope)
	if err != nil {
		response.NotFound(c, "file not found")
		return
	}

	response.Success(c, file)
}

// Download 下载文件
func (a *FileAPI) Download(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	file, err := a.fileService.GetFileByIDInScope(uint(id), dataScope)
	if err != nil {
		response.NotFound(c, "file not found")
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
		response.NotFound(c, "file not found on disk")
		return
	}

	// 设置下载头
	c.Header("Content-Disposition", "attachment; filename="+file.FileName)
	c.Header("Content-Type", file.MimeType)
	c.File(file.FilePath)
}

// DeleteFile 删除文件
func (a *FileAPI) DeleteFile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	userID, _ := c.Get("user_id")
	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	if err := a.fileService.DeleteFile(uint(id), userID.(uint), dataScope); err != nil {
		a.handleFileError(c, err)
		return
	}

	response.SuccessWithMessage(c, "file deleted successfully", nil)
}

// DeleteFiles 批量删除文件
func (a *FileAPI) DeleteFiles(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	if err := a.fileService.DeleteFiles(req.IDs, userID.(uint), dataScope); err != nil {
		a.handleFileError(c, err)
		return
	}

	response.SuccessWithMessage(c, "files deleted successfully", nil)
}

// GetFileStats 获取文件统计
func (a *FileAPI) GetFileStats(c *gin.Context) {
	var userID *uint
	if uidStr := c.Query("user_id"); uidStr != "" {
		uid, err := strconv.ParseUint(uidStr, 10, 32)
		if err == nil {
			u := uint(uid)
			userID = &u
		}
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	stats, err := a.fileService.GetFileStats(userID, dataScope)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.Success(c, stats)
}

// ServeStatic 静态文件服务（注册到路由）
func ServeStaticFiles(router *gin.Engine) {
	if config.Cfg.Upload.EffectiveStorageType() != "local" {
		return
	}

	uploadPath := config.Cfg.Upload.EffectiveLocalPath()
	urlPrefix := config.Cfg.Upload.EffectiveLocalURLPrefix()

	// 确保上传目录存在
	if err := os.MkdirAll(uploadPath, 0755); err == nil {
		router.Static(urlPrefix, uploadPath)
	}
}

// Preview 预览文件（图片）
func (a *FileAPI) Preview(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	file, err := a.fileService.GetFileByIDInScope(uint(id), dataScope)
	if err != nil {
		response.NotFound(c, "file not found")
		return
	}

	// 检查是否是图片
	if file.FileType != "image" {
		response.BadRequest(c, "file is not an image")
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
		response.NotFound(c, "file not found on disk")
		return
	}

	// 获取文件扩展名对应的 MIME 类型
	ext := filepath.Ext(file.FilePath)
	contentType := "image/jpeg"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".svg":
		contentType = "image/svg+xml"
	}

	c.Header("Content-Type", contentType)
	c.File(file.FilePath)
}

// CheckHash 检查文件哈希（用于秒传）
func (a *FileAPI) CheckHash(c *gin.Context) {
	hash := c.Query("hash")
	if hash == "" {
		response.BadRequest(c, "hash is required")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	file, err := a.fileService.GetFileByHash(hash, dataScope)
	if err != nil {
		response.Success(c, gin.H{"exists": false})
		return
	}

	response.Success(c, gin.H{
		"exists": true,
		"file":   file,
	})
}

// GetMyFiles 获取当前用户的文件
func (a *FileAPI) GetMyFiles(c *gin.Context) {
	var req system.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	userID, _ := c.Get("user_id")
	uid := userID.(uint)
	req.UserID = &uid
	req.DataScope = authz.UserDataScope{
		Scope:  authz.DataScopeSelf,
		UserID: uid,
	}

	files, total, err := a.fileService.GetFileList(req)
	if err != nil {
		response.InternalServerError(c, err.Error())
		return
	}

	response.PageSuccess(c, files, total, req.Page, req.PageSize)
}

// ImageResize 图片缩放（预留接口）
func (a *FileAPI) ImageResize(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
}

func (a *FileAPI) handleFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, system.ErrFileNotFoundOrPermissionDenied):
		response.NotFound(c, err.Error())
	default:
		response.BadRequest(c, err.Error())
	}
}
