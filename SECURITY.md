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

- 全部服务：`JWT_SECRET` 少于 32 位、或仍是默认/占位符值。
- `monitor` 与 `bpm`：另加 `DB_PASSWORD` 为空/默认/弱值；`monitor` 还校验 `REDIS_PASSWORD` 与对象存储凭据、CORS 危险组合。

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
