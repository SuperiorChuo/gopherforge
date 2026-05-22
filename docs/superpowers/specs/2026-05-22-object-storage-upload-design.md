# Go Admin Kit 对象存储上传设计

## 文档信息

- 日期：2026-05-22
- 状态：设计已确认，待进入 implementation plan
- 范围：补全 `upload.storage_type=s3|minio` 时的 `Store()` 与 `Delete()` 行为
- 目标文件：`server/internal/pkg/upload/storage.go` 与对应测试

## 背景

当前上传模块已经抽象出 `StorageProvider`，本地存储通过 `LocalStorageProvider` 完成写入、读取、删除和 URL 生成。S3/MinIO 路径已经接入 `github.com/minio/minio-go/v7`，并实现了 `Open()`，但 `Store()` 与 `Delete()` 仍返回 reserved 错误。

这会导致配置切到 `s3` 或 `minio` 后，文件上传无法落地；同时业务层在数据库落库失败时调用 `DeleteContext()` 做补偿清理，也无法真正删除对象。

## 目标

1. 在 `upload.storage_type=s3` 或 `upload.storage_type=minio` 时，上传 API 可以通过对象存储写入文件。
2. 上传成功后返回稳定的 `FilePath`、`URL` 与 `StorageType`，保持现有 `FileInfo` 响应结构不变。
3. 数据库落库失败或用户主动删除文件时，`DeleteContext()` 可以删除对象存储中的对象。
4. 继续保留现有配置校验、object key 清洗、公共 URL 拼接和 SDK 错误包装语义。

## 非目标

1. 不新增预签名上传、预签名下载或私有桶鉴权下载。
2. 不新增对象存储 bucket 自动创建或 bucket policy 管理。
3. 不改变本地存储行为。
4. 不改变上传 API、前端类型或数据库模型。
5. 不处理多分片上传、断点续传、秒传去重或图片处理流水线。

## 方案

采用最小闭环方案：把现有 `reservedObjectStorageProvider` 演进为 SDK-backed provider，复用已存在的配置校验、`newObjectStorageClient()`、`cleanObjectKey()` 和 `joinPublicURL()`。

### Store 行为

- 入参 `objectKey` 必须经过 `cleanObjectKey()` 清洗，拒绝空值、绝对路径、路径穿越和 URL。
- `ctx == nil` 时与现有 `Open()` 一致回退为 `context.Background()`。
- 调用 MinIO SDK 的 `PutObject(ctx, bucket, key, body, size, minio.PutObjectOptions{})`；当 `body` 可通过 `Len()` 或 `io.Seeker` 推导剩余大小时传入明确 size，无法推导时回退为 `-1` 流式上传。
- 返回：
  - `Key`: 清洗后的 object key
  - `FilePath`: 清洗后的 object key
  - `URL`: `PublicURL(key)` 的结果
  - `StorageType`: `s3` 或 `minio`

`FilePath` 使用 object key，而不是完整 URL，原因是后续 `Open()`、`Delete()` 和 `PublicURL()` 都应以受控的相对 key 为输入，避免把外部 URL 当作可信存储标识。

### Delete 行为

- 入参 `filePath` 继续按 object key 清洗。
- 调用 MinIO SDK 的 `RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})`。
- SDK 返回错误时，用 `ErrStorageProviderUnavailable` 包装，并包含 provider 类型和 key，便于 API 层生成稳定错误响应。

### 错误处理

- 配置缺失仍返回 `ErrStorageProviderNotConfigured`，错误文本保留 `upload.s3 missing ...` 或 `upload.minio missing ...`。
- SDK 初始化、上传、删除失败统一包装为 `ErrStorageProviderUnavailable`。
- object key 非法仍返回 `ErrStorageProviderNotConfigured`，保持当前路径校验语义。

## 测试设计

采用 TDD：

1. 新增 `TestS3StorageProviderStoreUploadsObject`，用 `httptest.Server` 模拟 S3-compatible `PUT /bucket/key`，断言写入内容、返回 key、`FilePath`、URL 与 `StorageType`。
2. 新增 `TestMinIOStorageProviderDeleteRemovesObject`，用 `httptest.Server` 模拟 `DELETE /bucket/key`，断言 provider 发出删除请求。
3. 更新原有 `TestUploaderMinIOConfiguredButReservedIsExplicit`，让它验证配置完整时 `Uploader.Upload()` 可以完成对象存储上传，而不是继续期待 reserved 错误。
4. 保留 `TestUploaderS3MissingConfigIsExplicit`，确认缺失配置仍短路失败，不访问 SDK。
5. 保留 object key raw URL 拒绝测试，避免上传、读取、删除接收外部 URL。

必要时扩展 smoke test，使真实对象存储冒烟可以覆盖 `Store()`、`Open()`、`Delete()` 完整链路；该测试仍通过环境变量显式启用。

## 风险与控制

- MinIO SDK 默认会根据 endpoint 和 bucket 选择请求样式；单元测试只验证兼容路径，不绑定生产网络环境。
- `PutObject` 优先使用可推导的剩余大小，便于 SDK 选择单次 PUT；无法推导时使用 unknown size 流式上传，避免在上传层额外缓存整文件。文件大小仍由 `Uploader` 的 `MaxSize` 负责限制。
- `Delete()` 对不存在对象的具体行为由后端实现决定，当前只保证 SDK 错误被可识别地包装。
- 不自动创建 bucket，避免应用进程在生产环境拥有过宽的存储管理权限。

## 验收标准

1. `go test ./internal/pkg/upload` 通过。
2. `go test ./...` 在当前后端工作区通过，若存在与本改动无关的既有失败，需要明确记录。
3. S3/MinIO provider 的 `Store()`、`Open()`、`Delete()` 都有单元测试或显式 smoke 路径覆盖。
4. 本地存储上传和删除测试保持通过。
