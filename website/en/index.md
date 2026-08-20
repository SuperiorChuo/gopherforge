---
layout: home
pageClass: gopherforge-home

hero:
  name: GopherForge
  text: Open-source Go Microservices Admin Scaffold
  tagline: Auth, RBAC, multi-tenancy and observability are wired up, ready to use as the starting point for a Go microservices project.
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
    title: Platform capabilities only
    details: Auth, RBAC, multi-tenancy, audit, files and monitoring are included; commerce, CMS and other business modules are not. Add each domain as a service.
    link: /en/guide/architecture
    linkText: Explore the boundaries
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m13 2-8 11h6l-1 9 9-12h-6V2Z"/></svg>'
    title: One command to start the stack
    details: make compose-up starts Traefik, 7 Go services, React, PostgreSQL, Redis and NATS, then runs migrations and seed data.
    link: /en/guide/getting-started
    linkText: Start the stack
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h12v5H6zM4 16h6v5H4zM14 16h6v5h-6zM12 8v4M7 16v-4h10v4"/></svg>'
    title: Lightweight workflow engine
    details: A native Go engine for joint, any-one and sequential approval, branches, timeouts and no-code forms — no heavyweight dependency.
    link: /en/modules/bpm
    linkText: Explore workflows
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 5h16v14H4zM8 9l2 2-2 2M12 15h4"/></svg>'
    title: Codegen in three modes
    details: Generate CRUD previews or ZIPs from schema metadata across single, tree and master-detail tables, with explicit integration boundaries.
    link: /en/modules/codegen
    linkText: View codegen
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 21V8l8-5 8 5v13M8 21v-7h8v7M8 10h.01M12 10h.01M16 10h.01"/></svg>'
    title: Secure multi-tenant foundation
    details: Row-level tenant_id isolation, tenant-code login and package-based permissions, enforced by a GORM plugin.
    link: /en/modules/tenant
    linkText: Understand isolation
  - icon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 19V9M10 19V5M16 19v-7M22 19V3M2 19h22"/></svg>'
    title: Quality gates & observability
    details: CI gates, OpenAPI drift checks, migration rehearsals, Playwright E2E, Prometheus/Grafana and optional OTel tracing are all wired up.
    link: /en/modules/observability
    linkText: View observability
---

::: tip Current release line
These docs describe the `v0.6.0` release ([release notes](/en/changelog)). It is still a 0.x release: APIs, database schemas and generated code formats may change. The Live Demo uses front-end-only mock data; start the [full stack](/en/guide/getting-started) for backend verification, or [deploy from the official images](/en/reference/deployment).
:::

## Browse by topic

- **Start from zero** — [Getting Started](/en/guide/getting-started), [Architecture](/en/guide/architecture) and [Extending](/en/guide/extend) get the project running first.
- **Build the frontend** — [Frontend Architecture](/en/frontend/overview), [Request Layer](/en/frontend/request), [Routing](/en/frontend/routing) and [Page Development](/en/frontend/page-dev) share one set of conventions.
- **Move to production** — [Modules](/en/modules/auth), [API Reference](/en/reference/api), [Deployment](/en/reference/deployment) and [Database Schema](/en/reference/database) get you ready to ship.

## One UI, two themes

Dark and light themes share the same layout, brand palette and glass material:

| Deep-space dark | Cloud light |
| --- | --- |
| ![Dashboard, dark](/screenshots/dashboard.png) | ![Dashboard, light](/screenshots/dashboard-light.png) |
