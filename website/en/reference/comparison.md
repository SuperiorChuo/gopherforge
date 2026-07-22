# Comparison

The detailed comparison with gin-vue-admin, go-admin, RuoYi-family scaffolds is maintained in Chinese: [同类项目对比（中文）](/reference/comparison) · [source on GitHub](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/comparison.md).

TL;DR — what sets GopherForge apart:

- **Real microservices** (not a monolith) with a gateway, yet only ~3 containers of infra (Traefik + NATS + PostgreSQL) — far lighter than Java-stack equivalents.
- **Zero business coupling**: a scaffold, not a demo app to gut.
- **React 19 + Ant Design 6** frontend for teams that prefer React over Vue.
- **Engineering gates** rarely seen in this space: OpenAPI drift detection, migration rehearsal, full-stack E2E in CI.
- Built-in **workflow engine, code generator (3 modes), multi-tenancy** without heavyweight dependencies.
