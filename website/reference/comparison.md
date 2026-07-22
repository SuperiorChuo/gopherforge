# 同类项目对比

> 本页与仓库 [`docs/comparison.md`](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/comparison.md) 同源。


> 选型参考。信息基于各项目公开仓库与文档（2026-07 核对），如有出入以各项目官方为准。
> 我们尽量客观：GopherForge 不是所有场景的最优解，下面同样写清楚它不适合谁。

## 一句话定位

**GopherForge** 是「只含基础设施」的 **Go 微服务** 后台脚手架：认证、RBAC、多租户、审计、文件、监控、代码生成器，前端 React 19 + Ant Design 6，Traefik 网关统一鉴权，不带任何业务模块。

## 对比总表

| 维度 | GopherForge | gin-vue-admin | go-admin (go-admin-team) | RuoYi-Cloud（若依微服务版） | Simple Admin |
|------|------------|---------------|--------------------------|------------------------------|--------------|
| 架构形态 | **微服务**（Traefik + 7 服务） | 单体 | 单体 | 微服务（Java 生态迁移风格，Nacos/Seata） | 微服务（go-zero） |
| 后端 | Go + Gin + GORM | Go + Gin + GORM | Go + Gin + GORM | Go/Java 混合生态 | Go + go-zero |
| 前端 | **React 19 + Ant Design 6** | Vue 3 + Element Plus | Vue 3 + Element Plus | Vue 3 + Element Plus | Vue 3 + Element Plus |
| 数据库 | PostgreSQL 16 | MySQL 为主 | MySQL 为主 | MySQL 为主 | MySQL / PostgreSQL |
| 网关与鉴权 | Traefik ForwardAuth 统一验签，服务只信网关注入头 | 应用内中间件 | 应用内中间件 | 独立网关组件 | go-zero gateway |
| 多租户 | ✅ 共享库 + tenant_id，登录带租户码 | 插件/自行扩展 | ❌ | 企业版 | ✅ |
| RBAC + 数据范围 | ✅（全部/部门及以下/仅本人） | ✅ | ✅ | ✅ | ✅ |
| 审计三件套（登录/操作/审计日志） | ✅，NATS 事件持久消费 | 部分 | ✅ | ✅ | 部分 |
| 系统监控（服务器/DB/Redis/任务） | ✅ 内置页面 | 部分 | 部分 | ✅ | 部分 |
| 代码生成器 | ✅ 选表配字段出前后端 CRUD | ✅ | ✅ | ✅ | ✅ |
| OpenAPI 契约 + CI 漂移校验 | ✅ | ❌ | ❌ | ❌ | 部分 |
| 业务模块耦合 | **零**（业务=加一个服务） | 携带示例业务 | 携带示例业务 | 携带较多业务模块 | 较少 |
| 一键启动 | `docker compose up`（含全部依赖） | 需自配依赖 | 需自配依赖 | 组件多、启动重 | 脚本化 |
| 许可证 | MIT | Apache-2.0 | MIT | MIT | Apache-2.0 |

## 什么时候选 GopherForge

1. **后端 Go、前端 React** 的团队——同类主流方案前端几乎全是 Vue，这是 GopherForge 最直接的差异位。
2. **真的要微服务**：按域拆好的 7 个服务 + 网关统一鉴权是起点而不是重构目标；新业务 = 新服务 + 一个网关标签，不动底座。
3. **讨厌删业务代码**：拿到手就是干净底座，不用先花两天剥离别人的商城/CMS 示例。
4. **PostgreSQL 优先**、需要多租户 SaaS 底座、看重 OpenAPI 契约和 CI 完备度的项目。

## 什么时候不选它

- **单人小项目 / 原型**：微服务是负担不是收益，选 gin-vue-admin 这类单体更快。
- **前端团队只熟 Vue**：gin-vue-admin / Simple Admin 的 Vue 生态更顺手。
- **需要现成业务模块**（商城、CMS、工作流开箱即用）：若依系带的业务多，二开起点离成品更近。
- **重度依赖 MySQL 生态工具链** 的团队。

## 常见对比问题

**Q：和 gin-vue-admin 比？**
gin-vue-admin 是单体 + Vue，生态和教程最多，适合快速交付中小项目；GopherForge 是微服务 + React，适合预期会长大、要按域扩展的系统。两者不在同一个架构档位，按团队技术栈和项目生命周期选。

**Q：和 go-admin（go-admin-team）比？**
go-admin 同为 Gin + GORM 单体，成熟稳定、star 多；GopherForge 提供它没有的微服务拆分、多租户、Traefik 统一鉴权与 React 前端。注意别和 GoAdminGroup/go-admin（数据可视化面板框架）混淆——三个是不同的项目。

**Q：和 RuoYi-Cloud / ruoyi-vue-pro 比？**
若依系功能最全、中文资料最多，但组件重（Nacos/Seata/XXL-Job 等）、携带大量业务模块；GopherForge 走轻路线：Traefik + NATS + goose，依赖少一个量级，底座干净。

**Q：和 Simple Admin 比？**
同为 Go 微服务脚手架，Simple Admin 基于 go-zero（自带 RPC 生态），前端 Vue；GopherForge 基于 Gin（团队上手门槛更低），前端 React，网关层用通用的 Traefik 而非框架自带网关。

## 快速体验

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge/microservices
cp .env.example .env
docker compose up -d --build
# 网关 http://localhost:8000 · 前端见 README 端口一览
```

或直接打开 [在线 Demo](https://superiorchuo.github.io/gopherforge/)（纯前端假数据，任意账号可登录）。
