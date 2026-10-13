# 选型与同类项目

GopherForge v0.8.0 候选为完整业务产品：16 个 Go API 服务、AI worker、React 管理台，以及独立媒体和客户端。它不再是拿来即无业务代码的空底座。

## 适合的场景

- 后端选择 Go + Gin + GORM、前端选择 React + Ant Design。
- 需要按领域拆分服务，同时复用 RBAC、多租户、审计、监控和审批底座。
- 希望参考或复用 AI、CRM、通信、支付、工单等实际业务源码，并能承担外部供应商配置和运行验收。
- 以 PostgreSQL 为主要数据库，愿意维护多服务、网络和异步任务。

## 不适合的场景

- 只需要轻量单体原型，无法承担多服务运维成本。
- 团队限定 Vue 或 MySQL，需要低成本直接适配。
- 希望无需账户、密钥或中继即可真实运行支付、短信、AI 和电话链路。
- 需要已有生产容量或服务等级保证；开源源码与本机门禁不能提供这种保证。

## 对比入口

请按当前版本核对各项目的架构、前端生态、许可证、维护状态、迁移策略和实际所需业务，避免把历史功能表当成实时结论：

- [gin-vue-admin](https://github.com/flipped-aurora/gin-vue-admin)
- [go-admin](https://github.com/go-admin-team/go-admin)
- [RuoYi-Cloud](https://gitee.com/y_project/RuoYi-Cloud)
- [Simple Admin](https://github.com/suyuan32/simple-admin-core)

[GopherForge 产品范围](PRODUCT_LINES.md) · [部署指南](deployment.md) · [在线 Demo](https://superiorchuo.github.io/gopherforge/)（仅假数据）
