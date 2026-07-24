---
layout: home

hero:
  name: GopherForge
  text: 开源 Go 微服务后台管理脚手架
  tagline: Go + Gin 按域拆分基础服务 · React 19 + Ant Design 6 · Traefik 网关统一鉴权 · docker compose 一条命令拉起全栈
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
  - icon: 🧩
    title: 纯脚手架，零业务耦合
    details: 只含认证、RBAC、多租户、日志、文件、监控等平台无关的基础设施服务，是干净的项目起点——加业务能力 = 加一个微服务 + 网关标签。
  - icon: 🚀
    title: 开箱即用
    details: make compose-up 约 3 分钟拉起 Traefik 网关 + 7 个 Go 服务 + React 前端 + PostgreSQL/Redis/NATS，自带迁移与种子数据；应用栈与数据栈分离。
  - icon: 📋
    title: 轻量审批流引擎
    details: 自研 Go 引擎不引 Flowable——仿钉钉设计器、会签/或签/依次、条件分支、超时自动动作、流程表单零代码发起、审批统计。
  - icon: 🛠️
    title: 代码生成器三模式
    details: 选表配字段生成 CRUD 前后端预览或 ZIP，支持单表、树表、主子表三种模式；下载后仍需按项目规范接入路由、菜单和迁移。
  - icon: 🏢
    title: 多租户 SaaS 底座
    details: 共享库 + tenant_id 隔离，登录带租户码，套餐即权限包；GORM 插件级自动隔离，漏挂 scope 也不会越权。
  - icon: 📐
    title: 工程完备
    details: CI 全绿门禁、OpenAPI 契约漂移检测、迁移彩排、Playwright E2E、Prometheus/Grafana、可选 OTel 链路追踪。
---

::: tip 当前发布线
当前文档对应 `v0.2.0-rc.1` 候选版。项目仍处于 0.x 阶段，API、数据库表结构和生成代码格式可能变化；在线 Demo 使用纯前端假数据，完整能力请按[快速上手](/guide/getting-started)启动本地栈。
:::

## 界面预览

深空暗色 / 白蓝亮色双主题，一套视觉语言：

| 🌌 深空暗色 | ☁️ 白蓝亮色 |
| --- | --- |
| ![系统概览 · 深空暗色](/screenshots/dashboard.png) | ![系统概览 · 白蓝亮色](/screenshots/dashboard-light.png) |

![用户管理](/screenshots/users.png)
