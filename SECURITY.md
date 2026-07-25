# 安全策略

GopherForge（曾用名 Go Admin Kit）是后台管理脚手架，默认配置用于本地开发。生产或公网环境部署前，必须替换默认密钥、默认密码和 CORS 配置。

## 支持范围

当前仅维护 `main` 分支。请优先基于最新 `main` 复现和提交安全问题。

## 报告安全问题

请不要在公开 issue 中披露完整攻击步骤、可利用 payload、真实密钥或生产数据。

推荐方式：

1. 优先使用 GitHub Security Advisory 私密上报。
2. 如果当前仓库未开启私密上报，请创建一个不包含利用细节的 issue，说明问题类型和影响范围，并等待维护者沟通后再提供复现细节。
3. 如问题已经被公开利用，请在报告中标注“疑似已被利用”，方便优先处理。

## 生产部署最低要求

上线前至少完成这些配置：

```bash
APP_ENV=production
JWT_SECRET=至少32位随机字符串
DB_PASSWORD=强密码
REDIS_PASSWORD=强密码
CORS_ALLOW_ORIGINS=https://你的前端域名
CORS_ALLOW_CREDENTIALS=true
SECURITY_HSTS_ENABLED=true
DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD=true
```

`APP_ENV=production` 下服务启动时会自校验并**拒绝启动**（这些项没有可安全降级的行为）：

- 全部服务：`JWT_SECRET` 少于 32 位、或仍是默认/占位符值；`DB_PASSWORD` 为空、默认（`123456`）、弱值或占位符。
- 全部连接 Redis 的服务（`auth` / `audit` / `file` / `identity` / `system` / `monitor`，`bpm` 不用 Redis）：`REDIS_PASSWORD` 为空、默认、弱值或占位符。
- 读取对象存储凭据的服务（`file` / `monitor`）：校验按 `UPLOAD_STORAGE_TYPE` 条件生效——取 `s3` 或 `minio` 时校验 endpoint 形态、bucket、access key、secret key（`s3` 另需 region），取 `local`（缺省值）时不校验任何对象存储凭据。
- 发起第三方登录的 `auth`：`GITHUB_CLIENT_SECRET` / `WECHAT_CLIENT_SECRET` 为默认值、弱值或占位符。校验**按 provider 条件生效**——只在该 provider 真正就绪时才校验，也就是 `*_OAUTH_ENABLED=true` 且 client id / client secret / redirect uri 三件套齐备（这正是 OAuth 服务放行该 provider 的同一道闸）；开关关闭、或三件套缺一时一律不校验（此时 provider 运行期本就返回不可用，弱值不会流向任何远端）。其余服务只是携带同一份配置模板、并不发起 OAuth 流程，不做这项校验。
- 读取邮件通道配置的 `system` / `monitor`：`EMAIL_SMTP_PASSWORD`（`monitor` 的 yaml 路径是 `notification.email.password`）为空、默认、弱值或占位符。校验**按通道条件生效**——只在通道真会发信且真会认证时才校验，也就是 `EMAIL_NOTIFICATION_ENABLED=true`、`EMAIL_SMTP_HOST` 非空，且用户名或密码至少一项非空（此时才会发 SMTP AUTH）；通道关闭、host 留空、或匿名转发（用户名与密码都不配、不发 AUTH）时一律不校验。
- `monitor` 另外还校验 CORS 危险组合。

弱凭据的判定是**已知默认值/占位符的精确匹配**，刻意不设长度下限（真实部署里存在 9 字符的对象存储 access key）。也就是说它拦得住 `123456`、`minioadmin`、`change-me` 这类值，拦不住"短但独特"的自造弱口令——强度仍需自己把关。

**尚未覆盖**（如实记录，不要默认已被拦住）：

- **运行期热改的邮件配置不经启动校验**：`system_settings` 的 `notification.email` 行可以在控制台打开通道或改 `smtp_host`（密码不在可热改字段里，仍取自环境变量）。所以"启动时通道是关的、之后在控制台打开"这条路径会绕过上面的启动校验——在控制台开启邮件通道前请自行确认 `EMAIL_SMTP_PASSWORD` 的强度。
- **基础设施容器自身的口令不经任何服务校验**：`MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`、`GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD` 是容器自己的环境变量（缺省值都是 `minioadmin` / `admin`），没有任何 Go 服务会读它们，因此弱值不会阻止启动。
- 可选的内部鉴权 token（`BPM_INTERNAL_TOKEN` / `BPM_CALLBACK_TOKEN` / `NOTIFY_INTERNAL_TOKEN`）不做强度校验，按下一段的 fail-closed 约定处理。

（事件总线不在这份清单里：这些服务只读 `NATS_URL` 这个连接地址，没有独立的 NATS 凭据配置项，也就无所谓校不校验。）

其余可选的内部鉴权 token **不阻断启动**（少配一个可选 token 不该让整个服务起不来），而是打 `WARNING` 并在使用点 fail closed——例如 `BPM_INTERNAL_TOKEN` 缺失或仍是占位符时，bpm 的 `/internal` 端点一律返回 503，绝不拿公开的开发占位符当真凭据校验。上线后请检查启动日志里有没有 `WARNING`。

同时请确认：

- 默认管理员密码已修改。
- PostgreSQL、Redis、MinIO、Grafana 等服务不暴露弱密码。
- 上传目录或对象存储 bucket 已隔离。
- 反向代理已配置 HTTPS、HSTS 和可信代理地址。
- 生产日志不输出密码、token、secret 或用户隐私字段。

## 已知边界

这些是脚手架**当前没有实现**的部分，二次开发时请自行评估补齐，不要默认已被覆盖：

- **未实现 CSRF 防护**。前端是 SPA，令牌走 `Authorization: Bearer`（存于前端，不依赖 cookie 自动携带），因此当前形态下浏览器不会跨站自动附带凭据，CSRF 面天然收窄。但**如果你在二次开发中改用 cookie（尤其是 `SameSite=None`）承载 token 或 session，必须自行补齐 CSRF token / Origin 校验**——仓库里没有这层。
- 权限的最终边界在 API 侧；前端按钮级权限只做体验层隐藏。
- 更多默认已启用能力与建议补强项见 `docs/SECURITY.md`。

更多安全能力说明见：

- [`docs/SECURITY.md`](docs/SECURITY.md)
- 上线前请替换默认密钥与管理员密码（见 [README](README.md) 「安全提示」）
