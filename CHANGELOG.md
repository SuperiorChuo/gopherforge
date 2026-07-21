# Changelog

本项目提交信息为全中文 Conventional 风格；版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。
0.x 期间 API 与表结构可能变化。

## [Unreleased]

### 修复

- **在线 Demo**：仪表盘因假数据返回形状不符崩进错误边界（`/notices/active` 应返回裸数组而非
  `{list}` 包装；`menus/tree`、`departments/tree` 一并修正）；GitHub Pages 部署改为按路由
  预渲染 `index.html`，深链接直接返回 200，`404.html` 仅兜未知路径

### 新增

- **代码生成器**（同步自上游完整版）：系统管理 → 代码生成，选表配字段一键生成
  CRUD 前后端起步包（Go model/store/handlers/routes + React 列表页 + axios api + 菜单 SQL），
  支持分文件预览与 zip 下载；权限点 `system:codegen:list|generate`（迁移 000017）

### 清理

- 剔除脚手架残留的 AI/IM/CC 引用与业务迁移（`ai_*` 表迁移、AiMarkdown、imContact、
  设置页 AI/呼叫分组、仪表盘业务快捷入口）

## [0.1.0] - 2026-07-18

微服务脚手架首个版本，只含平台无关的基础设施服务。

### 基础设施

- **认证鉴权**：登录（验证码 / TOTP 两步）、JWT Access/Refresh 轮转与吊销、OAuth（GitHub / 微信）、登录限流
- **RBAC**：用户 / 角色 / 权限 / 部门 / 菜单，角色数据范围（全部 / 部门及以下 / 仅本人）
- **多租户**：共享库 + `tenant_id`，登录带租户码，租户 CRUD 与网关头透传
- **系统管理**：字典、公告、系统设置（数据库热配置，控制台改完即生效）、在线用户、文件上传（MinIO / 本地）
- **审计**：登录日志 / 操作日志 / 审计日志，NATS 登录事件持久消费
- **监控**：服务器 / PostgreSQL / Redis / 定时任务监控，健康检查，Prometheus metrics，可选 OTel + Jaeger 链路
- **微服务架构**：Traefik 网关 + ForwardAuth 统一验签（业务服务只信任网关注入的 `X-Auth-*` 头），
  auth / identity / system / audit / file / monitor + shared 按域拆分，goose 版本化迁移，
  OpenAPI 3.1 契约（CI 漂移校验）
- **前端**：React 19 + Ant Design 6，深空暗色 / 白蓝亮色双主题，玻璃拟态视觉

### 工程化

- GitHub Actions：各服务独立 test+vet、前端 lint+build+audit、OpenAPI 契约漂移校验、
  迁移彩排、compose 集成冒烟（API smoke + Playwright E2E 经网关）
- pre-commit 密钥扫描钩子（`scripts/install-git-hooks.sh` 启用）
- 运维脚本（`scripts/ops/`）：PG 每日备份、磁盘清理、日志轮转、镜像回滚，`install-ops-cron.sh` 一键装 cron
- Docker Compose 一键启动全栈；宿主机端口默认只绑 loopback

[0.1.0]: https://github.com/SuperiorChuo/gopherforge/releases/tag/v0.1.0
