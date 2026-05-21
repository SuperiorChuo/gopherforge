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
- API handler 已增加源码级测试，禁止把 `err.Error()` 直接写入用户响应。

### 上下文传播与分层

- 主要 API/Service/DAO 链路已补充 `context.Context` 版本，GORM 查询使用 `WithContext(ctx)`。
- 共享 `dao.UserDAO` 已抽出，`dao/auth` 与 `dao/system` 只保留各自领域方法。
- `OAuthService` 已支持注入 user/binding store，默认行为保持兼容。
- `authz.DataScopeResolver` 已支持注入 `DataScopeStore` 与部门树缓存，部门树和自定义角色部门加载不再硬绑主流程。
- `RedisService` 已支持注入 Redis monitor client，默认仍使用全局 Redis client。
- `pkg/jwt` 的 token blacklist 已支持注入 `TokenBlacklistStore`，默认 Redis 行为保持兼容。
- `pkg/cache` 的 `CacheService` 已支持注入 Redis client，验证码、用户信息和权限缓存调用可脱离全局 Redis 进行测试。
- 登录失败限制已抽出 `LoginLimiter`，支持注入 Redis client，包级函数继续保持兼容。
- 限流中间件已抽出 `RateLimiter`，支持注入 Redis client，默认 `RateLimit(config)` 调用保持兼容。
- `OnlineUserService` 已支持注入 Redis client，在线用户记录、索引、计数和强制下线逻辑可脱离全局 Redis 测试。
- `HealthAPI` 已支持注入 database client 与 Redis ping client，健康检查可脱离全局依赖测试，同时保持依赖错误脱敏。

### 性能与资源安全

- Metrics 中间件的核心计数改为原子/更低锁竞争实现，数据库连接池统计已支持注入 provider。
- 操作日志中间件读取 request body 时使用 `io.LimitReader`。
- 部门树数据权限解析已增加 Redis 缓存、失效逻辑与可注入缓存接口。
- 在线用户查询已从 Redis `SCAN` 改为 zset 索引和批量 `MGET`。

### CI、部署与前端

- GitHub Actions 已运行 Go coverage、`go vet`、`golangci-lint`、前端测试、构建和 integration smoke。
- CI 已使用 Node 24 兼容路径，不再依赖 Node 20 action runtime。
- Dockerfile 已固定 Alpine 版本并使用非 root 用户运行。
- Docker 后端容器已统一走 goose migration。
- Makefile `status`/`logs` 已改为 Windows 友好的 Node 脚本。
- Vite dev server 已代理 `/uploads`。
- 前端路由守卫已将不存在路由导向 404。

## 当前剩余

### 可逐步推进

- 旧的非 `Context` convenience 方法仍保留，用于兼容历史调用；后续可在大版本中逐步收敛。
- 业务错误响应目前是稳定 message 映射；如果要多语言或更强契约，可升级为统一 error code catalog。
- 部分 service/pkg/DAO 仍允许零值结构体回退到全局依赖，这是兼容策略；新代码优先使用构造函数或注入接口。

### 发布前建议复核

- 运行后端全量验证：

```powershell
cd server
go test ./...
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --config ../.golangci.yml ./...
```

- 运行前端和契约验证：

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
```

- 发布前继续使用 `docs/development/READINESS_CHECKLIST.md` 做环境、密钥、迁移、健康检查和 Docker 验证。
