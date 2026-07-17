package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/go-admin-kit/services/im/internal/storage"
	"github.com/google/uuid"
)

// Attachment upload (M2.1 → M5 storage): visitors and agents share one
// endpoint; objects go to Server.Storage (MinIO in the stack, local disk in
// dev) under key "<yyyymm>/<uuid><ext>" and are served back via
// /im/uploads/<key>. Filenames are random UUIDs so URLs are not guessable.

const (
	attachmentMaxBytes = 10 << 20 // 10 MB
)

// imageExts render inline in clients; everything else in fileExts renders as
// a download link. Extensions outside both sets are rejected.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

var fileExts = map[string]bool{
	".pdf": true, ".txt": true, ".zip": true, ".doc": true, ".docx": true,
	".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".csv": true,
	".mp4": true, ".mp3": true,
}

// UploadAttachment handles POST /api/v1/im/attachments (multipart "file").
// Auth: guest token or agent token, same resolution order as SendMessage.
func (s *Server) UploadAttachment(c *gin.Context) {
	senderOK := false
	if tok := bearer(c); tok != "" {
		if _, err := authjwt.ParseGuest(s.Secret, tok); err == nil {
			senderOK = true
		}
	}
	if !senderOK {
		if _, ok := s.requireAgent(c); !ok {
			return
		}
	}

	fh, err := c.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, "missing file field")
		return
	}
	if fh.Size <= 0 || fh.Size > attachmentMaxBytes {
		Fail(c, http.StatusBadRequest, fmt.Sprintf("file size must be 1B-%dMB", attachmentMaxBytes>>20))
		return
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	msgType := ""
	switch {
	case imageExts[ext]:
		msgType = "image"
	case fileExts[ext]:
		msgType = "file"
	default:
		Fail(c, http.StatusBadRequest, "unsupported file type: "+ext)
		return
	}

	key := time.Now().Format("200601") + "/" + uuid.NewString() + ext
	src, err := fh.Open()
	if err != nil {
		Fail(c, http.StatusBadRequest, "unreadable file")
		return
	}
	defer src.Close()
	if err := s.Storage.Save(c.Request.Context(), key, src, fh.Size, fh.Header.Get("Content-Type")); err != nil {
		Fail(c, http.StatusInternalServerError, "storage unavailable")
		return
	}

	OK(c, gin.H{
		"url":      "/im/uploads/" + key,
		"name":     fh.Filename,
		"size":     fh.Size,
		"mime":     fh.Header.Get("Content-Type"),
		"msg_type": msgType,
	})
}

// ServeAttachment handles GET /im/uploads/*key: streams from Storage, then
// falls back to the legacy local dir (pre-MinIO files on the im_uploads
// volume).
func (s *Server) ServeAttachment(c *gin.Context) {
	key, err := storage.CleanKey(c.Param("key"))
	if err != nil {
		Fail(c, http.StatusBadRequest, "bad path")
		return
	}
	body, size, contentType, err := s.Storage.Open(c.Request.Context(), key)
	if err != nil && s.UploadDir != "" && s.Storage.Type() != "local" {
		// legacy fallback: files uploaded before object storage
		p := filepath.Join(s.UploadDir, filepath.FromSlash(key))
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			c.File(p)
			return
		}
	}
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			Fail(c, http.StatusNotFound, "not found")
			return
		}
		Fail(c, http.StatusInternalServerError, "storage unavailable")
		return
	}
	defer body.Close()
	c.DataFromReader(http.StatusOK, size, contentType, body, map[string]string{
		"Cache-Control": "public, max-age=31536000, immutable", // UUID 文件名内容不变
	})
}
