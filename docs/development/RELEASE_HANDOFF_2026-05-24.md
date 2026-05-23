# 2026-05-24 发布交接说明

本文档用于交接本轮企业级成熟度收口后的提交、PR、上线和回滚信息。代码侧已经完成验证；生产发布仍需按目标环境替换配置、执行迁移演练和 smoke。

## 当前状态

- 后端、前端、CI、文档、迁移和生成产物均有变更，当前工作区改动规模较大。
- `server/docs/openapi.json`、`tdesign-vue-go/src/api/generated/schema.d.ts`、`tdesign-vue-go/src/api/generated/client.ts`、`tdesign-vue-go/src/api/generated/client.typecheck.ts` 属于 OpenAPI 合约生成链路，应与 OpenAPI 源码同组提交。
- `server/docs/go_admin_kit.sql` 是数据库 schema 快照，应与迁移脚本同组提交。
- `server/configs/config.yaml` 像本地运行配置，提交前需再次确认是否有个人环境值；`.env.example` 和 `server/.env.example` 可提交，但生产必须替换示例密码和密钥。
- `docs/superpowers/plans/*.md` 是过程计划文档，是否提交取决于仓库是否希望保留实现计划。
- `git diff --check` 当前退出码为 0，仅有 CRLF 将被 Git 触碰时转换为 LF 的提示。

## 建议提交分组

1. **安全与认证能力增强**：登录安全、JWT、OAuth、TOTP、密码策略、限流、脱敏和默认管理员策略。
   - `server/internal/api/auth/`
   - `server/internal/service/auth/`
   - `server/internal/dao/auth/`
   - `server/internal/pkg/jwt/`
   - `server/internal/middleware/auth.go`
   - `server/internal/middleware/login_limit.go`
   - `server/internal/middleware/rate_limit.go`
   - `server/internal/pkg/response/error_codes.go`

2. **Context wrapper 清理与上下文传播改造**：清理 legacy Context wrapper，收紧架构测试，改造 DAO/Service 调用链。
   - `server/internal/architecture/`
   - `server/internal/dao/**`
   - `server/internal/service/**`
   - `server/internal/pkg/authz/`
   - `server/internal/pkg/cache/`
   - `server/internal/pkg/captcha/`
   - `server/internal/pkg/ipinfo/`

3. **运行时配置、系统设置与邮件通知**：系统设置、runtime config invalidation、SMTP 邮件通知和前端设置页。
   - `server/internal/pkg/runtimeconfig/`
   - `server/internal/pkg/mailer/`
   - `server/internal/api/system/setting.go`
   - `server/internal/dao/system/setting.go`
   - `server/internal/service/system/setting.go`
   - `server/internal/service/system/email_notification.go`
   - `tdesign-vue-go/src/api/system/setting.ts`
   - `tdesign-vue-go/src/pages/system/setting/`

4. **通知中心与 WebSocket 通知链路**：WebSocket ticket、通知广播、Redis bridge、前端实时通知。
   - `server/internal/api/system/notification_ws.go`
   - `server/internal/service/system/notification.go`
   - `server/internal/api/system/notice.go`
   - `server/internal/service/system/notice.go`
   - `tdesign-vue-go/src/store/modules/notification.ts`
   - `tdesign-vue-go/src/layouts/components/Notice.vue`

5. **对象存储、文件上传、hash 秒传与缩略图**：S3/MinIO、本地存储、图片尺寸、缩略图、前端 hash 秒传。
   - `server/internal/pkg/upload/`
   - `server/internal/api/system/file.go`
   - `server/internal/service/system/file.go`
   - `server/internal/dao/system/file.go`
   - `server/internal/model/file.go`
   - `tdesign-vue-go/src/api/system/file.ts`
   - `tdesign-vue-go/src/pages/system/file/`

6. **数据库迁移与迁移演练工具**：迁移脚本、migration rehearsal、schema 快照。
   - `server/migrations/000003_*.sql` 到 `server/migrations/000010_*.sql`
   - `server/internal/migrate/`
   - `server/cmd/migration-rehearsal/`
   - `server/docs/go_admin_kit.sql`
   - `docs/development/MIGRATIONS.md`

7. **OpenAPI 合约与生成类型**：后端 OpenAPI、生成脚本、前端 generated types/client、契约测试。
   - `server/internal/openapi/`
   - `server/docs/openapi.json`
   - `scripts/generate-openapi-types.mjs`
   - `tdesign-vue-go/src/api/generated/`
   - `tests/openapi-contract.test.mjs`

8. **前端页面适配与 smoke**：登录、个人中心、监控任务、路由 fallback、通用样式和 E2E。
   - `tdesign-vue-go/src/pages/login/components/`
   - `tdesign-vue-go/src/pages/profile/index.vue`
   - `tdesign-vue-go/src/pages/monitor/job/index.vue`
   - `tdesign-vue-go/src/router/modules/legacy-fallbacks.ts`
   - `tdesign-vue-go/src/style/`
   - `tdesign-vue-go/e2e/frontend-smoke.spec.ts`

9. **CI、文档、本地开发说明与发布清单**：CI 漂移检查、PR 模板、README、本地 setup、ready checklist。
   - `.github/workflows/ci.yml`
   - `.github/pull_request_template.md`
   - `README.md`
   - `LOCAL_SETUP.md`
   - `docs/SECURITY.md`
   - `docs/development/*.md`
   - `server/Makefile`
   - `.env.example`
   - `server/.env.example`

## PR 描述草案

```md
## Summary

本 PR 完成一轮后台安全、运行时配置、通知、文件上传和接口契约强化：新增密码有效期/历史密码策略、TOTP 两步验证、OAuth 绑定/解绑、系统设置运行时覆盖、WebSocket 实时通知、邮件通知、文件图片元数据/缩略图和对象存储读写能力，并同步 OpenAPI、前端生成类型、API smoke 与迁移演练。

## Backend changes

- 新增系统设置接口 `/api/v1/system-settings`，支持列表、单项读写、批量写入和删除，并对 `security.policy`、`notification.email` 做运行时缓存刷新和 Redis invalidation。
- 新增 TOTP 两步验证：登录挑战、验证码校验、绑定/启用/关闭、恢复码重新生成，普通登录和 console 登录均接入。
- 强化密码策略：支持 `password_max_age_days`、`password_history_count`、密码变更时间、密码历史表和过期强制修改。
- OAuth 改为可配置启用，补齐 GitHub/Wechat provider client、state 校验、绑定/解绑、唯一约束冲突处理，并支持 TOTP 登录挑战。
- 新增 WebSocket 通知 ticket、通知广播器、Redis bridge，公告启用后可推送实时通知并按配置发送邮件。
- 文件上传补齐图片宽高、缩略图、对象存储 open/delete、预览/下载流式读取和安全 `Content-Disposition`。
- 新增 migration rehearsal 命令，迁移覆盖 `000003` 到 `000010`。
- 清理大量 legacy context wrapper，主进程启动/关闭路径改为可测试的 `run(ctx)`，补齐操作日志处理器、任务调度器、通知 bridge 等 shutdown。

## Frontend changes

- 登录页支持 TOTP 二次校验弹窗，登录 store 可处理 `requires_totp` 会话。
- 个人中心新增两步验证管理。
- 系统文件页支持前端 hash 秒传检查、图片缩略图展示、仅图片预览、下载/预览二进制契约。
- 新增系统设置页，支持站点信息、邮件通知、安全策略的读取与批量保存。
- 通知 store 从静态 mock 改为 WebSocket 实时通知，支持 ticket 获取、自动重连、持久化最近消息。
- 同步 `server/docs/openapi.json`、`tdesign-vue-go/src/api/generated/schema.d.ts` 和 typed client/typecheck。

## Tests run

- `go test ./... -count=1`
- `go vet ./...`
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --config ../.golangci.yml ./...`
- `npm run test`
- `npm run lint`
- `npm run stylelint`
- `npm run build:type`
- `npm run build`
- 真实 MinIO smoke 等价命令通过
- `npm run smoke:api`
- `git diff --check`

## Rollout notes

- 发布前先备份数据库，并在目标版本同构环境执行 migration rehearsal。
- 正式环境执行 `go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations up` 后再启动新服务。
- 检查 OAuth、密码策略、邮件通知 SMTP、对象存储 bucket/endpoint/public URL、WebSocket 反向代理 upgrade。
- 若启用邮件通知，SMTP 密码仍应通过环境变量或静态配置注入，不建议写入运行时系统设置。
- 多实例环境需确认 Redis 可用，以同步 runtime config invalidation 和通知广播。

## Risks/rollback

- 数据库迁移包含表重命名和新增唯一索引，回滚前需确认业务代码版本与表名/索引匹配，并保留备份。
- TOTP、密码过期和历史密码策略可能影响登录流程；紧急回滚可先将相关安全策略调低或禁用，再回退服务版本。
- OAuth provider 默认需显式启用，生产环境若未配置完整 client/secret/redirect 会返回 provider unavailable。
- WebSocket 通知依赖代理 upgrade、Origin/CORS 和 Redis；异常时主要影响实时通知，不应阻断核心 CRUD。
- 对象存储文件删除增加引用计数逻辑；如需回滚，应避免在新旧版本之间交叉删除同一批文件记录。
```

## 上线前人工确认

必须替换或确认以下配置：

- `APP_ENV=production`
- `JWT_SECRET` / `jwt.secret`
- `MYSQL_ROOT_PASSWORD`、`DB_PASSWORD`、`DB_USER`、`DB_NAME`、`DB_HOST`、`DB_PORT`
- `REDIS_PASSWORD`、`REDIS_DB`、`REDIS_HOST`、`REDIS_PORT`
- `MINIO_ROOT_USER`、`MINIO_ROOT_PASSWORD`、`UPLOAD_STORAGE_TYPE`、`UPLOAD_S3_*` 或 `UPLOAD_MINIO_*`
- `GRAFANA_ADMIN_USER`、`GRAFANA_ADMIN_PASSWORD`
- `GITHUB_OAUTH_ENABLED`、`GITHUB_CLIENT_ID`、`GITHUB_CLIENT_SECRET`、`GITHUB_REDIRECT_URI`
- `WECHAT_OAUTH_ENABLED`、`WECHAT_CLIENT_ID`、`WECHAT_CLIENT_SECRET`、`WECHAT_REDIRECT_URI`
- `EMAIL_NOTIFICATION_ENABLED`、`EMAIL_SMTP_HOST`、`EMAIL_SMTP_PORT`、`EMAIL_SMTP_USERNAME`、`EMAIL_SMTP_PASSWORD`、`EMAIL_SENDER`、`EMAIL_ALERT_RECEIVER` / `EMAIL_ALERT_RECEIVERS`
- `CORS_ALLOW_ORIGINS`、`CORS_ALLOW_CREDENTIALS`
- `TRUSTED_PROXIES`
- `SECURITY_HEADERS_ENABLED`、`SECURITY_HSTS_ENABLED`
- `RATE_LIMIT_*`、`LOGIN_LIMIT_*`
- `DEFAULT_ADMIN_FORCE_CHANGE_PASSWORD`
- `PASSWORD_MAX_AGE_DAYS`、`PASSWORD_HISTORY_COUNT`
- `METRICS_ENABLED`、`TRACING_ENABLED`、`TRACING_OTLP_ENDPOINT`
- `VITE_BASE_URL`、`VITE_API_URL`、`VITE_API_URL_PREFIX`、`VITE_IS_REQUEST_PROXY`

生产环境禁止保留 `123456`、`admin/admin`、`minioadmin/minioadmin`、`local-dev-secret-change-me-32-chars`、`replace-with-at-least-32-random-characters` 等默认值或占位值。

## 数据库与回滚注意事项

- 当前迁移覆盖 `000001` 到 `000010`，上线前必须在同版本数据库副本执行 migration rehearsal。
- `000008_add_oauth_binding_user_provider_unique.sql` 会删除同一 `user_id + provider` 下较旧的重复 OAuth 绑定，上线前需确认生产数据是否存在重复。
- `000004_add_totp_2fa.sql` 回滚会删除用户 TOTP 配置和恢复码。
- `000009`、`000010` 回滚会删除文件图片尺寸与缩略图字段。
- `000001` 的 Down 会删除基础表，只能视为重建或灾难场景操作。
- 生产禁止执行 `make migrate-reset`、`db-reset`、`docker compose down -v`。

## 集成验证项

- 对象存储：上传、下载/预览、删除、缩略图、公开 URL、失败补偿删除。
- WebSocket：`POST /api/v1/ws/notifications/ticket`、`GET /api/v1/ws/notifications?ticket=...`、HTTP 101、断线重连、Origin/CORS。
- OAuth/TOTP：OAuth login/callback/bind/unbind、TOTP challenge、恢复码一次性使用、服务器时间同步。
- 邮件通知：SMTP TLS/STARTTLS、发件人/收件人、公告启用后发送邮件、失败日志。
- 可观测性：`/api/v1/health/ready`、metrics、tracing、请求 ID、审计日志。

## 建议发布顺序

1. 冻结发布窗口，备份 MySQL、Redis 关键数据和对象存储 bucket。
2. 注入生产 `.env` / `config.yaml`，先执行生产配置校验。
3. 确认 MySQL、Redis、对象存储、SMTP、OAuth provider、监控/Tracing 可用。
4. 在数据库副本执行 migration rehearsal，生产执行 `migrate up`。
5. 部署后端，检查 `/api/v1/health/ready`、日志、数据库、Redis、metrics。
6. 部署前端，确认 API 地址和代理路径。
7. 配置 Nginx/LB：HTTPS、CORS、WebSocket upgrade、转发头、上传静态路径或 CDN。
8. 执行登录、权限菜单、文件、WebSocket、OAuth/TOTP、邮件通知、监控页面 smoke。
9. 修改默认管理员密码或确认强制改密策略生效，再逐步放量。

## 建议回滚顺序

1. 停止放量或从 LB 摘除新实例。
2. 回滚前端静态资源。
3. 回滚后端镜像和生产配置。
4. 数据库优先向前修复，只有确认 Down 不会丢业务数据时才逐个版本回滚。
5. 涉及 `000004`、`000008`、`000009`、`000010` 的回滚必须先确认备份和恢复方案。
6. 对发布窗口内新增对象存储 key 做孤儿对象审计或清理。
7. 恢复后重新检查 `/api/v1/health/ready`、登录、核心菜单、上传下载、WebSocket。
