package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/im/internal/authjwt"
	"github.com/google/uuid"
)

// Attachment upload (M2.1): visitors and agents share one endpoint; files
// land on local disk under UploadDir and are served by the /im/uploads
// static route. Filenames are random UUIDs so URLs are not guessable.

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

	subdir := time.Now().Format("200601")
	dir := filepath.Join(s.UploadDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Fail(c, http.StatusInternalServerError, "storage unavailable")
		return
	}
	name := uuid.NewString() + ext
	dst := filepath.Join(dir, name)
	if err := c.SaveUploadedFile(fh, dst); err != nil {
		Fail(c, http.StatusInternalServerError, "save failed")
		return
	}

	OK(c, gin.H{
		"url":      "/im/uploads/" + subdir + "/" + name,
		"name":     fh.Filename,
		"size":     fh.Size,
		"mime":     fh.Header.Get("Content-Type"),
		"msg_type": msgType,
	})
}
