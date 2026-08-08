package system

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-admin-kit/services/file/internal/config"
	"github.com/go-admin-kit/services/file/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// chunkUploadChunkSize is the fixed part size the frontend slices with.
const chunkUploadChunkSize = 5 * 1024 * 1024 // 5 MB

// chunkUploadTTL bounds how long an unfinished session lives before the lazy
// prune on the next init removes it.
const chunkUploadTTL = 24 * time.Hour

var (
	ErrChunkSessionNotFound = errors.New("chunk upload session not found")
	ErrChunkSessionDone     = errors.New("chunk upload session already completed")
	ErrChunkPartInvalid     = errors.New("invalid chunk part number")
	ErrChunkIncomplete      = errors.New("chunk upload incomplete")
)

// ChunkedInitRequest describes a file the client wants to upload in parts.
type ChunkedInitRequest struct {
	FileName string
	FileSize int64
	Hash     string
}

// InitChunkedUploadContext creates a session for a chunked upload. Returns
// alreadyExists=true when the hash already belongs to this tenant (instant
// dedupe — the client can skip uploading).
func (s *FileService) InitChunkedUploadContext(ctx context.Context, req ChunkedInitRequest, userID uint) (*model.UploadSession, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Lazy prune: remove stale sessions before creating a new one.
	_, _ = s.sessionDAO.PruneExpiredContext(ctx, time.Now().Add(-chunkUploadTTL))

	tenantID := tenant.IDFromContext(ctx)
	if tenantID == 0 {
		tenantID = 1
	}
	if err := s.enforceStorageQuota(ctx, req.FileSize); err != nil {
		return nil, false, err
	}

	if req.Hash != "" {
		// 秒传：同 hash 已入库，跳过整个上传。
		existing, err := s.fileDAO.GetByHashContext(ctx, req.Hash)
		if err == nil && existing != nil && existing.TenantID == tenantID {
			return nil, true, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		// 断点续传：同 hash 的未完成 session 复用（保留已上传分片 bitmap）。
		if pending, perr := s.sessionDAO.GetPendingByHashContext(ctx, req.Hash, tenantID); perr == nil && pending != nil {
			if pending.FileSize == req.FileSize {
				return pending, false, nil
			}
		}
	}

	total := int((req.FileSize + chunkUploadChunkSize - 1) / chunkUploadChunkSize)
	if total <= 0 {
		total = 1
	}
	session := &model.UploadSession{
		TenantID:    tenantID,
		UserID:      userID,
		FileName:    req.FileName,
		FileSize:    req.FileSize,
		ChunkSize:   chunkUploadChunkSize,
		TotalChunks: total,
		Hash:        req.Hash,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(chunkUploadTTL),
	}
	if err := s.sessionDAO.CreateContext(ctx, session); err != nil {
		return nil, false, err
	}
	return session, false, nil
}

// UploadChunkContext stores one part (1-based) and records it in the bitmap.
// Re-uploading the same part idempotently overwrites it.
func (s *FileService) UploadChunkContext(ctx context.Context, sessionID uint, partN int, body io.Reader) (*model.UploadSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.sessionDAO.GetByIDContext(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChunkSessionNotFound
		}
		return nil, err
	}
	if session.Status != "pending" {
		return nil, ErrChunkSessionDone
	}
	if partN < 1 || partN > session.TotalChunks {
		return nil, ErrChunkPartInvalid
	}

	dir := chunkDir(session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	partPath := filepath.Join(dir, strconv.Itoa(partN))
	dst, err := os.Create(partPath)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(dst, body)
	closeErr := dst.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	bitmap, err := appendChunkBitmap(session.ReceivedBitmap, partN)
	if err != nil {
		return nil, err
	}
	count := session.ReceivedCount
	if !containsChunk(session.ReceivedBitmap, partN) {
		count++
	}
	if err := s.sessionDAO.MarkChunkReceivedContext(ctx, session.ID, bitmap, count); err != nil {
		return nil, err
	}
	session.ReceivedBitmap = bitmap
	session.ReceivedCount = count
	return session, nil
}

// CompleteChunkedUploadContext assembles all parts into one object, creates
// the file record and deletes the session. The assembled bytes run through the
// same StorageProvider as a normal upload.
func (s *FileService) CompleteChunkedUploadContext(ctx context.Context, sessionID uint) (*model.File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.sessionDAO.GetByIDContext(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChunkSessionNotFound
		}
		return nil, err
	}
	if session.Status != "pending" {
		return nil, ErrChunkSessionDone
	}
	if session.ReceivedCount < session.TotalChunks {
		return nil, ErrChunkIncomplete
	}

	dir := chunkDir(session.ID)
	merged, err := mergeChunks(dir, session.TotalChunks)
	if err != nil {
		return nil, err
	}
	defer merged.Close()

	objectKey := fmt.Sprintf("uploads/%s/%s", time.Now().Format("200601"), randomObjectName(session.FileName))
	stored, err := s.uploader.StoreObjectContext(ctx, objectKey, merged)
	if err != nil {
		return nil, err
	}
	_ = os.RemoveAll(dir)

	hash := session.Hash
	if hash == "" {
		if f, err := os.Open(stored.FilePath); err == nil {
			hash = fileHash(f)
			_ = f.Close()
		}
	}

	record := &model.File{
		TenantID:    session.TenantID,
		UserID:      session.UserID,
		FileName:    session.FileName,
		FilePath:    stored.FilePath,
		FileSize:    session.FileSize,
		StorageType: stored.StorageType,
		URL:         stored.URL,
		Hash:        hash,
		Extension:   filepath.Ext(session.FileName),
		MimeType:    detectMime(stored.FilePath),
		FileType:    classifyFileType(detectMime(stored.FilePath)),
	}
	if err := s.fileDAO.CreateContext(ctx, record); err != nil {
		_ = s.uploader.DeleteContext(ctx, stored.FilePath)
		return nil, err
	}
	_ = s.sessionDAO.DeleteContext(ctx, session.ID)
	return record, nil
}

// AbortChunkedUploadContext discards the session and any stored parts.
func (s *FileService) AbortChunkedUploadContext(ctx context.Context, sessionID uint) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := s.sessionDAO.GetByIDContext(ctx, sessionID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChunkSessionNotFound
		}
		return err
	}
	_ = os.RemoveAll(chunkDir(sessionID))
	return s.sessionDAO.DeleteContext(ctx, sessionID)
}

func chunkDir(sessionID uint) string {
	return filepath.Join(config.Cfg.Upload.LocalPath, "chunks", strconv.FormatUint(uint64(sessionID), 10))
}

func mergeChunks(dir string, total int) (io.ReadCloser, error) {
	var parts []string
	for i := 1; i <= total; i++ {
		parts = append(parts, filepath.Join(dir, strconv.Itoa(i)))
	}
	for _, p := range parts {
		if _, err := os.Stat(p); err != nil {
			return nil, ErrChunkIncomplete
		}
	}
	readers := make([]io.Reader, 0, total)
	closers := make([]io.Closer, 0, total)
	for _, p := range parts {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		readers = append(readers, f)
		closers = append(closers, f)
	}
	return &multiCloser{Reader: io.MultiReader(readers...), closers: closers}, nil
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var first error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func appendChunkBitmap(bitmap string, partN int) (string, error) {
	var parts []int
	if strings.TrimSpace(bitmap) != "" {
		if err := json.Unmarshal([]byte(bitmap), &parts); err != nil {
			parts = nil
		}
	}
	if !containsChunk(bitmap, partN) {
		parts = append(parts, partN)
	}
	sort.Ints(parts)
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func containsChunk(bitmap string, partN int) bool {
	var parts []int
	if strings.TrimSpace(bitmap) == "" {
		return false
	}
	if err := json.Unmarshal([]byte(bitmap), &parts); err != nil {
		return false
	}
	for _, p := range parts {
		if p == partN {
			return true
		}
	}
	return false
}

// randomObjectName keeps the original extension for content sniffing.
func randomObjectName(fileName string) string {
	ext := filepath.Ext(fileName)
	return fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
}

func detectMime(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

func classifyFileType(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.Contains(mime, "pdf"), strings.Contains(mime, "spreadsheet"), strings.Contains(mime, "document"):
		return "doc"
	default:
		return "file"
	}
}

func fileHash(f *os.File) string {
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

