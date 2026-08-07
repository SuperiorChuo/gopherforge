package system

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	service "github.com/go-admin-kit/services/file/internal/service/system"
	"github.com/go-admin-kit/services/shared/pkg/response"
)

type initChunkedUploadRequest struct {
	FileName string `json:"file_name" binding:"required"`
	FileSize int64  `json:"file_size" binding:"required"`
	Hash     string `json:"hash"`
}

// InitChunkedUpload creates a session for a chunked upload; returns
// {already_exists:true} when the hash is already on file (dedupe).
func (a *FileAPI) InitChunkedUpload(c *gin.Context) {
	var req initChunkedUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "file_name and file_size are required")
		return
	}
	if req.FileSize <= 0 {
		response.BadRequest(c, "file_size must be positive")
		return
	}
	userID := currentUserID(c)
	session, alreadyExists, err := a.fileService.InitChunkedUploadContext(c.Request.Context(), service.ChunkedInitRequest{
		FileName: req.FileName,
		FileSize: req.FileSize,
		Hash:     strings.TrimSpace(req.Hash),
	}, userID)
	if err != nil {
		writeSystemFileServiceError(c, "failed to init chunked upload", err)
		return
	}
	if alreadyExists {
		response.Success(c, gin.H{"already_exists": true})
		return
	}
	response.Success(c, gin.H{"already_exists": false, "session": session})
}

// UploadChunk stores one part (multipart field "chunk", part number in URL).
func (a *FileAPI) UploadChunk(c *gin.Context) {
	sessionID, ok := parseSessionID(c)
	if !ok {
		return
	}
	partN, err := strconv.Atoi(c.Param("part"))
	if err != nil || partN < 1 {
		response.BadRequest(c, "invalid part number")
		return
	}
	file, err := c.FormFile("chunk")
	if err != nil {
		response.BadRequest(c, "chunk part is required (multipart field 'chunk')")
		return
	}
	src, err := file.Open()
	if err != nil {
		response.BadRequest(c, "failed to open chunk")
		return
	}
	defer src.Close()

	session, err := a.fileService.UploadChunkContext(c.Request.Context(), sessionID, partN, src)
	if err != nil {
		writeSystemFileServiceError(c, "failed to store chunk", err)
		return
	}
	response.Success(c, gin.H{"session": session})
}

// CompleteChunkedUpload assembles all parts into a file record.
func (a *FileAPI) CompleteChunkedUpload(c *gin.Context) {
	sessionID, ok := parseSessionID(c)
	if !ok {
		return
	}
	record, err := a.fileService.CompleteChunkedUploadContext(c.Request.Context(), sessionID)
	if err != nil {
		writeSystemFileServiceError(c, "failed to complete chunked upload", err)
		return
	}
	signFileURLs(record)
	response.SuccessWithMessage(c, "file uploaded successfully", record)
}

// AbortChunkedUpload discards the session and stored parts.
func (a *FileAPI) AbortChunkedUpload(c *gin.Context) {
	sessionID, ok := parseSessionID(c)
	if !ok {
		return
	}
	if err := a.fileService.AbortChunkedUploadContext(c.Request.Context(), sessionID); err != nil {
		writeSystemFileServiceError(c, "failed to abort chunked upload", err)
		return
	}
	response.Success(c, nil)
}

func parseSessionID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("session_id"), 10, 32)
	if err != nil || id == 0 {
		response.BadRequest(c, "invalid session id")
		return 0, false
	}
	return uint(id), true
}

func currentUserID(c *gin.Context) uint {
	v, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

