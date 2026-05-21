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

// FileAPI handles file management endpoints.
type FileAPI struct {
	fileService *system.FileService
}

// NewFileAPI creates a FileAPI instance.
func NewFileAPI() *FileAPI {
	return &FileAPI{
		fileService: system.NewFileService(),
	}
}

// Upload uploads a single file.
func (a *FileAPI) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "no file uploaded")
		return
	}

	userID, _ := c.Get("user_id")

	fileRecord, err := a.fileService.UploadContext(c.Request.Context(), file, userID.(uint))
	if err != nil {
		writeSystemFileServiceError(c, "failed to upload file", err)
		return
	}

	response.SuccessWithMessage(c, "file uploaded successfully", fileRecord)
}

// UploadMultiple uploads multiple files.
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

	results, errs := a.fileService.UploadMultipleContext(c.Request.Context(), files, userID.(uint))

	// Preserve per-file errors in a compact response.
	var errMsgs []string
	for _, err := range errs {
		errMsgs = append(errMsgs, systemFileServiceErrorMessage(err))
	}

	response.Success(c, gin.H{
		"uploaded": results,
		"errors":   errMsgs,
		"success":  len(results),
		"failed":   len(errs),
	})
}

// GetFileList returns paginated files.
func (a *FileAPI) GetFileList(c *gin.Context) {
	var req system.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
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
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}
	req.DataScope = dataScope

	files, total, err := a.fileService.GetFileListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get file list", err)
		return
	}

	response.PageSuccess(c, files, total, req.Page, req.PageSize)
}

// GetFile returns a file by id.
func (a *FileAPI) GetFile(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	file, err := a.fileService.GetFileByIDInScopeContext(c.Request.Context(), uint(id), dataScope)
	if err != nil {
		writeSystemFileServiceError(c, "failed to get file", err)
		return
	}

	response.Success(c, file)
}

// Download streams a file as an attachment.
func (a *FileAPI) Download(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	file, err := a.fileService.GetFileByIDInScopeContext(c.Request.Context(), uint(id), dataScope)
	if err != nil {
		writeSystemFileServiceError(c, "failed to download file", err)
		return
	}

	// Verify the file still exists on disk.
	if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
		response.NotFound(c, "file not found on disk")
		return
	}

	// Set download headers before streaming the file.
	c.Header("Content-Disposition", "attachment; filename="+file.FileName)
	c.Header("Content-Type", file.MimeType)
	c.File(file.FilePath)
}

// DeleteFile deletes a file.
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
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	if err := a.fileService.DeleteFileContext(c.Request.Context(), uint(id), userID.(uint), dataScope); err != nil {
		writeSystemFileServiceError(c, "failed to delete file", err)
		return
	}

	response.SuccessWithMessage(c, "file deleted successfully", nil)
}

// DeleteFiles deletes multiple files.
func (a *FileAPI) DeleteFiles(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	userID, _ := c.Get("user_id")
	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	if err := a.fileService.DeleteFilesContext(c.Request.Context(), req.IDs, userID.(uint), dataScope); err != nil {
		writeSystemFileServiceError(c, "failed to delete files", err)
		return
	}

	response.SuccessWithMessage(c, "files deleted successfully", nil)
}

// GetFileStats returns file statistics.
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
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	stats, err := a.fileService.GetFileStatsContext(c.Request.Context(), userID, dataScope)
	if err != nil {
		internalServerError(c, "failed to get file stats", err)
		return
	}

	response.Success(c, stats)
}

// ServeStaticFiles registers static file serving for local storage.
func ServeStaticFiles(router *gin.Engine) {
	if config.Cfg.Upload.EffectiveStorageType() != "local" {
		return
	}

	uploadPath := config.Cfg.Upload.EffectiveLocalPath()
	urlPrefix := config.Cfg.Upload.EffectiveLocalURLPrefix()

	// Ensure the upload directory exists before registering the route.
	if err := os.MkdirAll(uploadPath, 0755); err == nil {
		router.Static(urlPrefix, uploadPath)
	}
}

// Preview streams an image file inline.
func (a *FileAPI) Preview(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid file id")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	file, err := a.fileService.GetFileByIDInScopeContext(c.Request.Context(), uint(id), dataScope)
	if err != nil {
		writeSystemFileServiceError(c, "failed to preview file", err)
		return
	}

	// Only image files can be previewed inline.
	if file.FileType != "image" {
		response.BadRequest(c, "file is not an image")
		return
	}

	// Verify the file still exists on disk.
	if _, err := os.Stat(file.FilePath); os.IsNotExist(err) {
		response.NotFound(c, "file not found on disk")
		return
	}

	// Resolve the MIME type from the file extension.
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

// CheckHash checks whether a file hash already exists.
func (a *FileAPI) CheckHash(c *gin.Context) {
	hash := c.Query("hash")
	if hash == "" {
		response.BadRequest(c, "hash is required")
		return
	}

	dataScope, err := authz.ResolveUserDataScopeFromContext(c)
	if err != nil {
		internalServerError(c, "failed to resolve file data scope", err)
		return
	}

	file, err := a.fileService.GetFileByHashContext(c.Request.Context(), hash, dataScope)
	if err != nil {
		if errors.Is(err, system.ErrFileNotFoundOrPermissionDenied) {
			response.Success(c, gin.H{"exists": false})
			return
		}
		writeSystemFileServiceError(c, "failed to check file hash", err)
		return
	}

	response.Success(c, gin.H{
		"exists": true,
		"file":   file,
	})
}

// GetMyFiles returns files owned by the current user.
func (a *FileAPI) GetMyFiles(c *gin.Context) {
	var req system.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "invalid query parameters")
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

	files, total, err := a.fileService.GetFileListContext(c.Request.Context(), req)
	if err != nil {
		internalServerError(c, "failed to get current user files", err)
		return
	}

	response.PageSuccess(c, files, total, req.Page, req.PageSize)
}

// ImageResize is reserved for future image resizing.
func (a *FileAPI) ImageResize(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "not implemented"})
}
