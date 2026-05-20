# Optional Playwright E2E

This directory is intentionally not wired into `make test` or CI. It is a place for browser-level smoke tests once the local frontend and backend are already running.

Suggested local setup when browser E2E is needed:

```bash
cd tdesign-vue-go
npm install --save-dev @playwright/test
npx playwright install chromium
npx playwright test ../tests/e2e --config ../tests/e2e/playwright.config.mjs
```

Keep API coverage in `tests/api-smoke.sh`; keep this directory for browser behavior that cannot be covered through API calls.
