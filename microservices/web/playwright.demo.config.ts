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
  use: { baseURL, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npm run build && node scripts/prerender-spa.mjs && npm run preview -- --host 127.0.0.1 --port ${port} --strictPort`,
    env: { VITE_DEMO: '1', VITE_BASE: '/gopherforge/' },
    url: baseURL,
    timeout: 120_000,
    reuseExistingServer: false,
  },
})
