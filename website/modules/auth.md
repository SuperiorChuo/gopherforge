# 认证与安全

认证由 **auth 服务**承担，覆盖从登录到网关验签的完整链路。

## 能力清单

- **账号密码登录**：图形验证码防爆破，Redis 限流（失败锁定阈值/窗口/时长可在控制台「系统设置 → 安全策略」热调）。
- **JWT 双令牌**：Access + Refresh，支持轮转与吊销（黑名单进 Redis）；前端 Axios 拦截器无感刷新。
- **并发刷新保护**：同一页面复用刷新请求；多个标签页通过 Web Locks 或本地租约协调，服务端再用 Redis 原子消费旧 refresh token，避免重复轮转和误登出。
- **TOTP 两步验证**：可选开启，兼容 Google Authenticator 等标准客户端。
- **OAuth 三方登录**：GitHub 等 provider 可配。
- **在线用户**：Redis 会话登记，控制台可查看并强制下线。
- **登录事件**：经 NATS JetStream 投递 audit 服务落登录日志（持久消费不丢）。

## ForwardAuth：鉴权只做一次

受保护路由在 Traefik 层挂 ForwardAuth 中间件，由 auth 服务统一验签后把身份注入请求头：

| 头 | 含义 |
|----|------|
| `X-Auth-User-ID` | 用户 id |
| `X-Auth-Username` | 用户名 |
| `X-Auth-Tenant-ID` | 当前租户 |
| `X-Auth-Platform-Admin` | 平台管理员标记 |

业务服务**只信任这些头**，不各自解析 JWT——鉴权逻辑集中一处，新服务零成本接入。

## 密码与安全策略

bcrypt 存储、历史密码复用限制、最长有效期强制改密（`must_change_password`）——阈值全部在 `system_settings` 的 `security.policy` 键下热配置。

## OAuth2 授权服务端 + OIDC

不只是「用 GitHub 登录」的客户端——脚手架本身就是一个 **OAuth2 授权服务器**，第三方应用可以「用本平台账号登录」：

- **授权模式**：`authorization_code`（+ PKCE，公开客户端强制）与 `client_credentials`（服务间调用）。
- **协议端点**：`/oauth2/authorize`（授权同意页）、`/oauth2/token`、`/oauth2/introspect`、`/oauth2/revoke`、`/oauth2/userinfo`。
- **OIDC**：`openid` scope 签发 **RS256 `id_token`**，配 `/oauth2/.well-known/openid-configuration` 发现文档与 `/oauth2/jwks` 公钥端点——第三方用现成 OIDC 客户端库即可对接 SSO，靠 JWKS 验签、无需共享密钥。签名用独立 RSA-2048 密钥（自动生成，`system_settings` 持久化多副本共享，`kid` 稳定），与控制台自身的 HS256 完全隔离。
- **控制台管理**：「系统管理 → OAuth2 应用」维护 client（回调地址 scheme 白名单）、查看与吊销已签发令牌。
- **安全加固**（经对抗性评审）：OIDC 私钥禁止经通用 settings API 读出；`introspect` 只能内省调用方自己签发的 token；refresh 旋转防并发双花，检测到已吊销 refresh 被复用即吊销整个令牌族（OAuth Security BCP）。

## 刷新失败时的边界

浏览器会优先采用其他标签页已经发布的新 token；只有在没有可用 token、刷新租约超时，或服务端确认 refresh token 已失效时才跳转登录页。服务端的 refresh 轮转仍是一次性消费语义，调用方不要重试同一个旧 refresh token。

## 安全评审

历经多轮对抗性安全评审（供应链、时序 oracle、令牌族攻击、redirect 劫持等面），修复均带回归单测；发现问题请走 [SECURITY.md](https://github.com/SuperiorChuo/gopherforge/blob/main/SECURITY.md) 私下披露。
