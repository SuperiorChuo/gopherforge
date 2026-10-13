# API 契约

微服务线使用 monitor 服务（及历史 Gin 路由）生成 OpenAPI，再生成前端类型声明。

## 命令（在 `microservices/` 下）

```bash
npm run openapi      # 生成 services/monitor/docs/openapi.json
npm run api:types    # 生成 web/src/api/generated/schema.d.ts
npm run api:contract # 两者串联
npm run test:contract
```

检查漂移：

```bash
git diff --exit-code -- services/monitor/docs/openapi.json
# 如提交了类型生成物：
git diff --exit-code -- web/src/api/generated/schema.d.ts
```

## 说明

- 业务 API 已拆到各微服务；monitor 的 OpenAPI 主要覆盖健康/监控/指标等兜底面。
- 完整业务契约可按服务后续拆分维护。
- 单体线使用 `monolith/server/docs/openapi.json`，与微服务契约独立。

更多产品线边界见 `docs/PRODUCT_LINES.md`。
