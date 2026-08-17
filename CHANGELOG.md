# Changelog

本项目提交信息为全中文 Conventional 风格；版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。
0.x 期间 API 与表结构可能变化。

## [Unreleased]

### 变更

- **短信管理列表状态统一（同步自主项目）**：短信渠道、模板与发送日志统一使用公共筛选表单、查询状态、刷新动作和实体编辑弹窗，保留权限控制、测试发送与发送详情。
- **系统管理列表骨架收敛（同步自主项目）**：字典、菜单、权限与租户页面补齐公共列表骨架；部门、错误码、文件、公告、岗位、租户套餐与 Webhook 操作列在窄屏统一收进更多菜单。
- **authz 收敛（同步自主项目）**：新增 `shared/pkg/authz` 与 `shared/pkg/tenantctx`，system/audit/monitor/file/identity 删除本地 authz 拷贝并统一装配数据范围插件。
- **生效公告查询上限（同步自主项目）**：前台生效公告列表加 50 条硬上限，避免异常写入把首页一次拉爆。
- **无界 Find 机检（同步自主项目）**：新增 `scripts/scan-unbounded-find.sh`，存量只列、暂存区相对 HEAD 新增才阻断；pre-commit 已接入。
- **健康检查收敛（同步自主项目）**：六服务删除本地 `health.go`，统一使用 `shared/pkg/health`；monitor 的 `/metrics` 仍留本服务。
- **tracing 收敛（同步自主项目）**：六服务删除本地 `observability` 包，统一使用 `shared/pkg/observability.InitTracer`。

## [0.6.0] - 2026-08-13

### 新增

- **边缘证书生命周期 V2**：控制台支持持久化异步签发、续期、部署与指纹探测；Compose 网关由 system-service 唯一接管 HTTP-01，并通过 Traefik file provider 热加载证书。
- **微服务发现与契约**：引入 Consul 服务发现、gRPC 契约与生成代码、连接池和重试策略；BPM 发起支持幂等键，服务具备优雅关停与健康状态聚合能力。
- **韧性与事务一致性底座**：新增 `graceful`、`idempotency`、`outbox`、`health`、`auditevents`、`identityclient` 与 `natsx` 共享能力，事务审计可自动写入 Outbox。
- **前端底座完善**：权限管理支持树形/列表双视图，补充 Jaeger 入口与主题 cssVar，并将认证页、布局、监控和表格样式拆分为按页面加载的 CSS 模块。

### 变更

- **服务边界收敛**：BPM 不再直查用户表，公共 model、DAO、JWT、middleware、连接池与 API 依赖统一下沉到 `shared/pkg`。
- **数据与运行时升级**：基础设施基线升级至 PostgreSQL 18、Redis 8、NATS 2.12、Traefik 3.7、Prometheus 3.13、Grafana 13、Loki 3.7 与 Jaeger v2；前端升级 Vite 8、Axios 1.19 和 Ant Design 6.5。
- **迁移发布门禁**：增加 schema 拆分、fresh DB 迁移演练、Compose 编排合同和 gRPC buf lint/breaking 检查。

### 修复

- **监控服务优雅关停与前端国际化**：恢复可注入上下文的 HTTP 排空路径并补齐公开词典，解除 CI 门禁阻断。
- **边缘证书菜单初始化**：菜单改由迁移单点写入并增加固定菜单 ID 唯一性测试，避免新环境因重复主键启动失败。
- **公开迁移边界**：迁移仅保留公开 BPM/Monitor 基础设施 schema，阻止业务域结构进入脚手架。
- **后台任务与事务审计**：修复任务心跳表兼容和 Outbox 回调污染 GORM Statement 导致的事务写入失败。
- **构建与发布链**：修复 monitor 镜像上下文、前端上传目录、国际化门禁和文档站构建依赖问题。

### 安全

- **边缘证书私钥保护**：ACME 账户密钥与证书私钥采用 AES-256-GCM envelope 加密；导出要求独立权限、密码+TOTP 二次验证、单次短时 proof 与同步审计，并禁止网关头伪造平台管理员身份。
- **生产密钥与服务通信**：增加容器密钥文件读取、密钥强度检查、Consul ACL 与可强制的 gRPC mTLS 策略，附带生产安全自检和 SLO 探针。
- **供应链加固**：升级 Vite、esbuild、PostCSS、nanoid 与 `golang.org/x/text`；Dependabot、Code Scanning 和 Secret Scanning 开放告警清零。

### 文档

- **版本与部署口径**：文档、示例与部署页统一指向 `v0.6.0`，并补齐迁移演练、生产安全与边缘证书部署说明。

## [0.5.0] - 2026-08-09

### 新增

- **功能完善六批同步**（2026-08-06 同步自主项目）：
  - **shared 收敛**：`mailer` / `pagination` / `tenant` / `notifyclient` 四包下沉
    `shared/pkg`，monitor/system/audit/file/identity/bpm 等服务的本地副本删除改引
    （pagination/tenant 各 5 份逐字节副本归一；notifyclient 统一走 internalhttp
    连接池 + 5xx/429 退避）。
  - **忘记密码邮件链路**：`POST /password/forgot`（防枚举 + IP 限频）与
    `/password/reset`（token sha256 存库、30 分钟过期、原子消费防重放），邮件配置
    复用 system_settings `notification.email`；迁移 `000060`；前端 login 页「忘记
    密码」入口 + forgot/reset 两页。
  - **审计收尾**：`bpm_process_definition` 接入事务审计（管理面五个写方法补 ctx +
    事务化）；`GET /logs/audit/export` CSV 导出（UTF-8 BOM，上限 5 万行）。
  - **网关加固**：entrypoint 级全局限流（average=100/s burst=200，env 可调）；
    `websecure:443` TLS-ready（无证书自签兜底，ACME_EMAIL 配了即启用 LE）；
    Dashboard 保持 loopback-only。
  - **文件存储配额**：`tenant_packages.storage_quota_mb`（迁移 `000061`，0=不限），
    上传前校验租户已用存储 + 本次大小；`UPLOAD_MAX_SIZE` env 化。
  - **杂项修复**：identity 重置/创建密码 binding min=8 对齐服务端强度校验；
    auth failed-login 事件按请求租户归因；`/monitor/mysql` 页 slow_queries 接真值
    （pg_stat_activity >10s 活跃查询计数）。

- **登录安全批**（2026-08-07 同步自主项目）：
  - **设备指纹采集**：`login_logs.device_id`（首次登录/新设备识别），前端 `X-Device-ID`
    持久化到 localStorage（跨标签页/重启保持一致）。
  - **新设备/新 IP 登录提醒**：audit 服务检测新设备→notify 站内信推送给用户。
  - **IP 级登录失败护盾**：同一 IP 连续失败超阈值触发临时封禁（auth 服务）。
  - **风控修复**：DeviceID JSON tag 缺失致新设备检测失效、IPv6 护盾、OAuth
    device_id 补全、隐私模式告警刷新修复、告警失败标注已提醒、落库失败吞告警修复、
    console 护盾失效修复、数据范围泄漏修复。
  - **IP 归因安全收窄**：forwardedHeaders + TRUSTED_PROXIES 补全（gwbridge
    172.22 收窄到 /32）。

- **基础设施加固**（2026-08-08/09 同步自主项目）：
  - **登录事件 Redis Streams**：auth→audit 登录事件从 Redis pub/sub 迁移到
    Streams，拿回 at-least-once 语义（消费确认 + 持久化）。
  - **观测栈安全**：各服务 metrics/pprof 端口绑 127.0.0.1（不暴露到容器外）；
    nginx frontend 容器降权为非 root 运行。
  - **日志↔追踪联动**：`X-Request-ID` 贯穿 gin context → logger → OpenTelemetry
    span attribute，观测栈资源限制（mem_limit）。
  - **构建收敛**：Dockerfile 参数化统一 + bpm service 追踪补全 `InitTracerFromEnv`。

- **前端依赖更新**（2026-08-07/08 同步自主项目）：
  - react-router-dom 6.30.4→7.18.2（消除 2 moderate 安全告警）
  - npm update 12 个 patch/minor 依赖

- **前端 i18n 国际化**（2026-08-05/06 同步自主项目）：
  - 全站 133 文件硬编码中文迁入 `t()` 层（react-i18next + key=中文原文
    message-as-key，零 zh.json，en.json 渐进词典回落中文）
  - 语言切换机制：antd/dayjs locale、头部按钮、localStorage 持久
  - 门禁 `lint:i18n` 扫描展示位中文不在词典即漏网；extract/check 工具链
  - micro en.json 子集多次重生成（同步主仓新 key，裁剪避业务域禁词）

- **文档**（2026-08-07 同步自主项目）：
  - `docs/CODE_MAP.md`：全项目代码图谱——按模块索引服务/路由/页面/关键文件/依赖关系

## [0.4.0] - 2026-08-04

### 新增

- **监控历史趋势**（2026-08-04 同步自主项目）：monitor 每 60s 用告警同源采集器落
  `monitor_metric_samples`（保留 7 天），新增 `/monitor/metrics/trends`（1h/24h/7d
  降采样）；服务器/数据库/Redis 监控页新增自绘 SVG 趋势图（无第三方图表库），
  服务器页聚合告警摘要与最近事件成监控总览。
- **告警引擎多通道通知 + 静默期**（2026-08-04 同步自主项目）：通知从单一邮件重构为
  `MultiChannelNotifier`——邮件 / 站内信（复用 notify 内部告警端点，micro 不配 env
  自动跳过）/ 企业微信机器人，规则级 `notify_channels` 选择；规则新增
  `silence_until` 静默期（到期前不评估不通知）。
- **告警指标扩展**（同进程）：新增 PostgreSQL 慢查询、Redis 逐出/过期（每秒增量）、
  磁盘 IO 与网络吞吐；迁移 `000055` 同步 metric 白名单 CHECK 约束。

### 修复

- **邀请注册用户名查重按邀请租户**（2026-08-04 同步自主项目）：注册查重此前硬编码
  默认租户，邀请指向其他租户时同名用户漏判（撞租户级唯一索引报 500）或误拒；
  现按邀请所属租户查重，补跨租户用例。
- **邀请注册错误文案回归英文源文**（2026-08-04 同步自主项目）：邀请 token 无效/已用
  时的注册错误信息改为英文，跟随脚手架英文源文约定（此前为中文，会触发源文语言
  守卫测试失败）；同批把 CI 长红的 Auth 测试跟进邀请制语义、补交监控趋势 API 的
  OpenAPI 与前端类型生成物，CI 恢复全绿。
- **告警工作台状态与窄屏布局**（2026-08-04 同步自主项目）：告警概览区分首次加载、
  请求中断和旧数据保留，接口失败不再显示四项真实 `0`；规则与事件列表补刷新失败
  提示和重试入口，并在窄屏将关键指标合并展示、行操作收进更多菜单。
- **服务器监控状态反馈**（2026-08-04 同步自主项目）：主机指标、服务健康与告警概览
  区分首次加载、请求中断、旧数据保留和局部采集不可用；缺失指标显示 `--`，不再
  伪装成绿色健康的 `0% / 0 B`，并补齐窄屏标题、统计与事件布局。
- **异步可靠性收敛**（2026-08-02 同步自主项目）：BPM 终态回调在目标暂未配置时
  保留任务并按退避重试，HTTP 投递跟随 worker 生命周期取消；文件对象或缩略图删除
  失败时保留数据库记录，避免产生失去管理入口的孤儿对象；认证事件发布增加固定并发
  上限，阻塞的 NATS transport 不再导致 goroutine 无界增长。

### 新增

- **服务间短信发送契约**（2026-08-01 同步自主项目）：system-service 新增仅供
  容器内服务调用的租户级短信入口，复用既有渠道、模板、参数校验和发送日志；
  `SYSTEM_INTERNAL_TOKEN` 留空时端点 fail closed，生产环境拒绝弱占位值。

### 安全

- **核心账号与权限表事务审计**（2026-08-01 同步自主项目）：`users`、`roles`、
  `departments` 与 `menus` 四张表本体的单行增删改接入通用 GORM 审计，记录
  actor、tenant 与前后快照，审计写入失败时与业务变更一并回滚；密码、TOTP 与
  恢复码等敏感字段递归脱敏，缺失归因、租户、事务或无法证明写入范围有界时
  fail closed。关联表与 Raw SQL 仍不在本批审计覆盖范围。

- **审计日志租户归属与隔离修复**（2026-08-01 同步自主项目）：auth、identity、
  system、file、monitor 五个基础服务补齐审计模型的 `tenant_id` 映射；创建记录时
  使用请求租户，列表、汇总和筛选统一限制在当前租户，避免审计事件误归默认租户 1
  或管理端读取到其他租户的审计数据。

- **PostCSS 构建链高危公告补修**（2026-07-30 同步自主项目）：Vite 间接依赖
  `postcss` 从 8.5.17 升至 8.5.25，修复 GHSA-r28c-9q8g-f849；CI 与定时安全
  扫描改为审计包含 devDependencies 的完整依赖树，避免漏掉构建供应链漏洞。
- **密钥回显、操作日志与网关身份头加固**（2026-07-30 同步自主项目）：系统设置
  对嵌套敏感字段统一掩码，保存回传掩码时恢复旧值，避免密钥回显或把占位符写入
  数据库；5 个基础服务的操作日志改为共享递归脱敏，非法/截断 JSON 按整段隐藏；
  ForwardAuth 注入去重排序的权限头（super_admin 压缩为 `*`），Traefik 在鉴权前
  清除客户端伪造的全部 `X-Auth-*` 头，并限制权限头最大 16KB。
- **前端路由依赖高危公告处置**（2026-07-30 同步自主项目）：管理端锁定
  `react-router-dom 6.30.4`，避开当前 `7.x` RSC 动作处理的高危 CSRF 公告；本项目
  仅使用 BrowserRouter SPA 的稳定 Library API，构建与 lint 通过，生产依赖审计
  high/critical 清零；CI 与定时安全扫描统一按 high 阻断，并删除旧公告的 npm/Trivy
  临时放行规则。

- **多租户隔离与无鉴权下载修复**（2026-07-29 同步自主项目）：部门树缓存键补租户
  维度（原全局单键会跨租户串数据）；角色删除后按受影响用户失效角色码缓存；在线
  用户按租户建索引且索引键补 TTL（volatile-lru 下原永不淘汰）；file `/uploads`
  直链收口——对象 key 补 crypto/rand 随机段、访问改 HMAC-SHA256 签名 URL（新增
  `UPLOAD_URL_SIGN_SECRET` / `UPLOAD_URL_SIGN_TTL_SECONDS`，留空回退 JWT_SECRET）；
  tenant 兜底插件补注册 system/audit/file。**行为变更**：`/uploads` 裸直链一律
  404，须经 API 下发的签名 URL 访问。

### 性能

- **会话鉴权缓存与请求路径收敛**（2026-07-29 同步自主项目）：console_session 改
  Redis 缓存 + last_seen 写节流（命中后每请求 0 次 DB 查询，原 3 查 1 写）；
  admin/super_admin 数据范围短路复用角色码缓存；monitor 操作日志批量写；identity
  用户导出 keyset 分页流式 xlsx；system 字典去 N+1 加缓存；users/sms_logs 补列表
  复合索引（迁移 000031）；前端轮询接后台标签页停表与可见性暂停。
- **构建提速**（2026-07-29 同步自主项目）：7 个服务 Dockerfile 依赖层只拷
  go.mod/go.sum + BuildKit cache mount，增量构建分钟级降到秒级；`ARG MAIN_SRC`
  双源支持本机交叉编译产物直接打包（缺省行为不变）。

### 变更

- **shared 下沉**（2026-07-29 同步自主项目）：monitor 删 5 个本地包改引
  shared/pkg 并补回漂移丢失的错误码；bpm 删 metrics/jobbeat 副本改引 shared
  （构建上下文随之拓宽为 services/）；服务间内部 HTTP 统一走
  shared/pkg/internalhttp（连接池复用 + 5xx/429 退避重试）。

- **系统管理菜单拆组**（2026-07-29 同步自主项目）：原 19 个子菜单全堆在「系统管理」
  下，拆为 系统管理（组织+权限）/ 消息中心 / 日志审计 / 系统工具 四个一级分组。
  子菜单 path/组件/ID/权限码全部不变，仅调整挂载与排序；迁移 000030 幂等重挂已有
  库，seed 与迁移用同一组显式分组 ID（134-136）互相去重，OAuth2 菜单按 path 定位挂入
  系统工具。前端侧栏展开与面包屑分组由「路径首段推断」改为菜单树驱动的祖先链查找，
  分组 key 不再要求是子路径前缀。

### 文档

- **交互式系统架构图**（2026-07-28 同步自主项目）：架构总览中英文页面新增
  Archify 演示，覆盖 React Admin、Traefik、7 个 Go 基础服务、状态层、NATS
  JetStream 与可观测组件，并提供主题切换、导览、链路追踪和全屏查看。

### 样式

- **主题语义颜色变量层**（2026-07-28 同步自主项目）：新增明暗两套
  `--c-*`、文字层级与容器背景变量；BPM 时间线、统计卡和流程设计器不再写死
  Ant Design 旧默认色，亮色主题下自动使用更深的高对比度色值。登录页同步收紧
  表单玻璃底色、输入框边界和辅助文字对比度，桌面与移动端信息层级更稳定。

### 修复

- **BPM 终态回调不再因重启丢失**（2026-08-01 同步自主项目）：审批终态与
  回调任务在同一数据库事务提交，后台 worker 通过 `SKIP LOCKED` 支持多副本安全
  抢占，并具备租约恢复、指数退避和失败耗尽留痕；移除原进程内 goroutine 长时间
  等待，服务滚动更新后未完成的回调仍可继续投递。

- **CI 全链路门禁恢复**（2026-07-30 同步自主项目）：重新生成 monitor 的 OpenAPI
  与 TypeScript 契约，补齐任务日志和调度目标接口；修正 monitor 静态检查问题；菜单
  拆组迁移改用未被 identity sequence 占用的分组 ID 并重置序列，避免全新数据库在
  迁移阶段因主键冲突退出。

- **数据范围 SQL 的 MySQL 反引号**（2026-07-27 同步自主项目）：`ApplyOwnerScope`
  生成的子查询写作 ``user_id IN (SELECT `id` FROM `users` ...)``，而全部服务都是
  postgres 驱动——反引号在 PG 是标识符语法错误，数据范围为部门/部门树/自定义档的
  用户查操作日志列表会直接失败。改为裸列名，各服务 sql/plugin/cache/dao 测试期望同步。

### 性能

- **一批后端性能同步**（2026-07-27 同步自主项目）：
  - 连接池收敛：6 个配置服务默认 `MaxOpenConns 100→10 / MaxIdleConns 10→5` 并新增
    `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` 环境变量覆盖（`.env.example` 已记）；
    bpm 裸 `gorm.Open` 补 `connpool.go` 护栏（database/sql 默认 MaxOpen 无上限）
  - GORM 全线开 `PrepareStmt`（语句按连接预编译复用）；`SkipDefaultTransaction`
    刻意不开——取消关联写入隐式事务属语义变更
  - 角色码缓存补齐到 system/audit/file/monitor/auth（此前只有 identity 有，
    这些服务的 super_admin 前置判定仍每请求查库）；`role_cache_test.go` 守卫随行
  - 登录趋势由按天循环 2×COUNT 改整窗一条 GROUP BY；角色/菜单分配权限改批量
    INSERT；operation_logs 列表 `Omit` 掉两个 4KB text 列；操作日志中间件
    GET/HEAD 跳过请求体读取、脱敏挪到响应写出后
  - Redis 增加 `maxmemory 512mb` + `volatile-lru` 护栏（JWT 黑名单等安全键无 TTL
    不受驱逐；AOF 保留）

- **一批前端性能同步**（2026-07-27 同步自主项目）：
  - nginx 开启 gzip（此前产物全量原文传输，JS 体积为压缩后 3-4 倍）
  - `@ant-design/icons` 并成单个长缓存 chunk（此前拆成数十个 1-3KB 碎片，
    进管理台一次触发几十个请求）
  - BPM 流程设计器改 `lazy()` 懒加载，定义列表路由包不再背整个设计器
  - 租户套餐权限 Tree 传 `height` 启用虚拟滚动；角色授权弹窗权限全集模块级缓存
  - 新增 `useVisibilityInterval`：后台标签页暂停轮询（monitor/server 接入）

### 新增

- **监控告警规则闭环**（2026-07-31 同步自主项目）：新增真实 CPU、内存、磁盘、
  PostgreSQL 与 Redis 指标的周期评估，支持 `pending/firing/resolved/error` 状态、
  告警事件留痕、SMTP 通知结果、手动评估 API 及管理页面；迁移 `000032` 同步权限
  与菜单，邮件未启用或无收件人时明确记录为 `skipped`。

- **代码生成器补齐到主项目当前形态**（同步自主项目，补此前遗漏的整代升级）。
  此前本仓的生成器停留在早期版本，缺失分层模板、schema 内省、多对多、字典绑定、
  变更计划与仓库写入。本次整体对齐（约 4200 行新代码 + 1600 行测试）：
  - **产物改为与本仓一致的分层结构**（model / dao / service / api + 前端 api 与
    页面 + 权限迁移共 7 个文件），并会改写四处接线（服务路由、前端路由表、
    侧栏布局、菜单种子）。
  - **新增「写入仓库」交付方式**（`POST /codegen/write`，权限 `system:codegen:write`，
    迁移 `000028`）——**默认全关**：需 `CODEGEN_WRITE_ENABLED=true` 与
    `CODEGEN_REPO_ROOT` 同时显式配置，且调用者为平台管理员。路径侧绝对化 +
    符号链接解析 + 越界一律拒绝，写入走暂存 + 备份 + 加锁。仅建议开发环境启用。
  - **预览/下载改为对着仓库快照算变更计划**，镜像内固化了一份只含接入源文件的
    快照，故开箱即用、无需挂载宿主仓库。为此 system 服务的构建上下文提到
    `microservices` 根（Dockerfile 要拷 web 与迁移目录下的接入文件），并补
    `.dockerignore` 控制上下文体积。
  - 新增多对多关系与字典绑定；文档站代码生成器页（中英）按新形态重写——旧文档
    的「不写入仓库」「产物为 `server/<module>/` 四文件」「不支持字典」等描述均已失实。

- **OAuth2 服务端 B② 收尾：per-client 限流与 JWT 形态 access token**（同步自主项目）。
  客户端两个新字段都有默认值，存量行为不变。
  - **JWT access token（RFC 9068）**：客户端可选 `access_token_format=jwt`，签发
    `typ=at+jwt` 的 RS256 自包含令牌，复用 OIDC 同一把 RSA 密钥与 JWKS——资源
    服务器只需一个密钥源。**两种形态都照旧在库里留行**，introspect 与吊销走
    完全同一条路径；JWT 只是多给了离线验签的选项。代价明说：离线验签方在
    过期前看不到吊销，故 opaque 仍是默认、jwt 客户端应配短 TTL。签名密钥不可用
    时拒绝签发而非静默退回 opaque。
  - **per-client 限流**：token 与 introspect 按 `client_id` 计每分钟配额
    （`token_rate_per_minute`，0=服务端默认 120/min），Redis 滑动窗口，超限返
    429 + `slow_down` + `Retry-After`。落点在客户端认证之后（认证前按请求里的
    client_id 计数等于给了任何人耗尽他人配额的手段，认证前泛洪由服务级 IP 限流
    兜底）；**revoke 刻意不限流**——吊销是安全止损动作；Redis 故障放行。
  - 管理页可视配置两项，迁移 `000027` 两列均带默认值。

### 变更

- **TypeScript 6 → 7**（前端工具链，原生化大版本）。适配面只有
  `tsconfig.app.json`：移除已删除的 `baseUrl`（TS5102）与配套的
  `ignoreDeprecations: "6.0"`，`paths` 的 `@/*` 补前导 `./`（TS5090 起
  非相对路径不再允许）。别名解析行为不变，vite 侧 alias 独立配置不受影响；
  二开者若自定义过 `paths`，升级时需同样补前导 `./`。

### 文档

- **文档站新增「API 参考」页**（中英，参考区首位入口）：调用总则（网关入口 /
  Bearer 认证 / 响应信封 / 分页）+ 六个模块端点速查的汇总跳转 + OpenAPI 3.1
  契约在线浏览（Scalar 交互式页面直读仓库 main 的 `openapi.json`，与代码
  永远同步；Scalar 版本钉死不追 latest）。如实标注契约当前覆盖
  monitor + common，其余服务以模块页端点表为准。

- **README 顶部新增微信交流群入口**：放回群二维码（新增
  `docs/screenshots/wechat-group.png`），欢迎扫码加群交流使用与开发。

- **文档站与演示站补齐近期能力说明**：`observability` / `audit` 模块（中英）新增
  monitor 内置告警规则引擎（阈值规则、状态机、SMTP 邮件通知、规则与事件管理页）
  与核心权限表事务审计 + 租户隔离说明；在线 Demo 补「告警管理」菜单与规则/事件
  假数据（覆盖 ok/pending/firing/error 状态）；README 功能矩阵扩充告警与短信条目。

- **文档站新增「前端开发」分组**（中英各 7 页）：架构总览、请求层与 API 封装
  （信封解包 / 401 单飞无感刷新 / silent）、路由与权限（lazyLoad / usePermission /
  route-permissions）、页面开发规范（列表页三件套 / 公共组件）、状态管理、主题与
  样式、演示模式（假数据 adapter 与补数据步骤）；架构与二次开发指南加交叉链接。

## [0.3.0] - 2026-07-26

### 新增

- **官方镜像双架构**（同步自主项目）：`release.yml` 补 QEMU，`platforms` 加
  `linux/arm64`——arm64 云机与 Apple Silicon 自本版起可直接拉官方镜像部署，
  不再被迫本地构建（代价是发版构建时长约翻倍）。
- **审计日志保留策略**（同步自主项目）：`AUDIT_LOG_RETENTION_DAYS` 按天数周期
  自动清理操作/登录日志（默认 0=关闭，绝不隐式删数据；`audit_logs` 是合规取证
  面，刻意不在自动清理范围）。后台任务没有租户上下文，直接走既有清理链路会
  回落默认租户漏删其余租户——DAO 为此新增显式命名的跨租户方法（保留策略专用，
  租户闸门本身不动）；每轮经 jobbeat 上报任务中心心跳（`audit.log_retention`），
  失败标 error。

### 安全

- **grpc 升至 1.82.1**（同步自主项目）：消除 GHSA-hrxh-6v49-42gf（HIGH，xDS
  RBAC 与 HTTP/2 漏洞），6 个模块的 indirect 依赖统一升级。govulncheck 判定
  漏洞符号不可达，但 trivy 门禁按设计拦截"有修复版的 HIGH"，升级后 PR 的
  CI 全部解堵。
- **bpm 依赖升级：x/crypto 0.54、x/net 0.56**（同步自主项目）：消除 2026-07-25
  当天新披露的 ssh/agent/knownhosts（CVE-2026-39828 等，含未授权命令执行）与
  x/net html/idna 系列 HIGH——bpm 是仓内唯一低于修复线的模块，其余本就 ≥0.52/0.55。
- **bpm 弱凭据名单对齐 auth/monitor 口径**（同步自主项目）：补齐 aws/access-key
  系 8 个条目与 `minioadmin`/`secret-key`/`secretkey`，并新增 `dev-` 前缀兜底
  ——生产环境下 dev-xxx 形态的 token 一律视为占位符，经 sanitize 归零后由
  使用点 fail closed（此前只逐个枚举，换个名字就漏）。

### 文档

- **文档站充实**：RBAC / 多租户 / 审批流 / 代码生成器四个模块页扩写为操作走查级
  ——接口与权限码速查表、数据范围六档、任务动作约束、Excel 导入导出、套餐约束
  语义、类型映射等，事实均自代码提取；顺带修正两处与实现不符的旧描述（代码
  生成器并无「组件类型」可配，控件按列类型自动推断；产物为 model/store/handlers/
  routes，无独立 service 层）。新增「审计日志」模块页、「常见问题 FAQ」与
  「版本升级」（含 0.1.0→0.2.0 注意事项）三个页面。中英两条线同步更新。
- **部署文档跟上 v0.2.0**：生产部署页（website 与 `docs/deployment.md` 同源）新增
  「方式 A · 拉官方 ghcr 镜像」（`IMAGE_PREFIX`/`IMAGE_TAG` 用法、amd64-only 说明），
  升级/回滚章节改为镜像版本切换优先；中英首页与部署页版本口径更新为 `v0.2.0`。

## [0.2.0] - 2026-07-26

### 新增（工程化）

- **发布流程与供应链扫描**（同步自主项目）：新增 `release.yml`（tag `v*` 触发 →
  SemVer 与 CHANGELOG 门禁 → Go/前端/契约三条门禁 → 8 个镜像推 ghcr.io →
  建 GitHub Release；镜像双 tag `vX.Y.Z` + `sha-<7位>`，`latest` 仅非预发布时更新，
  服务清单由 `docker compose config --format json` 现算而非硬编码）、
  `security-scan.yml`（govulncheck 按模块跑，第三方符号级可达才阻断、stdlib 与
  非符号级只告警；trivy 只对 HIGH/CRITICAL 且上游已有修复版本阻断）、
  `dependabot.yml`（gomod / npm / github-actions / docker，weekly）。
  trivy 刻意不用 `aquasecurity/trivy-action`——2026-03 该 action 的 76 个 tag 曾被
  强推为窃取 runner secret 的恶意版本，改跑固定版本官方镜像。
- **镜像名版本化**：compose 里每个可构建服务显式声明
  `${IMAGE_PREFIX:-go-admin-kit}-<服务名>:${IMAGE_TAG:-latest}`，两变量都不设时
  与旧推导名完全一致（本机开发 / CI bake / ops 脚本零改动）。
- **CI 补 `shared-module` 与 `bpm-service` 两个门禁**：`shared` 是 8 个服务共用的
  公共库（metrics/logger/response/jobbeat/mask/iploc），改坏了全线崩却一直没有门禁。
- **CI 的 Go 版本抽成顶层 `env.GO_VERSION`**，升版本只改一处；`setup-buildx-action`
  与 `upload-artifact` 同步升到当前主版本（后者 v4 已是 Node 20 弃用线）。

### 安全

- **Go 工具链升 1.26.5**（同步自主项目）：`go.work` 与 9 个模块加 `toolchain go1.26.5`。
  刻意不抬高 `go` 语言版本指令——那是使用者的最低要求，抬高会强迫下游全部升级，
  而消除 stdlib 漏洞只需用新工具链编译。实测 govulncheck 在 `services/shared` 上
  由 3 条 stdlib 符号级可达降为 0 条（`crypto/tls` 需 1.26.5，1.26.4 不够）。
- **bpm 生产配置强校验**：`APP_ENV=production` 下 `JWT_SECRET`（<32 位或占位符）与
  `DB_PASSWORD`（空/弱值）拒绝启动；其余内部 token 不阻断启动，改为在使用点
  fail closed 并打 WARNING——`BPM_INTERNAL_TOKEN` 缺失或为占位符时 `/internal`
  端点一律 503，绝不拿公开的开发占位符当真凭据校验。占位符判定用 `dev-` 前缀兜底。
- 清掉 compose 中 alertmanager 的 `NOTIFY_INTERNAL_TOKEN` 开发占位符默认值，
  让"没配"就是没配，不再静默注入一个人尽皆知的 token。

### 修复

- 认证与监控契约：加固刷新协调并完善 OpenAPI 生成链路（即 `v0.2.0-rc.2` 的候选期修复）。

### 文档

- 文档站新增「更新日志」页（中英各一个导航入口），经 `@include` 直读仓库根
  `CHANGELOG.md`——与 `release.yml` 抽取的 Release notes 同一来源，发版后
  GitHub Release、文档站、仓库三处日志天然一致，无需维护第二份。
- `CONTRIBUTING.md` 技术栈订正 Go + Vue → **React**；启动流程改为 `make compose-up`
  （数据栈在独立 infra compose，只起应用栈会失败）；移除仓库内不存在的 `monolith/` 引用。
- `SECURITY.md`：`MYSQL_ROOT_PASSWORD` → `DB_PASSWORD`、MySQL → PostgreSQL；
  新增「已知边界」显式声明**未实现 CSRF 防护**（纯 Bearer + SPA，改用 cookie 承载
  token 需自行补齐）；补生产自校验的分服务准确口径。
- 新增 `CODE_OF_CONDUCT.md`（Contributor Covenant v2.1 中文版）。
- README 中英两版：服务数订正为 7 个 Go 服务进程，补行为准则与收录范围链接。

0.2.0 正式版相对 0.1.0 的完整范围 = 本段落 + 下方 [0.2.0-rc.1] 段落。

## [0.2.0-rc.1] - 2026-07-24

### 新增（运维管理面）

- **运维管理面：任务中心 + 服务健康总览 + 告警闭环**（同步自主项目）：
  ① 任务中心——新表 `ops_job_heartbeats`（迁移 `000026`）+ `shared/pkg/jobbeat`
  上报包（无侵入嵌入现有循环）+ monitor `/monitor/jobs/heartbeats` 聚合读取
  （`stale = 超 2 倍间隔`）+ 定时任务页新增「服务任务心跳」卡片，本进程 cron
  与进程外分布式任务同页可见。
  ② 服务健康总览——monitor `/monitor/services` 并发探测底座服务 `ready`
  (3s 超时，`MONITOR_HEALTH_EXTRA` 可加网外目标)，服务器监控页新增「微服务
  健康」卡 10s 自刷。
  ③ 告警闭环——`node_exporter` 主机指标 + Prometheus 告警规则（服务 down /
  磁盘 <15% / 内存 >92% / 5xx 陡增，`for 3m` 吸收滚动更新）+ Alertmanager
  分组去抖/恢复通知/宕机抑制错误率告警（`webhook` 通道，token 启动时 sed
  注入模板不进 git）；Demo 侧补三端点假数据含超期示例，方便无后端也能预览。

- **审批引擎加签 / 委派（M3+ 收尾）**（同步自主项目）：待办详情增加「加签」
  与「委派」两个动作——加签在当前节点动态插入前置或后置审批人（不改流程
  定义），委派把待办转派给他人代办（原受理人可撤回、可见性覆盖发起人/受理人/
  委派方三视角）。BPM 侧持久化 `bpm_task_delegations`（迁移 `000027`），
  identity 侧无变更；前端待办中心加两个操作条目、列表加委派标记。

### 安全

- **登录态并发刷新加固**：前端在同一页面内复用刷新请求，优先通过 Web Locks 非阻塞获取跨标签页锁；
  不支持 Web Locks 时使用 IndexedDB 原子租约，只有 IndexedDB 不可用才降级到 `localStorage` 最佳努力租约，
  避免并发消费同一个 refresh token。服务端用 Redis 原子消费令牌 ID，只有一个并发请求能成功轮转，
  另一个明确收到已吊销错误；刷新请求增加 15 秒超时。补充跨请求回归测试与 Playwright 场景测试。
- **OAuth2/OIDC 安全评审修复**（同步自主项目）：三方对抗性审查后的一批加固——① OIDC 签名
  私钥（RSA）禁止经通用 `system-settings` API 读出/改删（新增保护名单，list 读取遮蔽为
  `{protected:true}`），杜绝拿到私钥后伪造任意用户 id_token 的"通杀"面；② `introspect`
  限定只能内省调用方自己签发的 token（`token.client_id != caller` 返回 `{active:false}`），
  堵住跨 client/跨租户元数据泄漏；③ refresh 旋转检查 `RowsAffected` 防并发双花，检测到已吊销
  refresh 被复用即吊销整个令牌族（OAuth Security BCP）；④ `redirect_uri` 注册加 scheme 白名单
  （拒 `javascript:`/`data:`）；⑤ `invalid_client` 错误文案统一 + dummy bcrypt 抹平 client
  存在性时序 oracle。附 14 条 service 层安全单测。

### 新增

- **OAuth2 服务端 OIDC（M2）**（同步自主项目）：在 OAuth2 服务端上补齐 OpenID Connect——
  `openid` scope 签发 RS256 `id_token` + `/.well-known/openid-configuration` 发现文档 +
  `/oauth2/jwks` 公钥端点，第三方用现成 OIDC 客户端库即可对接 SSO。`id_token` 用独立
  RSA-2048 密钥（不碰 console 的 HS256），私钥自动生成后持久化在 `system_settings`
  （多副本共享、`kid` 稳定），第三方靠 JWKS 验签无需共享密钥；`nonce` 透传绑定；走路径式
  issuer（`${OIDC_ISSUER_URL}/api/v1/oauth2`）故网关零改动。设计见 `docs/design/oauth2-server.md` §10。

- **全服务 /metrics 可观测**（同步自主项目）：`shared/pkg/metrics` 零依赖 Prometheus
  指标包（HTTP 计数/错误/延迟直方图 + Go runtime + DB 连接池），auth/identity/system/
  audit/file 一行接入，bpm（独立构建上下文）持 `internal/metrics` 同源副本；Prometheus
  新增 `go-admin-kit-services` 抓取任务（service 标签聚合），Grafana 预置「服务概览」
  看板（QPS/错误率/P95/在途/goroutine/内存/连接池）。`/metrics` 仅容器网络内可达，
  网关不路由；`METRICS_ENABLED=false` 可关。

- **部门主管选人**（同步自主项目，补齐 BPM M2 缺腿）：部门管理支持指定主管用户
  （`departments.leader_user_id`，迁移 `000025`），审批流「部门主管」规则据此解析
  审批人——此前引擎已读该列但 identity 侧未建列，全新部署走 dept_leader 规则会
  解析报错，本次补齐迁移 + 模型 + 接口 + 前端选人表单。

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
  bpm-service 与「7 个 Go 服务 + shared 公共库」口径；部署指南补 IP 归属地离线库（ip2region.xdb）下载步骤

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
[0.2.0-rc.1]: https://github.com/SuperiorChuo/gopherforge/releases/tag/v0.2.0-rc.1
