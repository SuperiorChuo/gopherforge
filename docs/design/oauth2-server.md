# OAuth2 授权服务端设计

> 状态：**M1 已落地** · 应用管理 + authorization_code / refresh_token / client_credentials + 授权确认页
> 范围：`microservices/services/auth`（协议端点与数据表内聚于认证域）
> 原则：授权端点在任何跳转前先校验 `client_id` + `redirect_uri`；令牌只存哈希；`redirect_uri` 精确匹配
> 对标：yudao 开放平台（应用-授权-令牌）

相关：[`../EXPANSION_PLAN.md`](../EXPANSION_PLAN.md) Phase 5（开放平台）· [`multi-tenant.md`](multi-tenant.md)

---

## 1. 术语与范围

| 术语 | 说明 |
|------|------|
| 授权服务器 | 本系统。签发/校验令牌、承载授权确认页 |
| 客户端（client / 应用） | 接入方，持 `client_id`（+机密客户端持 `client_secret`） |
| 资源所有者 | 登录本系统的普通用户，在确认页对 scope 授权 |
| 机密客户端 | 服务端应用，`client_type=1`，用 `client_secret` 认证 |
| 公开客户端 | SPA/移动端，`client_type=2`，**强制 PKCE**，无可用密钥 |

本设计是**授权服务端**（让第三方接入本系统账号）。与既有的三方登录**客户端侧**（GitHub/微信登录进本系统，`service/auth/oauth_github.go`）方向相反，互不影响。

## 2. 目标与非目标

### 2.1 M1 目标

1. 应用（客户端）注册管理：CRUD + 密钥一次性回显 + 重置 + 启停，租户内隔离
2. 三种 grant：`authorization_code`（public 强制 PKCE S256）、`refresh_token`（旋转）、`client_credentials`
3. 协议端点：`/oauth2/authorize`、`/token`、`/introspect`、`/revoke`、`/userinfo`
4. 授权确认页（沿用登录页玻璃风格），支持自动授权 / 已授权记忆跳过确认
5. 令牌管理视图：列出已签发 access token、按需吊销

### 2.2 非目标（M1 不做）

- `implicit` / `password` grant（已废弃，不实现）
- 完整 OIDC `id_token`（`userinfo` 已够资源服务器取身份；JWT 形态 access token 留 M2）
- 动态客户端注册（RFC 7591）
- per-client 精细限流（复用全局 `DynamicRateLimit`，M2 再细化）

## 3. 数据模型（迁移 `000026`，4 表全挂 `tenant_id`）

| 表 | 关键字段 | 说明 |
|----|----------|------|
| `oauth2_clients` | `client_id`(全局唯一)、`client_secret_hash`(bcrypt)、`client_type`、`redirect_uris`/`scopes`/`grant_types`(JSONB)、`access_token_ttl`/`refresh_token_ttl`、`auto_approve`、`status` | 注册应用 |
| `oauth2_access_tokens` | `token_hash`(SHA-256)、`client_id`、`user_id`(可空)、`scopes`、`grant_type`、`refresh_token_id`、`expires_at`、`revoked_at` | 不透明访问令牌 |
| `oauth2_refresh_tokens` | 同上（无 `refresh_token_id`） | 不透明刷新令牌 |
| `oauth2_approvals` | `user_id`+`client_id` 唯一、`scopes`(同意并集)、`expires_at`(180 天) | 用户对应用的授权记忆 |

**为何存哈希不存明文**：令牌是不透明随机串（`crypto/rand` 32 字节 base64url），DB 只存 `SHA-256(token)`。拖库无法还原可用令牌；管理视图展示的是元数据（client/user/scope/过期），按 id 吊销，不需要明文。

**为何 `client_id` 全局唯一**：令牌端点无租户上下文（第三方直连），只能按 `client_id` 全局定位 client，再以 `client.tenant_id` 构造下游租户 ctx。管理面查询严格按调用者租户过滤。

## 4. 链路

### 4.1 授权码流程（含 PKCE）

```
用户浏览器                     前端授权页(/oauth/authorize)      auth-service
   │  第三方应用跳转 authorize     │                                  │
   │──────────────────────────────>│  GET /api/v1/oauth2/authorize    │
   │                               │─────────────────────────────────>│ 校验 client→redirect_uri→scope→PKCE
   │                               │<── 200 consent view ─────────────│ （失败：渲染错误卡，不跳转）
   │   点「授权」                   │  POST /api/v1/oauth2/authorize   │
   │                               │─────────────────────────────────>│ 重跑校验→写 approvals→code 入 Redis(10min)
   │<── 302 redirect_uri?code&state ──（前端整页跳转 redirect_url）───│
   │  第三方后端 code 换令牌         │                                  │
   │──────────────────────────────────────────────────────────────── >│ POST /oauth2/token 校验 client+code绑定+PKCE
   │<── access_token + refresh_token (RFC 裸 JSON) ────────────────────│ 旋转签发
```

### 4.2 安全红线

- **先校验后跳转**：`ValidateAuthorizeRequest` 校验链任一步失败都返回错误由前端渲染，绝不用未校验输入拼跳转（open-redirect 防护）
- **`redirect_uri` 精确匹配**：不做前缀匹配（前缀有 code 窃取面）；多环境注册多条
- **code 强绑定**：`code` 绑定 `client_id`+`redirect_uri`+`user`+PKCE challenge，token 端点逐项复核；一次性（Redis GetDel）
- **PKCE S256-only**：拒绝 `plain`；public client 强制
- **refresh 旋转**：换新令牌时吊销旧 refresh + 其 access，旧 refresh 复用即 `invalid_grant`
- **密钥脱敏**：`client_secret` 仅创建/重置时明文回显一次，入库 bcrypt；模型 `json:"-"`
- **管理面审计**：写 `audit_logs`（`AuditLogService.Record`，与 console-routes 一致），快照不含密钥

## 5. 端点与响应格式（双轨，刻意设计）

| 端点 | 认证 | 响应 |
|------|------|------|
| `GET/POST /oauth2/authorize` | 控制台 JWT（用户登录态） | **仓内封装** `{code,message,data}` |
| `POST /oauth2/token` | client 认证（Basic 优先，回退 form） | **RFC 6749 裸 JSON** |
| `POST /oauth2/introspect` (RFC 7662) | client 认证 | RFC 裸 JSON |
| `POST /oauth2/revoke` (RFC 7009) | client 认证 | 恒 200 |
| `GET /oauth2/userinfo` | 不透明 access token（Bearer） | RFC 裸 JSON |
| `/oauth2/clients*`、`/oauth2/tokens*`、`/oauth2/catalog` | JWT + `PermissionMiddleware("system:oauth2-*")` | 仓内封装 |

**双轨是刻意的，勿"统一"**：token/introspect/revoke/userinfo 面向第三方，必须裸 RFC 格式（客户端 SDK 按标准解析）；authorize + 管理面只给自家前端消费，走仓内封装以兼容 axios 拦截器。

## 6. 权限与前端

- 权限码：`system:oauth2-client:{list,create,update,delete,reset-secret}`、`system:oauth2-token:{list,delete}`（迁移种子 + 超管补挂）
- 菜单：系统管理 → 「OAuth2 应用」（`/system/oauth2`）
- 管理页 `web/src/pages/system/oauth2/`：Tab1 应用管理（创建后一次性密钥弹窗）、Tab2 令牌管理（列表+吊销）
- 授权确认页 `web/src/pages/oauth/authorize/`：**放 MainLayout 之外**（第三方用户不见管理台骨架），未登录跳 `/login?redirect=` 回跳
- 登录回跳：`login/index.tsx` 读 `?redirect=`，正则 `^\/(?!\/)` 拒绝外站跳转

## 7. 网关

`docker-compose.yml` auth router rule 追加 `PathPrefix(/api/v1/oauth2/)`。前端 `/oauth/authorize` 是 SPA 路由（nginx `try_files` 兜底），网关零改动。

## 8. 验证

`microservices/tests/api-smoke.mjs` 覆盖全流程：建应用（记一次性密钥）→ authorize 视图 → approve 取 code → token(PKCE) → userinfo → introspect(active) → refresh 旋转 → 旧 refresh 拒绝 → client_credentials → revoke → introspect(inactive) → 删应用。

## 9. 与 yudao 的差异

| 维度 | 本实现 | yudao |
|------|--------|-------|
| 令牌存储 | SHA-256 哈希 | 明文/Redis |
| redirect_uri | 精确匹配 | 精确匹配 |
| PKCE | S256-only，public 强制 | 支持 |
| 部署足迹 | 内聚 auth-service，无新进程 | 独立模块 |
