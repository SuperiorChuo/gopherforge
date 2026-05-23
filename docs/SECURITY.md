# 安全治理

## 已启用的默认能力

- 请求级限流：默认 `100 req/s/IP`
- 登录失败锁定：默认 15 分钟内失败 5 次，锁定 30 分钟
- 安全响应头：`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`、`Permissions-Policy`
- 请求 ID：所有请求返回并记录 `X-Request-ID`
- 生产配置校验：生产环境会拒绝弱 JWT secret、默认数据库密码、危险 CORS 组合
- 敏感日志脱敏：密码、token、secret 等字段会在操作日志里脱敏
- Token 撤销：退出登录会撤销 access token，refresh token 默认轮换并撤销旧 token
- 强制下线：在线用户管理会撤销目标用户当前 Redis 记录的 access token，后续请求返回 401
- GitHub OAuth：启用真实 GitHub provider 时，登录流程使用 Redis-backed 一次性 `state` 和 PKCE `S256`，回调必须先消费 state/code verifier，再换取 access token 并调用 GitHub `/user` 重新确认身份
- WeChat OAuth：启用真实 WeChat provider 时，登录流程使用开放平台扫码登录、一次性 Redis `state`、服务端 token exchange 和 `/sns/userinfo` 身份确认；配置缺失或 provider 返回异常时 fail-closed
- 强制改密：可通过 `DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD=true` 要求默认管理员首次登录后修改密码
- 运行时安全策略：`system_settings.security.policy` 可覆盖密码过期天数、密码历史数量、登录失败阈值和 RPS 限流；保存或删除该 key 后会刷新当前进程内存快照，并通过 Redis Pub/Sub 通知其他实例刷新
- 邮件通知：`notification.email` 可覆盖启用状态、SMTP 主机、发件人、告警收件人、收件组、纯文本模板和 TLS/STARTTLS 模式；SMTP 用户名和密码建议通过环境变量配置，公告/通知启用后的邮件发送失败不会阻断业务接口
- HTTP 状态码：认证、授权、参数、资源不存在和限流分别返回 401/403/400/404/429
- 文件上传校验：后缀白名单、大小限制、文件头 MIME sniffing

## 生产环境必须调整

```bash
APP_ENV=production
JWT_SECRET=至少32位随机字符串
DB_PASSWORD=强密码
CORS_ALLOW_ORIGINS=https://你的前端域名
CORS_ALLOW_CREDENTIALS=true
SECURITY_HSTS_ENABLED=true
TRUSTED_PROXIES=你的反向代理IP
DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD=true
```

## 文件上传

当前项目已有文件大小、后缀限制和 MIME sniffing，并通过 `upload.storage_type` 抽象存储后端。本地模式会把文件写入 `upload.local_path`，用 `upload.public_base_url` 生成下载 URL；`s3`/`minio` 模式已通过 MinIO SDK 接入 `Store()`、`Open()` 和 `Delete()`，上传响应仍只返回受控 object key 与公共 URL。JPEG/PNG/GIF 上传会记录 `image_width` 和 `image_height` 元数据，WebP 等暂不解析尺寸。

生产落地时建议继续补：

- 文件内容签名校验
- 对公网下载地址做鉴权或短期签名
- 上传目录隔离到对象存储或专用卷

## 权限要求

- API 侧是最终权限边界，前端按钮权限只做体验层隐藏。
- 新增接口时必须同步新增权限码和授权 SQL。
- `super_admin` 可以旁路权限校验；其他角色必须具备具体权限。
- 数据权限默认按角色编码解析：`super_admin/admin` 全量、`dept_admin` 本部门及子部门、普通用户仅本人。
- 带归属字段的业务表应保留 `creator_id` 和 `department_id`，并在 DAO 中应用数据范围过滤。

## 自动化测试覆盖

- 快速 Go 单测覆盖数据权限 fallback、错误响应真实 HTTP 状态码、JWT blacklist/revoke、在线用户强制下线。
- Redis 相关测试使用进程内 miniredis，不依赖外部 Redis/MySQL。

## 观测和审计

- 监控：`/api/v1/metrics`
- 就绪：`/api/v1/health/ready`
- 日志：`server/logs/app.log`
- 操作审计：系统内置操作日志表
