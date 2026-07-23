# Changelog

本项目提交信息为全中文 Conventional 风格；版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。
0.x 期间 API 与表结构可能变化。

## [Unreleased]

### 新增

- **Compose 双栈拆分**（同步自主项目）：有状态服务（PG/Redis/NATS/MinIO）独立为
  `docker-compose.infra.yml`（project `go-admin-kit-infra`），应用栈任意 down/up/rebuild
  不再触碰数据；两栈经外部网络 `go-admin-kit-net` 互通。新增 `make infra-up/infra-down`，
  `make compose-up` 自动先起数据栈；migrate job 改有界重试（跨栈无法 depends_on PG 健康）。
  升级注意：首次切换需先创建共享网络（`make compose-up` 已内置），volume 名不变、数据零迁移。
- **file-service 对象存储直链**（同步自主项目）：`/uploads` 在 `UPLOAD_STORAGE_TYPE=minio/s3`
  下由 file-service 动态回源（对象存储优先、本地磁盘兜底存量），URL 形态不变、桶无需公开——
  多副本/多节点部署的前置能力，默认 `local` 行为不变。
- **Kubernetes 部署指南**（`docs/deploy-k8s.md`）：Compose→K8s 映射表、Service 命名沿用
  容器名实现环境变量零改动、migrate Job、Deployment/probes 模板、Traefik
  IngressRoute+ForwardAuth 平移、k3s 起步与迁移实操顺序。

- **OAuth2 授权服务端**（同步自主项目）：应用管理 + `authorization_code`（公开客户端强制
  PKCE S256）/`refresh_token`（旋转）/`client_credentials` 三种授权模式 +
  `/oauth2/{authorize,token,introspect,userinfo,revoke}` 协议端点 + 授权确认页。令牌以
  SHA-256 哈希入库、`redirect_uri` 精确匹配、`client_secret` 一次性回显；协议端点走 RFC
  裸 JSON、授权与管理面走仓内封装。落在 auth-service，迁移 `000024`，权限码
  `system:oauth2-*`，设计见 `docs/design/oauth2-server.md`。

### 修复

- **在线 Demo**：仪表盘因假数据返回形状不符崩进错误边界（`/notices/active` 应返回裸数组而非
  `{list}` 包装；`menus/tree`、`departments/tree` 一并修正）；GitHub Pages 部署改为按路由
  预渲染 `index.html`，深链接直接返回 200，`404.html` 仅兜未知路径

### 新增

- **同步自主项目：IP 归属地 + 代码生成器树表/主子表 + 租户套餐 + 审批流 M1**：
  IP 归属地（cbd253f）——`shared/pkg/iploc` 离线库（ip2region.xdb 运行时下载、
  不进 git），登录日志与在线用户两处接入解析归属地，`scripts/download-ip2region.sh` 配套；
  代码生成器树表/主子表（3d74003）——在单表基础上新增「树表 / 主子表」两种生成模式，
  单表逐字节回归测试保持通过；
  租户套餐（0bb7fd4）——套餐＝权限包，租户绑定套餐后租户内角色分配权限必须 ⊆ 套餐，
  越界分配拦截；`tenant_packages` 表与 `tenants.package_id`（迁移 000022），菜单 29、
  权限点 `system:tenant-package:*`、web 路由 / 侧栏 / 面包屑补齐；
  审批流 M1（31d8942）——新增 **bpm-service**（流程定义版本化、实例单游标推进、
  会签 / 或签、行锁防并发、空候选人三兜底、终态 HTTP 回调、AutoMigrate 自管五表），
  审批中心前端（仿钉钉纵向卡片流设计器、流程定义、待办中心、我发起的、可复用时间线组件），
  菜单 35-38（审批中心分组）、权限点 `bpm:definition:*`（迁移 000023）、compose 新增
  bpm-service 与网关规则；四模块均接入在线 Demo 假数据。
  下游适配：bpm 走 Bearer JWT 自校验（不挂 ForwardAuth，与其它业务路由一致），
  notify 中性化（`NOTIFY_API_BASE` 默认空、未配则静默跳过），剥除全部 CRM / 合同接入与
  业务词引用（发起 / 反查 / 预置类型改为通用示例 `demo_expense`）
- **同步自主项目：短信管理 + 错误码管理 + 岗位管理**：
  短信管理（system）——渠道 / 模板 / 发送日志三 Tab，发送器可插拔（debug 联调直通 /
  阿里云 / 腾讯云，均无 SDK 依赖），密钥读时脱敏、更新占位保留，权限点
  `system:sms-*`（迁移 000019/000020）；
  错误码管理（system）——错误码 → 对外文案在线改，30s TTL 热生效，字典 / 公告两处
  接入示例，权限点 `system:errcode:*`（迁移 000018）；
  岗位管理（identity）——`sys_posts` / `sys_user_posts` 表（迁移 000021），岗位 CRUD
  （code 租户内唯一、有用户关联拒删），用户建改可带 `post_ids`、列表 / 详情带岗位摘要，
  权限点 `system:post:*`；网关按新路径分发（sms / error-codes → system，posts → identity），
  三页面均接入在线 Demo 假数据
- **代码生成器**（同步自上游完整版）：系统管理 → 代码生成，选表配字段一键生成
  CRUD 前后端起步包（Go model/store/handlers/routes + React 列表页 + axios api + 菜单 SQL），
  支持分文件预览与 zip 下载；权限点 `system:codegen:list|generate`（迁移 000017）

### 文档

- **审批流引擎设计方案**：`docs/design/bpm-approval-flow.md` 随引擎同步自上游
  （置顶脚手架适配说明：本仓只含引擎本体，文中 CRM 场景为上游叙事参考、不在本仓；
  notify 未配置时站内信静默跳过）；服务清单（PRODUCT_LINES / README 中英）补
  bpm-service 与「8 服务」口径；部署指南补 IP 归属地离线库（ip2region.xdb）下载步骤

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
