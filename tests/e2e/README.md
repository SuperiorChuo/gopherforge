# Playwright E2E 入口已迁移

浏览器级 E2E 统一维护在前端应用目录：

```bash
cd tdesign-vue-go
npm run e2e:frontend
```

根目录 `tests/e2e` 仅保留这份迁移说明，避免继续维护第二套 Playwright 配置。API smoke 仍由 `tests/api-smoke.sh` 覆盖。
