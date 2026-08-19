---
layout: home
pageClass: gopherforge-home

hero:
  name: GopherForge
  text: 开源 Go 微服务后台管理脚手架
  tagline: 从服务边界、统一鉴权到可观测与持续交付，把生产级工程能力锻造成开箱即用的微服务底座。
  actions:
    - theme: brand
      text: 15 分钟快速上手
      link: /guide/getting-started
    - theme: alt
      text: 在线体验 Demo
      link: https://superiorchuo.github.io/gopherforge/
    - theme: alt
      text: GitHub
      link: https://github.com/SuperiorChuo/gopherforge

features:
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z"/><path d="m8.5 9.2 3.5 2 3.5-2M12 11.2V16"/></svg>'
    title: 纯脚手架，零业务耦合
    details: 只含认证、RBAC、多租户、日志、文件与监控等平台能力。加业务能力，就是增加一个边界清晰的领域服务。
    link: /guide/architecture
    linkText: 查看架构边界
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m13 2-8 11h6l-1 9 9-12h-6V2Z"/></svg>'
    title: 三分钟拉起完整技术栈
    details: 一条命令启动 Traefik、Go 服务、React 前端、PostgreSQL、Redis 与 NATS，迁移和种子数据同步就绪。
    link: /guide/getting-started
    linkText: 开始运行
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h12v5H6zM4 16h6v5H4zM14 16h6v5h-6zM12 8v4M7 16v-4h10v4"/></svg>'
    title: 轻量审批流引擎
    details: 自研 Go 引擎支持会签、或签、依次审批、条件分支、超时动作与零代码表单发起，不引入沉重工作流依赖。
    link: /modules/bpm
    linkText: 探索审批流
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9l2 2-2 2M12 15h4"/></svg>'
    title: 代码生成器三种模式
    details: 从数据表生成 CRUD 前后端预览或 ZIP，覆盖单表、树表与主子表，并保留清晰的路由、菜单和迁移接入边界。
    link: /modules/codegen
    linkText: 查看生成能力
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 21V8l8-5 8 5v13M8 21v-7h8v7M8 10h.01M12 10h.01M16 10h.01"/></svg>'
    title: 多租户 SaaS 安全底座
    details: tenant_id 行级隔离、租户码登录与套餐权限包协同工作，GORM 插件自动挂载隔离条件，降低越权风险。
    link: /modules/tenant
    linkText: 了解租户隔离
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19V3M2 19h22"/></svg>'
    title: 可验证的工程闭环
    details: CI 门禁、OpenAPI 契约漂移检测、迁移彩排、Playwright E2E、Prometheus/Grafana 与可选 OTel 追踪一体化。
    link: /modules/observability
    linkText: 查看可观测能力
---

::: tip 当前发布线
当前文档对应 `v0.6.0` 正式版（[Release notes](/changelog)）。项目仍处于 0.x 阶段，API、数据库表结构和生成代码格式可能变化；在线 Demo 使用纯前端假数据，完整能力请按[快速上手](/guide/getting-started)启动本地栈，或直接[拉官方镜像部署](/reference/deployment)。
:::

## 找到你需要的答案

- **从零启动** — [快速上手](/guide/getting-started)、[架构总览](/guide/architecture)与[二次开发](/guide/extend)，建立第一条可运行链路。
- **构建前端** — [前端架构](/frontend/overview)、[请求层](/frontend/request)、[路由权限](/frontend/routing)与[页面规范](/frontend/page-dev)，沿用同一套工程约束。
- **走向生产** — [功能模块](/modules/auth)、[API 参考](/reference/api)、[生产部署](/reference/deployment)与[数据库结构](/reference/database)，把能力安全交付出去。

## 真实界面，同一套视觉语言

深空暗色与白蓝亮色共享同一组信息层级、品牌色和液态玻璃质感：

| 深空暗色 | 白蓝亮色 |
| --- | --- |
| ![系统概览 · 深空暗色](/screenshots/dashboard.png) | ![系统概览 · 白蓝亮色](/screenshots/dashboard-light.png) |

![用户管理](/screenshots/users.png)
