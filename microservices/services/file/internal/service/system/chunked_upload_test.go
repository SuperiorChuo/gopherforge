package system

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-admin-kit/services/file/internal/config"

	"github.com/go-admin-kit/services/file/internal/model"
	"gorm.io/gorm"
)

// fakeUploadSessionDAO is an in-memory UploadSessionDAO for the service test.
type fakeUploadSessionDAO struct {
	rows map[uint]*model.UploadSession
	next uint
}

func newFakeUploadSessionDAO() *fakeUploadSessionDAO {
	return &fakeUploadSessionDAO{rows: map[uint]*model.UploadSession{}, next: 1}
}

func (f *fakeUploadSessionDAO) CreateContext(_ context.Context, s *model.UploadSession) error {
	s.ID = f.next
	f.next++
	f.rows[s.ID] = s
	return nil
}

func (f *fakeUploadSessionDAO) GetByIDContext(_ context.Context, id uint) (*model.UploadSession, error) {
	s, ok := f.rows[id]
	if !ok {
		return &model.UploadSession{}, gorm.ErrRecordNotFound
	}
	return s, nil
}

func (f *fakeUploadSessionDAO) MarkChunkReceivedContext(_ context.Context, id uint, bitmap string, count int) error {
	s, ok := f.rows[id]
	if !ok || s.Status != "pending" {
		return gorm.ErrRecordNotFound
	}
	s.ReceivedBitmap = bitmap
	s.ReceivedCount = count
	return nil
}

func (f *fakeUploadSessionDAO) DeleteContext(_ context.Context, id uint) error {
	delete(f.rows, id)
	return nil
}

func (f *fakeUploadSessionDAO) PruneExpiredContext(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func TestChunkBitmapAppend(t *testing.T) {
	b, err := appendChunkBitmap("", 3)
	if err != nil || b != "[3]" {
		t.Fatalf("first append = %q err %v", b, err)
	}
	b, _ = appendChunkBitmap(b, 1)
	b, _ = appendChunkBitmap(b, 2)
	if b != "[1,2,3]" {
		t.Fatalf("sorted bitmap = %q, want [1,2,3]", b)
	}
	b, _ = appendChunkBitmap(b, 2) // 幂等
	if b != "[1,2,3]" {
		t.Fatalf("duplicate append changed bitmap: %q", b)
	}
	if !containsChunk(b, 2) || containsChunk(b, 9) {
		t.Fatal("containsChunk wrong")
	}
}

func TestChunkPartValidation(t *testing.T) {
	svc := &FileService{sessionDAO: newFakeUploadSessionDAO()}
	sess := &model.UploadSession{ID: 1, TotalChunks: 3, Status: "pending", ChunkSize: 5}
	_ = svc.sessionDAO.CreateContext(context.Background(), sess)

	if _, err := svc.UploadChunkContext(context.Background(), 1, 0, bytes.NewReader(nil)); err != ErrChunkPartInvalid {
		t.Fatalf("part 0 error = %v, want ErrChunkPartInvalid", err)
	}
	if _, err := svc.UploadChunkContext(context.Background(), 1, 4, bytes.NewReader(nil)); err != ErrChunkPartInvalid {
		t.Fatalf("part 4 error = %v, want ErrChunkPartInvalid", err)
	}
	if _, err := svc.UploadChunkContext(context.Background(), 999, 1, bytes.NewReader(nil)); err != ErrChunkSessionNotFound {
		t.Fatalf("missing session error = %v, want ErrChunkSessionNotFound", err)
	}
}

func TestChunkCompleteRequiresAllParts(t *testing.T) {
	config.Cfg.Upload.LocalPath = t.TempDir()
	svc := &FileService{sessionDAO: newFakeUploadSessionDAO()}
	sess := &model.UploadSession{ID: 1, TenantID: 1, UserID: 1, TotalChunks: 2, Status: "pending", ChunkSize: 5, FileName: "a.bin"}
	_ = svc.sessionDAO.CreateContext(context.Background(), sess)

	// 只传 1 片 → complete 拒绝
	if _, err := svc.CompleteChunkedUploadContext(context.Background(), 1); err != ErrChunkIncomplete {
		t.Fatalf("incomplete complete error = %v, want ErrChunkIncomplete", err)
	}
}

func TestMergeChunksAssemblesInOrder(t *testing.T) {
	dir := t.TempDir()
	// 分片 1 = "hel", 分片 2 = "lo,", 分片 3 = "world"
	if err := os.WriteFile(filepath.Join(dir, "1"), []byte("hel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2"), []byte("lo,"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "3"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	merged, err := mergeChunks(dir, 3)
	if err != nil {
		t.Fatalf("mergeChunks error = %v", err)
	}
	defer merged.Close()
	got, err := io.ReadAll(merged)
	if err != nil {
		t.Fatalf("read merged error = %v", err)
	}
	if string(got) != "hello,world" {
		t.Fatalf("merged = %q, want hello,world", got)
	}

	// 缺片 → ErrChunkIncomplete
	if err := os.Remove(filepath.Join(dir, "2")); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeChunks(dir, 3); err != ErrChunkIncomplete {
		t.Fatalf("missing part error = %v, want ErrChunkIncomplete", err)
	}
}
