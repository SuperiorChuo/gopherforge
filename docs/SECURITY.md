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
- Redis 相关测试使用进程内 miniredis，不依赖外部 Redis/PostgreSQL。

## 生产环境密钥管理

### 读取约定（`shared/pkg/envsecret`）

敏感配置**优先**读 Docker Swarm secrets 文件，再回退环境变量，避免密钥出现在 `docker inspect` / 进程环境列表：

| 环境变量 | 尝试的 secret 文件名（`/run/secrets/` 下） |
|----------|------------------------------------------|
| `JWT_SECRET` | `jwt_secret` → `jwt-secret` → `go-admin-kit-jwt-secret` |
| `DB_PASSWORD` | `db_password` → `db-password` → `go-admin-kit-db-password` |
| `REDIS_PASSWORD` | `redis_password` → … → `go-admin-kit-redis-password` |
| `CONSUL_HTTP_TOKEN` | `consul_http_token` → … → `go-admin-kit-consul-http-token` |
| `GITHUB_CLIENT_SECRET` / `WECHAT_CLIENT_SECRET` | 同规则 |

**已接线服务（底座）**：

| 服务 | 敏感项（文件优先于 env） |
|------|--------------------------|
| `auth` | JWT / DB / Redis / OAuth client secret |
| `identity` | JWT / DB / Redis / INTERNAL_TOKEN |
| `system` | JWT / DB / Redis / OAuth / SYSTEM_INTERNAL_TOKEN / EMAIL_SMTP_PASSWORD |
| `audit` | JWT / DB / Redis / OAuth / NOTIFY_INTERNAL_TOKEN |
| `file` | JWT / DB / Redis / OAuth / UPLOAD_S3_* / UPLOAD_MINIO_* 密钥 |
| `monitor` | JWT / DB / Redis / OAuth / EMAIL_SMTP_PASSWORD / NOTIFY_INTERNAL_TOKEN / ALERT_* webhook |
| `bpm` | JWT / DB / BPM_INTERNAL_TOKEN / BPM_CALLBACK_TOKEN / NOTIFY_INTERNAL_TOKEN / INTERNAL_TOKEN |

**Consul / gRPC 池**：`grpcx.NewResolver` / `Register` 自动带 `CONSUL_HTTP_TOKEN`；`ConnPool` 在设置 `TLS_CA_PATH` 时走 mTLS（`DialWithEnvTLS`）。

**已接线业务服**：

| 服务 | 额外敏感项（除 JWT/DB 外） |
|------|---------------------------|
| `ai` | Redis / OAuth / AI_API_KEY / EMBED / ENCRYPTION_KEY / Hermes 签名密钥 |
| `cc` | CC_WEBHOOK_SECRET / FS_TOKEN / CRM&Notify internal token |
| `crm` | CRM_INTERNAL_TOKEN / IDENTITY / NOTIFY / BPM tokens |
| `im` | MinIO keys / AI_API_KEY / AI&Notify internal tokens |
| `mp` | REDIS_PASSWORD |
| `notify` | NOTIFY_INTERNAL_TOKEN / SYSTEM_INTERNAL_TOKEN / SMTP password |
| `pay` | PAY_INTERNAL_TOKEN / PAY_CALLBACK_TOKEN |
| `ticket` | NOTIFY_INTERNAL_TOKEN |
| `visibility` | AI/Perplexity/Gemini API keys / notify&identity tokens |

开发仍可只用 env；生产挂载 `/run/secrets` 后自动优先生效。

### Docker Swarm Secrets（推荐）

```bash
# 创建 secret（一次性；勿把真实值写进仓库或对话）
openssl rand -base64 48 | docker secret create go-admin-kit-jwt-secret -
echo -n "$DB_PASSWORD" | docker secret create go-admin-kit-db-password -
echo -n "$REDIS_PASSWORD" | docker secret create go-admin-kit-redis-password -
echo -n "$CONSUL_HTTP_TOKEN" | docker secret create go-admin-kit-consul-http-token -

# 在 docker-stack.yml 中挂载（示例）
# services:
#   auth-service:
#     secrets:
#       - source: go-admin-kit-jwt-secret
#         target: jwt_secret
#       - source: go-admin-kit-db-password
#         target: db_password
# secrets:
#   go-admin-kit-jwt-secret:
#     external: true
```

### Consul ACL

生产环境应启用 Consul ACL 和 gossip 加密：

```hcl
# consul.hcl
acl {
  enabled = true
  default_policy = "deny"
  enable_token_persistence = true
}
encrypt = "<gossip-encryption-key>"
```

初始化后创建受限 token，注入服务为 `CONSUL_HTTP_TOKEN` 或 Swarm secret `go-admin-kit-consul-http-token`：

```bash
consul acl token create -policy-name "service-discovery" -secret "<token>"
```

### gRPC mTLS（可选，内网默认明文）

| 环境变量 | 用途 |
|----------|------|
| `TLS_CA_PATH` | CA 包；客户端 `DialWithEnvTLS` / `ConnPool` 见此则启用 TLS |
| `TLS_CERT_PATH` / `TLS_KEY_PATH` | 服务端证书；`grpcx.NewServerWithEnvTLS` |
| `TLS_SERVER_NAME` | 客户端 SNI / 校验证书 CN |

开发不设 `TLS_*` 时保持 insecure；生产零信任时同时挂载服务端证书与 CA。

### 上线前自检

```bash
# 本机/容器内（不打印密钥）
bash scripts/prod-security-check.sh
# 生产严格：APP_ENV=production 且强制 Consul ACL + mTLS 路径
APP_ENV=production bash scripts/prod-security-check.sh --strict
```

### 必须修改的默认值

| 配置项 | 开发默认值 | 生产要求 |
|--------|-----------|----------|
| `POSTGRES_PASSWORD` / `DB_PASSWORD` | `123456` | 强密码或 Swarm secret |
| `JWT_SECRET` | `local-dev-secret-change-me-32-chars` | 32+ 随机字符或 Swarm secret |
| `MINIO_ROOT_PASSWORD` | `minioadmin` | 修改或使用独立 secret |
| `REDIS_PASSWORD` | 空 | 必须设置 |
| `GF_AUTH_ANONYMOUS_ENABLED` | `true` | 必须设为 `false` |
| Consul ACL / `CONSUL_HTTP_TOKEN` | 未启用 | 启用 `default_policy=deny` + token |
| gRPC mTLS | 明文 | 生产建议 `TLS_*` 齐备 |

## 观测和审计

- 监控：`/api/v1/metrics`
- 就绪：`/api/v1/health/ready`
- 日志：`server/logs/app.log`
- 操作审计：系统内置操作日志表

## 密钥轮换 Runbook（P0）

目标：JWT / DB / Redis / Consul token 可轮换且不把密钥写进 git。

### 原则

1. **只读代码路径**：应用经 `envsecret` 读 `/run/secrets` 或环境变量，不硬编码。
2. **轮换窗口**：双密钥并存 → 发布 → 摘旧密钥（JWT 需短于 access token TTL 的并存策略，或强制全员 re-login）。
3. **不在对话/日志打印密钥全文**。

### JWT_SECRET

1. 生成新 secret（≥32 随机）：`openssl rand -base64 48`
2. Swarm：`docker secret create go-admin-kit-jwt-secret-vN -`（新版本名）
3. 更新 stack 挂载 target 仍为 `jwt_secret`（滚动服务）
4. 滚动 `auth-service` / `identity-service` 及校验 JWT 的服务
5. 可选：强制下线（撤销 refresh / 清会话）使旧 token 失效
6. 确认后删除旧 secret 版本

### DB_PASSWORD / REDIS_PASSWORD

1. 在 PG/Redis 创建新密码或 `ALTER USER`
2. 更新 Swarm secret / `.env`（109 不进 git）
3. 滚动所有依赖服务（或重启应用栈）
4. 验证健康探针与登录
5. 废弃旧密码

### CONSUL_HTTP_TOKEN

1. `consul acl token create` 发新 token
2. secret `go-admin-kit-consul-http-token` 更新并滚动服务
3. 吊销旧 token

### gRPC mTLS 证书

1. 开发：`bash scripts/gen-grpc-mtls-dev-certs.sh`
2. 生产：内部 CA 签发，挂载 `TLS_CERT_PATH` / `TLS_KEY_PATH` / `TLS_CA_PATH`
3. 客户端与服务端**同时**滚动；`ConnPool` 在失败时 Invalidate 重拨
4. 强制明文拒绝：`GRPC_TLS_REQUIRED=1`（证书不全则进程退出）

### 检查清单

```bash
bash scripts/prod-security-check.sh --strict   # 含 mTLS/ACL 要求时
bash scripts/slo-probe.sh                      # 轮换后延迟回归
```

## 边缘免费证书（控制台）

- 路径：系统管理 → **边缘证书**（`/system/edge-certs`）
- 协议：Let's Encrypt **HTTP-01**（可选 Staging）
- 公开回调：`GET /.well-known/acme-challenge/:token` → system-service（无鉴权）
- system-service 是唯一 ACME owner；Traefik 内建 certresolver 必须关闭，HTTP-01 token 持久化并带 TTL
- 私钥与 ACME 账户密钥以 AES-256-GCM envelope 加密，密钥由 Swarm secret 注入；列表/任务/证书链接口均不返回私钥
- 私钥导出使用独立 `system:edge-cert:export` 权限，要求平台管理员重新输入密码和 TOTP；2 分钟 proof 绑定用户、会话与证书且只能使用一次
- 导出响应是 `no-store` 附件，同步审计落库/outbox 失败时拒绝返回私钥；旧 `/:id/download` 接口固定返回 410
- 签发、续期、部署、探测均为持久化异步任务；“已签发”不等于“线上正在使用”，只有 TLS 指纹探测一致才显示已生效
- `external` 模式（如 `admin.chouai.cc.cd` 的 Caddy）只做线上 TLS 探测，不接管对方证书或自动续期
- **不是** 服务间 gRPC mTLS（见 `scripts/gen-grpc-mtls-dev-certs.sh`）
