package system

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/go-admin-kit/services/file/internal/config"
	systemdao "github.com/go-admin-kit/services/file/internal/dao/system"

	localmodel "github.com/go-admin-kit/services/file/internal/model"
	"gorm.io/gorm"
)

// fakeUploadSessionDAO is an in-memory UploadSessionDAO for the service test.
type fakeUploadSessionDAO struct {
	rows map[uint]*localmodel.UploadSession
	next uint
}

func newFakeUploadSessionDAO() *fakeUploadSessionDAO {
	return &fakeUploadSessionDAO{rows: map[uint]*localmodel.UploadSession{}, next: 1}
}

func (f *fakeUploadSessionDAO) CreateContext(_ context.Context, s *localmodel.UploadSession) error {
	s.ID = f.next
	f.next++
	f.rows[s.ID] = s
	return nil
}

func (f *fakeUploadSessionDAO) GetByIDContext(_ context.Context, id uint) (*localmodel.UploadSession, error) {
	s, ok := f.rows[id]
	if !ok {
		return &localmodel.UploadSession{}, gorm.ErrRecordNotFound
	}
	return s, nil
}

func (f *fakeUploadSessionDAO) GetPendingByHashContext(_ context.Context, hash string, tenantID, userID uint) (*localmodel.UploadSession, error) {
	// 与真实 DAO 对齐：tenant_id + user_id 过滤 + id DESC 取最新
	var best *localmodel.UploadSession
	for _, s := range f.rows {
		if s.Hash == hash && s.Status == "pending" && s.TenantID == tenantID && s.UserID == userID {
			if best == nil || s.ID > best.ID {
				best = s
			}
		}
	}
	if best == nil {
		return &localmodel.UploadSession{}, gorm.ErrRecordNotFound
	}
	return best, nil
}

func (f *fakeUploadSessionDAO) UpdateFileNameContext(_ context.Context, id uint, fileName string) error {
	s, ok := f.rows[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	s.FileName = fileName
	return nil
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
	sess := &localmodel.UploadSession{ID: 1, TotalChunks: 3, Status: "pending", ChunkSize: 5}
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

func TestChunkResumeReusesPendingSession(t *testing.T) {
	// fileDAO 用 sqlmock：quota 查询报错 → enforceStorageQuota fail-open（不限量）
	db, mock := setupSystemUserServiceContextTestDB(t)
	// 三次 Init（同 hash 续传/不同 size 新建/跨用户拒绝）各触发 quota + hash 查询；
	// 改名场景用独立 hash，其 mock 在下方单独注册
	for range 3 {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(storage_quota_mb, 0) FROM tenant_packages WHERE id = $1`)).
			WithArgs(uint(1)).WillReturnError(gorm.ErrRecordNotFound)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE tenant_id = $1 AND hash = $2 ORDER BY "files"."id" LIMIT $3`)).
			WithArgs(uint(1), "abc123", 1).WillReturnError(gorm.ErrRecordNotFound)
	}
	svc := &FileService{sessionDAO: newFakeUploadSessionDAO(), fileDAO: *systemdao.NewFileDAO(db)}
	sess := &localmodel.UploadSession{ID: 1, TenantID: 1, UserID: 1, Hash: "abc123", FileSize: 100, TotalChunks: 2, Status: "pending", ChunkSize: 5, ReceivedBitmap: "1"}
	_ = svc.sessionDAO.CreateContext(context.Background(), sess)

	// 同 hash 再次 Init → 返回既有 session（保留 bitmap），不新建
	got, already, err := svc.InitChunkedUploadContext(context.Background(), ChunkedInitRequest{
		FileName: "a.bin", FileSize: 100, Hash: "abc123",
	}, 1)
	if err != nil {
		t.Fatalf("init resume: %v", err)
	}
	if already {
		t.Fatal("resume must not report already_exists (hash not in files table)")
	}
	if got == nil || got.ID != 1 || got.ReceivedBitmap != "1" {
		t.Fatalf("resume session = %+v, want id=1 bitmap=1", got)
	}

	// 不同 FileSize → 不续传，新建 session
	got2, _, err := svc.InitChunkedUploadContext(context.Background(), ChunkedInitRequest{
		FileName: "a.bin", FileSize: 200, Hash: "abc123",
	}, 1)
	if err != nil {
		t.Fatalf("init different size: %v", err)
	}
	if got2 == nil || got2.ID == 1 {
		t.Fatalf("different-size init = %+v, want new session", got2)
	}

	// 跨用户（user 2）→ 不接管 user 1 的 session，新建
	got3, _, err := svc.InitChunkedUploadContext(context.Background(), ChunkedInitRequest{
		FileName: "a.bin", FileSize: 100, Hash: "abc123",
	}, 2)
	if err != nil {
		t.Fatalf("init cross-user: %v", err)
	}
	if got3 == nil || got3.ID == 1 || got3.UserID != 2 {
		t.Fatalf("cross-user init = %+v, want new session owned by user 2", got3)
	}

	// 同人重传改名（独立 hash，避免不同-size 的 ID=2 挡在前面）→ 回写新文件名
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(storage_quota_mb, 0) FROM tenant_packages WHERE id = $1`)).
		WithArgs(uint(1)).WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "files" WHERE tenant_id = $1 AND hash = $2 ORDER BY "files"."id" LIMIT $3`)).
		WithArgs(uint(1), "renamed-hash", 1).WillReturnError(gorm.ErrRecordNotFound)
	sessRename := &localmodel.UploadSession{TenantID: 1, UserID: 1, Hash: "renamed-hash", FileSize: 100, TotalChunks: 2, Status: "pending", ChunkSize: 5, FileName: "old.bin"}
	_ = svc.sessionDAO.CreateContext(context.Background(), sessRename)
	got4, _, err := svc.InitChunkedUploadContext(context.Background(), ChunkedInitRequest{
		FileName: "renamed.bin", FileSize: 100, Hash: "renamed-hash",
	}, 1)
	if err != nil {
		t.Fatalf("init rename: %v", err)
	}
	if got4 == nil || got4.ID != sessRename.ID || got4.FileName != "renamed.bin" {
		t.Fatalf("rename resume = %+v, want id=%d file_name=renamed.bin", got4, sessRename.ID)
	}
}

func TestChunkCompleteRequiresAllParts(t *testing.T) {
	config.Cfg.Upload.LocalPath = t.TempDir()
	svc := &FileService{sessionDAO: newFakeUploadSessionDAO()}
	sess := &localmodel.UploadSession{ID: 1, TenantID: 1, UserID: 1, TotalChunks: 2, Status: "pending", ChunkSize: 5, FileName: "a.bin"}
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
