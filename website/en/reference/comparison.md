# Comparison

The detailed comparison with gin-vue-admin, go-admin, RuoYi-family scaffolds is maintained in Chinese: [同类项目对比（中文）](/reference/comparison) · [source on GitHub](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/comparison.md).

TL;DR — what sets GopherForge apart:

- **Microservices from the start**: 7 Go services behind Traefik, with PostgreSQL, Redis and NATS as the required stateful dependencies.
- **No bundled business modules**: it starts as a platform scaffold rather than a demo application to strip down.
- **React 19 + Ant Design 6** frontend for teams that prefer React over Vue.
- **Engineering gates** rarely seen in this space: OpenAPI drift detection, migration rehearsal, full-stack E2E in CI.
- Built-in **workflow engine, code generator (3 modes), multi-tenancy** without heavyweight dependencies.
