# 认证与安全

认证由 **auth 服务**承担，覆盖从登录到网关验签的完整链路。

## 能力清单

- **账号密码登录**：图形验证码防爆破，Redis 限流（失败锁定阈值/窗口/时长可在控制台「系统设置 → 安全策略」热调）。
- **JWT 双令牌**：Access + Refresh，支持轮转与吊销（黑名单进 Redis）；前端 Axios 拦截器无感刷新。
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
