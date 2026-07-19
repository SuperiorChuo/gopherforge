import { defineConfig, devices } from '@playwright/test'

// E2E 目标是「网关后的完整栈」（CI 的 integration job 会起全栈到 :8000）。
// 本地跑：BASE_URL=http://127.0.0.1:8000 npx playwright test（需先起栈）。
const BASE_URL = process.env.E2E_BASE_URL || 'http://127.0.0.1:8000'

export default defineConfig({
  testDir: './e2e',
  // 登录态由 global-setup 通过 API 拿 token 写入 storageState，测试直接复用。
  globalSetup: './e2e/global-setup.ts',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['list']] : [['list']],
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
