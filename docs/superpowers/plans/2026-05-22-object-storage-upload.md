# Object Storage Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `upload.storage_type=s3|minio` 的上传、读取、删除形成完整对象存储闭环。

**Architecture:** 复用现有 `StorageProvider` 接口，把 `reservedObjectStorageProvider` 演进为 MinIO SDK-backed provider。`Store()` 与 `Delete()` 继续使用 object key 作为受控存储标识，URL 只在响应层由 `PublicURL()` 生成。

**Tech Stack:** Go 1.26、`github.com/minio/minio-go/v7`、`net/http/httptest`、标准 `go test`。

---

## 文件结构

- Modify: `server/internal/pkg/upload/storage.go`
  - 为对象存储 provider 增加 `Store()` 和 `Delete()` 的 SDK 实现。
  - 增加小型 helper：从 `io.Reader` 推导对象大小，能推导时传给 `PutObject`，否则回退 `-1` 流式上传。
- Modify: `server/internal/pkg/upload/upload_test.go`
  - 新增 S3 `Store()` 单元测试。
  - 新增 MinIO `Delete()` 单元测试。
  - 更新 MinIO uploader 测试，从 reserved 错误改为真实上传成功。
- Modify: `server/internal/pkg/upload/storage_smoke_test.go`
  - 扩展真实对象存储 smoke，使其通过 provider `Store()` 写入，再 `Open()` 读取，最后 `Delete()` 清理。
- Modify: `docs/superpowers/specs/2026-05-22-object-storage-upload-design.md`
  - 同步记录“可推导大小优先，未知大小回退 `-1`”的 `PutObject` 细化，避免设计文档和实现不一致。
- Modify: `docs/SECURITY.md`
  - 将文件上传安全说明从“对象存储预留”更新为“对象存储已接入 `Store()`、`Open()` 和 `Delete()`”。

---

### Task 1: 写入失败测试

**Files:**
- Modify: `server/internal/pkg/upload/upload_test.go`

- [ ] **Step 1: 添加 S3 Store 失败测试**

在 `TestS3StorageProviderOpenStreamsObject` 前后加入测试：

```go
func TestS3StorageProviderStoreUploadsObject(t *testing.T) {
	const key = "uploads/2026/05/22/avatar.png"
	content := tinyPNG()
	var received []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		received = body
		w.Header().Set("ETag", `"uploaded-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "s3",
		PublicBaseURL: "https://cdn.example.test/uploads",
		S3: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "access",
			SecretKey: "secret",
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	stored, err := provider.Store(context.Background(), key, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if !bytes.Equal(received, content) {
		t.Fatalf("uploaded body length = %d, want %d", len(received), len(content))
	}
	if stored.Key != key {
		t.Fatalf("key = %q, want %q", stored.Key, key)
	}
	if stored.FilePath != key {
		t.Fatalf("file path = %q, want object key", stored.FilePath)
	}
	if stored.StorageType != "s3" {
		t.Fatalf("storage type = %q, want s3", stored.StorageType)
	}
	if stored.URL != "https://cdn.example.test/uploads/"+key {
		t.Fatalf("url = %q, want CDN URL", stored.URL)
	}
}
```

- [ ] **Step 2: 添加 MinIO Delete 失败测试**

```go
func TestMinIOStorageProviderDeleteRemovesObject(t *testing.T) {
	const key = "uploads/2026/05/22/avatar.png"
	deleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/go-admin-kit/"+key {
			t.Fatalf("path = %q, want /go-admin-kit/%s", r.URL.Path, key)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	provider, err := NewStorageProvider(config.UploadConfig{
		StorageType:   "minio",
		PublicBaseURL: "http://127.0.0.1:9000/go-admin-kit",
		MinIO: config.ObjectStorageConfig{
			Endpoint:  server.URL,
			Bucket:    "go-admin-kit",
			Region:    "us-east-1",
			AccessKey: "minio-access",
			SecretKey: "minio-secret",
			UseSSL:    false,
		},
	})
	if err != nil {
		t.Fatalf("new provider failed: %v", err)
	}

	if err := provider.Delete(context.Background(), key); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !deleted {
		t.Fatal("delete request was not sent")
	}
}
```

- [ ] **Step 3: 更新 MinIO uploader 测试**

把 `TestUploaderMinIOConfiguredButReservedIsExplicit` 改名为 `TestUploaderMinIOConfiguredUploadsObject`，并使用 `httptest.Server` 接收 `PUT` 请求，断言 `Upload()` 返回 `StorageType=minio` 且 `FilePath` 是 object key。

- [ ] **Step 4: 运行失败测试**

Run:

```bash
go test ./internal/pkg/upload -run "TestS3StorageProviderStoreUploadsObject|TestMinIOStorageProviderDeleteRemovesObject|TestUploaderMinIOConfiguredUploadsObject" -count=1
```

Expected: FAIL，失败原因包含 reserved `Store()` / `Delete()` 行为或测试名尚未通过。

---

### Task 2: 实现对象存储 Store/Delete

**Files:**
- Modify: `server/internal/pkg/upload/storage.go`

- [ ] **Step 1: 实现 Store**

替换 `reservedObjectStorageProvider.Store`：

```go
func (p *reservedObjectStorageProvider) Store(ctx context.Context, objectKey string, body io.Reader) (*StoredObject, error) {
	if p.configErr != nil {
		return nil, p.configErr
	}
	key, err := cleanObjectKey(objectKey)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newObjectStorageClient(p.storageType, p.cfg)
	if err != nil {
		return nil, err
	}
	size := objectSize(body)
	if _, err := client.PutObject(ctx, p.cfg.Bucket, key, body, size, minio.PutObjectOptions{}); err != nil {
		return nil, fmt.Errorf("%w: %s put %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	publicURL, err := p.PublicURL(key)
	if err != nil {
		return nil, err
	}
	return &StoredObject{
		Key:         key,
		FilePath:    key,
		URL:         publicURL,
		StorageType: p.Type(),
	}, nil
}
```

- [ ] **Step 2: 实现 Delete**

替换 `reservedObjectStorageProvider.Delete`：

```go
func (p *reservedObjectStorageProvider) Delete(ctx context.Context, filePath string) error {
	if p.configErr != nil {
		return p.configErr
	}
	key, err := cleanObjectKey(filePath)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newObjectStorageClient(p.storageType, p.cfg)
	if err != nil {
		return err
	}
	if err := client.RemoveObject(ctx, p.cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("%w: %s delete %q failed: %v", ErrStorageProviderUnavailable, p.storageType, key, err)
	}
	return nil
}
```

- [ ] **Step 3: 增加 objectSize helper**

在 `storage.go` 中加入：

```go
func objectSize(reader io.Reader) int64 {
	switch r := reader.(type) {
	case interface{ Len() int }:
		return int64(r.Len())
	case io.Seeker:
		current, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return -1
		}
		end, err := r.Seek(0, io.SeekEnd)
		if err != nil {
			_, _ = r.Seek(current, io.SeekStart)
			return -1
		}
		_, _ = r.Seek(current, io.SeekStart)
		return end - current
	default:
		return -1
	}
}
```

- [ ] **Step 4: 运行上传包测试**

Run:

```bash
go test ./internal/pkg/upload -count=1
```

Expected: PASS。

---

### Task 3: 扩展真实对象存储 smoke 链路

**Files:**
- Modify: `server/internal/pkg/upload/storage_smoke_test.go`

- [ ] **Step 1: 改为 provider 写入**

在 `TestObjectStorageSmokeOpenReadsRealEndpoint` 中，创建 provider 后先调用：

```go
stored, err := provider.Store(ctx, objectKey, strings.NewReader(content))
if err != nil {
	t.Fatalf("store smoke object through provider: %v", err)
}
if stored.FilePath != objectKey {
	t.Fatalf("stored file path = %q, want %q", stored.FilePath, objectKey)
}
```

然后删除直接 `client.PutObject(...)` 的写入代码。

- [ ] **Step 2: 清理由 SDK client 改为 provider Delete**

在 `t.Cleanup` 中调用：

```go
if err := provider.Delete(cleanupCtx, objectKey); err != nil {
	t.Logf("cleanup smoke object %q failed: %v", objectKey, err)
}
```

- [ ] **Step 3: 运行 smoke 默认路径**

Run:

```bash
go test ./internal/pkg/upload -run TestObjectStorageSmokeOpenReadsRealEndpoint -count=1
```

Expected: PASS with skip message，除非设置了 `BLACK8_OBJECT_STORAGE_SMOKE=1`。

---

### Task 4: 收尾验证

**Files:**
- Verify only

- [ ] **Step 1: 运行上传包测试**

Run:

```bash
go test ./internal/pkg/upload -count=1
```

Expected: PASS。

- [ ] **Step 2: 运行后端全量测试**

Run:

```bash
go test ./... -count=1
```

Expected: PASS；若有与本改动无关的既有失败，记录具体包和错误。

- [ ] **Step 3: 查看本次改动**

Run:

```bash
git diff -- server/internal/pkg/upload docs/SECURITY.md docs/superpowers/plans/2026-05-22-object-storage-upload.md docs/superpowers/specs/2026-05-22-object-storage-upload-design.md
```

Expected: 只包含对象存储上传、删除、smoke 与对应文档相关改动。
