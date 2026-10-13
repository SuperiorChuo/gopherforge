# 完整产品范围与运行边界

v0.8.0 候选在基础管理平台之上公开完整业务产品。正式发行状态以 [Releases](https://github.com/SuperiorChuo/gopherforge/releases) 为准。本仓不包含单体产品。

## API 服务

以下 16 个服务位于 `microservices/services/`，共享 Go module，按域独立运行。

| 服务 | 主要职责 |
|---|---|
| auth | 登录注册、JWT、OAuth、TOTP、网关验签 |
| identity | 用户、角色、权限、部门、租户与套餐 |
| system | 菜单、字典、设置、通知、代码生成 |
| audit | 日志查询与 NATS 登录事件消费 |
| file | 上传与存储 |
| monitor | 健康、数据库/Redis/任务监控；共享 goose 迁移 |
| bpm | 审批定义、实例、任务与回调 |
| ai | AI 对话、知识库、Agent、任务及工位；独立 worker |
| crm | 客户、线索、分配与跟进 |
| im | 会话、消息、访客接入与词库 |
| pay | 支付配置、订单与回调 |
| mp | 公众号配置、内容与消息 |
| notify | 通知渠道、模板与投递 |
| ticket | 工单、事件与参与者 |
| cc | 坐席、话单、计费、报表与语音配置 |
| visibility | 业务可视化与看板 |

`shared`、`internal` 和 `api` 是代码/契约目录，不是额外服务。

## 其他交付物

- React 管理台：`microservices/web/`。
- [Hermes 管理客户端](../clients/hermes-manager/README.md)：单独安装/启动。
- [FreeSWITCH 媒体子系统](../freeswitch-cc/README.md)：独立 Compose 与数据库，不与主应用媒体端口混用。
- 可观测：`platform/deploy/`，按 Compose profile 启用。

## 必须明确的边界

- 主应用 `make compose-up` 启动数据栈、应用服务、worker、网关与前端；媒体与客户端另行启动。
- AI、公众号、短信和电话中继依赖用户配置的供应商、账户或硬件。源码交付不等于这些链路已开通。
- Pay 包含订单、回调和退款流程，但当前仅提供 mock 渠道；微信/支付宝适配器尚未实现，配置商户凭据不能直接开通收款。`PAY_DEMO_MODE` 在生产默认关闭；显式开启会允许模拟支付，只用于隔离测试/演示，不能当作真实收款验收。
- Demo 仅为前端假数据，不调用真实业务 API；不能用于验证鉴权、数据库、worker 或外部投递。
- RBAC 菜单按权限下发；新增服务必须同时配置网关、权限、菜单及前端路由。
- 迁移真源为 `microservices/services/monitor/migrations/`；保留公开历史 SQL，追加 forward-only 升级，详见 [迁移约定](development/MIGRATIONS.md)。
- 产品构建、静态检查与真实运行验收分别记录，见 [发行验证](release-validation.md)。
