import { defineConfig, devices } from '@playwright/test'

const port = Number(process.env.DEMO_PREVIEW_PORT || 4177)
const baseURL = `http://127.0.0.1:${port}/gopherforge/`

// Pages is intentionally tested without a backend, separately from full-stack E2E.
export default defineConfig({
  testDir: './demo-e2e',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  retries: 0,
  reporter: 'list',
  // 功能回归使用产品已有的减少动效偏好；仍验证真实点击、守卫回跳与无后端请求。
  // 默认登录卡片持续浮动，不以 force 点击或重试绕过 Playwright 的稳定性检查。
  use: { baseURL, contextOptions: { reducedMotion: 'reduce' }, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npm run build && node scripts/prerender-spa.mjs && npm run preview -- --host 127.0.0.1 --port ${port} --strictPort`,
    env: { VITE_DEMO: '1', VITE_BASE: '/gopherforge/' },
    url: baseURL,
    timeout: 120_000,
    reuseExistingServer: false,
  },
})
