# Playwright E2E 入口已迁移

浏览器级 E2E 统一维护在前端应用目录：

```bash
cd web (微服务前端) 或仓库根 tdesign-vue-go（遗留）
npm run e2e:frontend
```

根目录 `tests/e2e` 仅保留这份迁移说明，避免继续维护第二套 Playwright 配置。API smoke 仍由 `tests/api-smoke.sh` 覆盖。
