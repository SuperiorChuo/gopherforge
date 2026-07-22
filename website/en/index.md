---
layout: home

hero:
  name: GopherForge
  text: Open-source Go Microservices Admin Scaffold
  tagline: Go + Gin services split by domain · React 19 + Ant Design 6 · Traefik gateway with unified auth · full stack up with one docker compose command
  actions:
    - theme: brand
      text: Getting Started (15 min)
      link: /en/guide/getting-started
    - theme: alt
      text: Live Demo
      link: https://superiorchuo.github.io/gopherforge/
    - theme: alt
      text: GitHub
      link: https://github.com/SuperiorChuo/gopherforge

features:
  - icon: 🧩
    title: Pure scaffold, zero business coupling
    details: Only platform-agnostic infrastructure services — auth, RBAC, multi-tenancy, audit logs, files, monitoring. A clean starting point; adding a business capability = one new microservice + gateway labels.
  - icon: 🚀
    title: Batteries included
    details: docker compose up brings up the Traefik gateway, 8 Go services, the React frontend and PostgreSQL/Redis/NATS in about 3 minutes, with migrations and seed data.
  - icon: 📋
    title: Lightweight workflow engine
    details: A home-grown Go approval engine (no Flowable) — DingTalk-style designer, AND/OR/sequential approval, conditional branches, timeout auto-actions, no-code form flows and approval analytics.
  - icon: 🛠️
    title: Code generator, three modes
    details: Pick a table, configure fields, generate full CRUD backend + frontend. Supports single-table, tree-table and master-detail modes.
  - icon: 🏢
    title: Multi-tenant SaaS foundation
    details: Shared database with tenant_id row isolation, tenant-code login, packages as permission bundles. A GORM plugin auto-scopes every tenant table — a missed manual scope no longer means a data leak.
  - icon: 📐
    title: Solid engineering
    details: Green-gate CI, OpenAPI contract drift detection, migration rehearsal, Playwright E2E, Prometheus/Grafana and optional OpenTelemetry tracing.
---

## UI Preview

Deep-space dark / light themes with one visual language:

| 🌌 Dark | ☁️ Light |
| --- | --- |
| ![Dashboard, dark](/screenshots/dashboard.png) | ![Dashboard, light](/screenshots/dashboard-light.png) |
