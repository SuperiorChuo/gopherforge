---
description: 按架构、技术栈、业务范围与运维成本选择 GopherForge 或同类项目。
---

# 选型与同类项目

GopherForge v0.8.0 候选交付完整业务源码，不再是无业务模块的空底座：16 个 Go API 服务、AI worker、React 管理台，以及独立的媒体栈和客户端。

适合 Go + React 团队，需要多租户、RBAC、审计和监控，同时希望复用 AI、CRM、通信、支付、工单等业务实现。多服务运维、外部供应商配置、升级演练和生产容量仍需自行负责。

如果只需要单体原型、团队限定 Vue/MySQL，或期望无需供应商账户即可真实支付、发短信和打电话，应重新评估匹配度。

## 核对入口

比较当前版本的架构、许可证、维护状态、迁移策略和所需业务，而非使用过时功能表：

- [gin-vue-admin](https://github.com/flipped-aurora/gin-vue-admin)
- [go-admin](https://github.com/go-admin-team/go-admin)
- [RuoYi-Cloud](https://gitee.com/y_project/RuoYi-Cloud)
- [Simple Admin](https://github.com/suyuan32/simple-admin-core)

参见[架构与服务范围](/guide/architecture)、[部署](/reference/deployment)和[仓库选型说明](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/comparison.md)。[在线 Demo](https://superiorchuo.github.io/gopherforge/)只使用假数据，不是生产验收环境。
