# 脚手架优化状态

本文记录最近一轮 Go Admin Kit 脚手架稳定性、安全性、可测试性和工程化优化的完成情况，便于后续接手和发布前复核。

## 已完成

### 生产稳定性

- `cmd/main.go` 已改为 `http.Server` + signal handling，支持 graceful shutdown。
- Gin 默认输出已使用 `io.Discard`，避免 `gin.DefaultWriter = nil` 引发 panic。
- 数据库连接池已补充 `ConnMaxLifetime` 和 `ConnMaxIdleTime`。
- 验证码响应不再返回 `code_hint` 明文。

### 安全与认证

- 限流改为 Redis `INCR` first 模式，避免 GET/INCR 竞态。
- JWT blacklist 已使用 token JTI 作为 Redis key，不再使用完整 token。
- Console session cookie 已按安全配置设置 `Secure`。
- 在线用户记录不再存储明文 access token，改用 token ID。
- API 500 错误统一走日志记录 + 稳定用户可见消息，避免泄露内部错误。
- API 错误响应已补充稳定 `error_code` 字段，认证和系统模块的已知业务错误已接入领域错误码。
- 认证中间件的 JWT 过期、无效、撤销和 token 类型错误已返回稳定认证错误码。
- 认证/权限/限流中间件的缺少鉴权头、鉴权头格式错误、用户上下文缺失、Console 登录缺失、请求限流和登录锁定场景已返回细分稳定错误码。
- API handler 已增加源码级测试，禁止把 `err.Error()` 直接写入用户响应。

### 上下文传播与分层

- 主要 API/Service/DAO 链路已补充 `context.Context` 版本，GORM 查询使用 `WithContext(ctx)`。
- 共享 `dao.UserDAO` 已抽出，`dao/auth` 与 `dao/system` 只保留各自领域方法。
- `OAuthService` 已支持注入 user/binding store，默认行为保持兼容。
- `authz.DataScopeResolver` 已支持注入 `DataScopeStore` 与部门树缓存，部门树和自定义角色部门加载不再硬绑主流程。
- `RedisService` 已提供公开注入构造函数，默认仍使用全局 Redis client。
- `pkg/jwt` 的 token blacklist 已支持注入 `TokenBlacklistStore`，默认 Redis 行为保持兼容。
- `pkg/cache` 的 `CacheService` 已支持注入 Redis client，验证码、用户信息和权限缓存调用可脱离全局 Redis 进行测试。
- 登录失败限制已抽出 `LoginLimiter`，支持注入 Redis client，包级函数继续保持兼容。
- 限流中间件已抽出 `RateLimiter`，支持注入 Redis client，默认 `RateLimit(config)` 调用保持兼容。
- `OnlineUserService` 已支持注入 Redis client，在线用户记录、索引、计数和强制下线逻辑可脱离全局 Redis 测试。
- `HealthAPI` 已支持注入 database client 与 Redis ping client，健康检查可脱离全局依赖测试，同时保持依赖错误脱敏。
- API 层已增加架构守护测试，阻止 handler 新增对全局 `database.DB` 或 Redis `Client` 的直接依赖；健康检查兼容 fallback 已精确列入 allowlist。
- 已移除项目内所有 `// Deprecated: use ...Context` legacy wrapper；`authz.ResolveUserDataScope` 原有出错回退语义已迁移为显式 `ResolveUserDataScopeFallbackContext`，`pkg/ipinfo` 默认 helper 和登录日志位置解析也已改走 Context 链路。架构测试会拦截直接、经局部变量或简单别名传播传入 `context.Background()` 的已清理前缀 wrapper，并保留 `AuditLogService.Record` 这类非纯 Gin bridge 的精确 allowlist。
- 表名前缀已统一：运行时代码使用 `audit_logs`、`console_routes`、`console_sessions`、`system_settings`，并通过 `000007_rename_wm_tables.sql` 将历史 `wm_` 表迁移到统一命名；`server/docs/go_admin_kit.sql` 快照同步使用新表名。
- service/pkg/DAO/middleware 中保留的全局 DB/Redis fallback 已纳入精确 allowlist 架构测试，防止兼容兜底继续扩散。
- OAuth bind/unbind 已从空实现改为服务层与 DAO 层闭环：绑定只使用服务端解析出的 provider identity，按当前登录用户写入，支持重复绑定、被其他用户绑定、未找到绑定等稳定错误码；`oauth_bindings` 已通过 `000008_add_oauth_binding_user_provider_unique.sql` 增加 `(user_id, provider)` 唯一键，并在迁移前清理同用户同 provider 的历史重复行。
- GitHub OAuth 已接入真实 web flow：登录 URL 会生成一次性 Redis-backed `state` 与 PKCE `S256` challenge，回调时先 `GETDEL` 消费 state/code verifier，再向 GitHub token endpoint 换取 access token，并通过 `/user` 重新确认身份；GitHub 关闭或配置缺失时返回 `AUTH_OAUTH_PROVIDER_UNAVAILABLE`。WeChat OAuth 已接入开放平台扫码登录 flow：登录 URL 生成一次性 Redis `state`，回调先消费 state，再向微信 token endpoint 换取 access token 并调用 `/sns/userinfo` 确认身份；关闭、配置缺失或 provider 异常时保持 fail-closed。
- `system_settings.security.policy` 已从纯 CRUD 配置升级为运行时配置源：`password_max_age_days`、`password_history_count`、`login_limit_max_failures`、`login_limit_window_minutes`、`login_limit_lock_minutes` 和 `rate_limit_rps` 会合并 YAML/env fallback 后进入内存快照；保存或删除该 key 后当前进程立即刷新，并通过 Redis Pub/Sub 广播给其他实例，登录、改密、OAuth 登录、Console 登录和全局限流会读取该快照。
- 前端系统设置页已补齐 `security.policy` 六个后端运行时字段，保存时随 `system_settings.security.policy` 批量提交，并通过契约测试防止字段再次缺失。
- 邮件通知已完成第一阶段：`notification.email` 可覆盖非密钥字段、TLS/STARTTLS 模式、纯文本 subject/body 模板和 `recipient_groups.notice` 收件组，YAML/env 保留 SMTP 密钥配置；公告/通知启用后会尝试向告警收件人发送邮件，邮件失败只记录日志，不影响公告接口返回。

### 性能与资源安全

- Metrics 中间件的核心计数改为原子/更低锁竞争实现，数据库连接池统计已支持注入 provider。
- 操作日志中间件读取 request body 时使用 `io.LimitReader`。
- 部门树数据权限解析已增加 Redis 缓存、失效逻辑与可注入缓存接口。
- 在线用户查询已从 Redis `SCAN` 改为 zset 索引和批量 `MGET`。

### CI、部署与前端

- GitHub Actions 已运行 Go coverage、`go vet`、`golangci-lint`、前端测试、构建和 integration smoke。
- CI 已使用 Node 24 兼容路径，不再依赖 Node 20 action runtime。
- `typedApi` 已补充运行时单测，覆盖 path 编码、缺参报错、`/api/v1` 前缀裁剪以及 query/body/options 透传。
- Dockerfile 已固定 Alpine 版本并使用非 root 用户运行。
- Docker 后端容器已统一走 goose migration。
- 已新增 `cmd/migration-rehearsal` 与 `make migrate-rehearse`，可在一次性数据库中执行 `up -> down-to 0 -> up` 演练；CI integration job 在启动 API smoke 前会先跑迁移演练。
- Makefile `status`/`logs` 已改为 Windows 友好的 Node 脚本。
- Vite dev server 已代理 `/uploads`。
- 文件下载和预览已改为通过 `StorageProvider.Open` 流式读取，支持 `local`、`s3`、`minio` 记录按 `storage_type` 选择对应 provider；对象缺失统一映射为文件 404，`Content-Disposition` 使用标准格式化并过滤换行。
- 图片上传已记录尺寸与缩略图元数据：JPEG/PNG/GIF 会读取 `image_width` 与 `image_height`，读取后重置 reader，避免影响 hash 和对象存储内容；同时为 JPEG/PNG/GIF 生成静态 PNG 缩略图并持久化 `thumbnail_*` 元数据，前端文件列表优先展示缩略图。
- OpenAPI 契约已区分文件 download/preview 的二进制响应，并补充 OAuth login/callback 的 302/503、bind/unbind 的请求体和 409/404/503 错误响应；前端生成类型和契约测试同步覆盖。
- 前端 `dev-server` 端口守卫默认值已与 Vite `server.port` 保持一致。
- 前端路由守卫已将不存在路由导向 404。
- 旧的根目录 Playwright E2E 入口已收敛为迁移说明，正式浏览器 E2E 统一从 `tdesign-vue-go/e2e` 运行。

## 当前剩余

### 发布前建议复核

- 代码层面的 P0/P1 优化项已继续收口；GitHub/WeChat OAuth 真实第三方登录、`security.policy`/`notification.email` 多实例运行时热生效、SMTP TLS/STARTTLS、模板化邮件、多收件组策略、图片尺寸与缩略图元数据、迁移 Up/Down 演练入口和 API smoke 扩面已完成。后续重点剩余为本机依赖栈恢复后的真实 API smoke。
- 2026-05-21 已完成本机自动化复核：后端测试、`go vet`、`golangci-lint`、OpenAPI 契约、API smoke 单元测试、前端测试/类型检查/lint/stylelint/build/E2E，以及 Docker compose 本机烟测均通过。
- 上线前仅剩目标环境人工确认项：生产密钥、默认密码、CORS 域名、上传存储、目标库迁移、默认管理员密码、正式域名下的鉴权/审计/页面访问和前后端网关访问。

- 需要复跑时可使用后端全量验证命令：

```powershell
cd server
go test ./...
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --config ../.golangci.yml ./...
```

- 需要复跑时可使用前端和契约验证命令：

```powershell
npm run api:contract
npm run test:contract
npm run test:smoke:unit
cd tdesign-vue-go
npm run test
npm run build:type
npm run lint
npm run stylelint
npm run build
npm run e2e:frontend
```

- 发布前继续使用 `docs/development/READINESS_CHECKLIST.md` 跟踪目标环境确认项。
