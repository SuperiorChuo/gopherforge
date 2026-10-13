import { defineConfig, devices } from '@playwright/test'

const port = Number(process.env.COMPONENT_TEST_PORT || 4179)
const baseURL = `http://127.0.0.1:${port}`

// Real React components with explicit network fixtures; not backend acceptance.
export default defineConfig({
  testDir: './component-e2e',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  retries: 0,
  reporter: 'list',
  use: { baseURL, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${port} --strictPort`,
    env: { VITE_DEMO: '0', VITE_BASE: '/' },
    url: baseURL,
    timeout: 60_000,
    reuseExistingServer: false,
  },
})
