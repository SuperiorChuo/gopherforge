# Choosing a platform

The GopherForge v0.8.0 candidate delivers complete business source code, not an empty infrastructure scaffold: 16 Go API services, an AI worker, the React application, and separately deployed media and client products.

It suits Go + React teams seeking RBAC, tenancy, audit and monitoring alongside AI, CRM, messaging, payment and ticket implementations. You remain responsible for multi-service operations, provider configuration, upgrade rehearsals and production capacity.

Reconsider the fit if you need a small monolith, require Vue/MySQL, or expect real payments, SMS and telephone calls without provider accounts.

## Comparison sources

Check current architecture, licenses, maintenance, migrations and required business functionality directly:

- [gin-vue-admin](https://github.com/flipped-aurora/gin-vue-admin)
- [go-admin](https://github.com/go-admin-team/go-admin)
- [RuoYi-Cloud](https://gitee.com/y_project/RuoYi-Cloud)
- [Simple Admin](https://github.com/suyuan32/simple-admin-core)

See [Architecture](/en/guide/architecture) and [Deployment](/en/reference/deployment). The [online demo](https://superiorchuo.github.io/gopherforge/) uses mock data and is not a production acceptance environment.
