package system

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/file/internal/pkg/upload"
	filesvc "github.com/go-admin-kit/services/file/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

// UploadAvatar accepts a current user's custom avatar without granting the
// broader file-management upload permission.
func (a *FileAPI) UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "avatar image is required")
		return
	}
	userID, ok := c.Get("user_id")
	if !ok {
		response.Unauthorized(c, "user not found in context")
		return
	}

	record, err := a.fileService.UploadAvatarContext(c.Request.Context(), file, userID.(uint))
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrFileEmpty),
			errors.Is(err, upload.ErrAvatarTooLarge),
			errors.Is(err, upload.ErrAvatarTypeNotAllowed),
			errors.Is(err, upload.ErrAvatarDimensions):
			response.BadRequest(c, err.Error())
		default:
			writeSystemFileServiceError(c, "failed to upload avatar", err)
		}
		return
	}

	if record.PublicToken == nil {
		internalServerError(c, "failed to issue avatar URL", errors.New("avatar public token is missing"))
		return
	}
	response.SuccessWithMessage(c, "avatar uploaded successfully", gin.H{
		"url": avatarPublicURL(*record.PublicToken),
	})
}

// CleanupAvatars removes avatar files owned by the current user except the
// explicitly retained tokens. It runs after the profile update succeeds.
func (a *FileAPI) CleanupAvatars(c *gin.Context) {
	userID, ok := c.Get("user_id")
	if !ok {
		response.Unauthorized(c, "user not found in context")
		return
	}
	var req struct {
		KeepTokens []string `json:"keep_tokens"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	for _, token := range req.KeepTokens {
		if !filesvc.ValidAvatarPublicToken(token) {
			response.BadRequest(c, "invalid avatar token")
			return
		}
	}
	if err := a.fileService.DeleteOtherAvatarsContext(c.Request.Context(), userID.(uint), req.KeepTokens); err != nil {
		writeSystemFileServiceError(c, "failed to clean up old avatars", err)
		return
	}
	response.SuccessWithMessage(c, "old avatars cleaned up", nil)
}

// ServeAvatar streams immutable avatar bytes by an unguessable public token.
// The URL is stable and safe for img tags, unlike expiring generic upload URLs.
func (a *FileAPI) ServeAvatar(c *gin.Context) {
	content, err := a.fileService.OpenAvatarByTokenContext(c.Request.Context(), c.Param("token"))
	if err != nil {
		if errors.Is(err, filesvc.ErrFileNotFoundOrPermissionDenied) {
			c.Status(http.StatusNotFound)
			return
		}
		internalServerError(c, "failed to open avatar", err)
		return
	}
	defer content.Body.Close()

	c.Header("Content-Type", content.ContentType)
	c.Header("Content-Length", sizeHeader(content.Size))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, content.Body)
}

func avatarPublicURL(token string) string {
	return path.Join("/api/v1/files/avatars", url.PathEscape(token))
}

func sizeHeader(size int64) string {
	if size <= 0 {
		return ""
	}
	return strconv.FormatInt(size, 10)
}
