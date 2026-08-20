---
layout: home
pageClass: gopherforge-home

hero:
  name: GopherForge
  text: 开源 Go 微服务后台管理脚手架
  tagline: 认证、RBAC、多租户与可观测已经接好，可直接作为 Go 微服务业务项目的起点。
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
    title: 只保留平台能力
    details: 仓库提供认证、RBAC、多租户、日志、文件与监控，不预置商城、CMS 等业务模块；新业务按领域服务接入。
    link: /guide/architecture
    linkText: 查看架构边界
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m13 2-8 11h6l-1 9 9-12h-6V2Z"/></svg>'
    title: 一条命令拉起技术栈
    details: make compose-up 启动 Traefik、7 个 Go 服务、React 前端、PostgreSQL、Redis 与 NATS，并执行迁移和种子数据。
    link: /guide/getting-started
    linkText: 开始运行
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h12v5H6zM4 16h6v5H4zM14 16h6v5h-6zM12 8v4M7 16v-4h10v4"/></svg>'
    title: 轻量审批流引擎
    details: 自研 Go 引擎：会签、或签、依次审批、条件分支、超时动作、零代码表单，不用引重型工作流依赖。
    link: /modules/bpm
    linkText: 探索审批流
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9l2 2-2 2M12 15h4"/></svg>'
    title: 代码生成器三种模式
    details: 从数据表生成 CRUD 前后端预览或 ZIP，覆盖单表、树表与主子表，并保留清晰的路由、菜单和迁移接入边界。
    link: /modules/codegen
    linkText: 查看生成能力
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 21V8l8-5 8 5v13M8 21v-7h8v7M8 10h.01M12 10h.01M16 10h.01"/></svg>'
    title: 多租户 SaaS 安全底座
    details: 行级 tenant_id 隔离、租户码登录与套餐权限包配合，GORM 插件自动附加隔离条件，统一约束租户数据范围。
    link: /modules/tenant
    linkText: 了解租户隔离
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19V3M2 19h22"/></svg>'
    title: 质量门禁与可观测
    details: CI 门禁、OpenAPI 契约检测、迁移彩排、Playwright E2E、Prometheus/Grafana 与可选 OTel 追踪都配好了。
    link: /modules/observability
    linkText: 查看可观测能力
---
