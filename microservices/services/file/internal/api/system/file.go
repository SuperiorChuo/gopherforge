package system

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/file/internal/config"
	localmodel "github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/file/internal/pkg/authz"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	"github.com/go-admin-kit/services/file/internal/pkg/urlsign"
	"github.com/go-admin-kit/services/file/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
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

// NewFileAPIWithService creates a FileAPI instance from an injected service.
func NewFileAPIWithService(fileService *system.FileService) *FileAPI {
	return &FileAPI{fileService: fileService}
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

	signFileURLs(fileRecord)
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
	for _, result := range results {
		signFileURLs(result)
	}

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

	signFileListURLs(files)
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

	signFileURLs(file)
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

	content, err := a.fileService.OpenFileContentContext(c.Request.Context(), file)
	if err != nil {
		writeSystemFileServiceError(c, "failed to open file content", err)
		return
	}
	defer content.Body.Close()

	// Set download headers before streaming the file.
	c.Header("Content-Disposition", fileDownloadDisposition(content.FileName))
	c.Header("Content-Type", content.ContentType)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, content.Body)
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

// ServeStaticFiles registers /uploads serving for the configured storage.
// Objects stream under the same /uploads URL shape for every backend, so
// PUBLIC_BASE_URL stays unchanged and the bucket never needs public access.
// Objects missing from the configured backend fall back to the local disk so
// legacy files uploaded before a storage switch keep working.
//
// Access control: /uploads cannot rely on the Authorization header (browsers
// load these URLs from img tags), so every request must carry a valid signed
// URL (expiry + HMAC query params, issued by the API together with the file
// metadata). Unsigned, tampered, or expired requests get 404. Signing covers
// only the object key, so legacy keys stored before this change remain
// reachable through freshly signed URLs without any data migration.
func ServeStaticFiles(router *gin.Engine) {
	storageType := config.Cfg.Upload.EffectiveStorageType()
	urlPrefix := config.Cfg.Upload.EffectiveLocalURLPrefix()
	uploadPath := config.Cfg.Upload.EffectiveLocalPath()

	if storageType == "local" {
		// Ensure the upload directory exists before registering the route.
		_ = os.MkdirAll(uploadPath, 0755)
	}

	uploader := upload.NewUploader()
	router.GET(strings.TrimRight(urlPrefix, "/")+"/*filepath", func(c *gin.Context) {
		key := strings.TrimPrefix(c.Param("filepath"), "/")

		signer := newUploadURLSigner()
		if err := signer.Verify(key, c.Query(urlsign.QueryExpires), c.Query(urlsign.QuerySignature), time.Now()); err != nil {
			// 404 (not 401/403) so unauthenticated probes cannot tell
			// missing objects apart from denied ones.
			c.Status(http.StatusNotFound)
			return
		}

		obj, err := uploader.OpenForStorageTypeContext(c.Request.Context(), storageType, key)
		if err != nil && storageType != "local" {
			obj, err = uploader.OpenForStorageTypeContext(c.Request.Context(), "local", key)
		}
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer obj.Body.Close()

		if ct := mime.TypeByExtension(strings.ToLower(pathpkg.Ext(key))); ct != "" {
			c.Header("Content-Type", ct)
		}
		if obj.Size > 0 {
			c.Header("Content-Length", strconv.FormatInt(obj.Size, 10))
		}
		// Signed URLs expire; shared caches must not serve them to others.
		c.Header("Cache-Control", "private, max-age=300")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, obj.Body)
	})
}

// newUploadURLSigner builds the /uploads signer from configuration. A
// dedicated UPLOAD_URL_SIGN_SECRET wins; otherwise the JWT secret keeps the
// service self-contained without introducing a new mandatory setting.
func newUploadURLSigner() *urlsign.Signer {
	secret := strings.TrimSpace(config.Cfg.Upload.URLSignSecret)
	if secret == "" {
		secret = strings.TrimSpace(config.Cfg.JWT.Secret)
	}
	return urlsign.New(secret, time.Duration(config.Cfg.Upload.URLSignTTLSeconds)*time.Second)
}

// signFileURLs replaces the stored /uploads URLs on a file record with signed
// ones before the record leaves the API. Stored values stay unsigned.
func signFileURLs(file *localmodel.File) {
	if file == nil {
		return
	}
	signer := newUploadURLSigner()
	if !signer.Enabled() {
		return
	}
	prefix := config.Cfg.Upload.EffectiveLocalURLPrefix()
	now := time.Now()
	file.URL = signer.SignURL(file.URL, prefix, now)
	file.ThumbnailURL = signer.SignURL(file.ThumbnailURL, prefix, now)
}

func signFileListURLs(files []localmodel.File) {
	for i := range files {
		signFileURLs(&files[i])
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

	content, err := a.fileService.OpenFileContentContext(c.Request.Context(), file)
	if err != nil {
		writeSystemFileServiceError(c, "failed to open preview content", err)
		return
	}
	defer content.Body.Close()

	c.Header("Content-Type", content.ContentType)
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, content.Body)
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

	signFileURLs(file)
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

	signFileListURLs(files)
	response.PageSuccess(c, files, total, req.Page, req.PageSize)
}

func fileDownloadDisposition(filename string) string {
	filename = strings.NewReplacer("\r", "_", "\n", "_").Replace(strings.TrimSpace(filename))
	if filename == "" {
		filename = "download"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}
