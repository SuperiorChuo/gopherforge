---
layout: home
pageClass: gopherforge-home

hero:
  name: GopherForge
  text: Open-source Go Microservices Admin Scaffold
  tagline: Production-minded service boundaries, unified auth, observability and delivery — forged into one ready-to-run platform foundation.
  actions:
    - theme: brand
      text: Get Started in 15 Minutes
      link: /en/guide/getting-started
    - theme: alt
      text: Explore Live Demo
      link: https://superiorchuo.github.io/gopherforge/
    - theme: alt
      text: GitHub
      link: https://github.com/SuperiorChuo/gopherforge

features:
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z"/><path d="m8.5 9.2 3.5 2 3.5-2M12 11.2V16"/></svg>'
    title: Pure scaffold, zero business coupling
    details: Auth, RBAC, multi-tenancy, audit, files and monitoring — platform capabilities only. Add your domain as a cleanly bounded service.
    link: /en/guide/architecture
    linkText: Explore the boundaries
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m13 2-8 11h6l-1 9 9-12h-6V2Z"/></svg>'
    title: Full stack in three minutes
    details: One command starts Traefik, Go services, React, PostgreSQL, Redis and NATS, including migrations and seed data.
    link: /en/guide/getting-started
    linkText: Start the stack
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h12v5H6zM4 16h6v5H4zM14 16h6v5h-6zM12 8v4M7 16v-4h10v4"/></svg>'
    title: Lightweight workflow engine
    details: A native Go engine for joint, any-one and sequential approval, branches, timeouts and no-code forms — without a heavyweight dependency.
    link: /en/modules/bpm
    linkText: Explore workflows
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9l2 2-2 2M12 15h4"/></svg>'
    title: Codegen in three modes
    details: Generate CRUD previews or ZIPs from schema metadata across single, tree and master-detail tables, with explicit integration boundaries.
    link: /en/modules/codegen
    linkText: View codegen
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 21V8l8-5 8 5v13M8 21v-7h8v7M8 10h.01M12 10h.01M16 10h.01"/></svg>'
    title: Secure multi-tenant foundation
    details: Row-level tenant_id isolation, tenant-code login and package-based permissions work together, enforced automatically by a GORM plugin.
    link: /en/modules/tenant
    linkText: Understand isolation
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19V3M2 19h22"/></svg>'
    title: Verifiable engineering loop
    details: CI gates, OpenAPI drift checks, migration rehearsals, Playwright E2E, Prometheus/Grafana and optional OTel tracing are built in.
    link: /en/modules/observability
    linkText: View observability
---

::: tip Current release line
These docs describe the `v0.6.0` release ([release notes](/en/changelog)). It is still a 0.x release: APIs, database schemas and generated code formats may change. The Live Demo uses front-end-only mock data; start the [full stack](/en/guide/getting-started) for backend verification, or [deploy from the official images](/en/reference/deployment).
:::

## Find the answer you need

- **Start from zero** — [Getting Started](/en/guide/getting-started), [Architecture](/en/guide/architecture) and [Extending](/en/guide/extend) establish your first working path.
- **Build the frontend** — [Frontend Architecture](/en/frontend/overview), [Request Layer](/en/frontend/request), [Routing](/en/frontend/routing) and [Page Development](/en/frontend/page-dev) share one engineering model.
- **Move to production** — [Modules](/en/modules/auth), [API Reference](/en/reference/api), [Deployment](/en/reference/deployment) and [Database Schema](/en/reference/database) help ship safely.

## Real interface, one visual language

Deep-space dark and cloud-light themes share the same hierarchy, brand palette and liquid-glass material:

| Deep-space dark | Cloud light |
| --- | --- |
| ![Dashboard, dark](/screenshots/dashboard.png) | ![Dashboard, light](/screenshots/dashboard-light.png) |
