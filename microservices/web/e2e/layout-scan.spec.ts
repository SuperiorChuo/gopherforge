import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
// 全站布局扫描：路由清单由 routes.ts 从 router 真源动态发现（新增页面自动
// 入扫，勿再手工登记）。× 1280/390 双档，检查口径见 scan-lib。
// 运行：E2E_BASE_URL=http://localhost:8000 npx playwright test layout-scan
import { discoverScanRoutes } from './routes'
import { scanPage } from './scan-lib'

const DESKTOP = discoverScanRoutes().desktop

test.describe('全站布局扫描', () => {
  test.use({ storageState: STORAGE_STATE })

  test.describe('desktop 1280', () => {
    test.use({ viewport: { width: 1280, height: 800 } })
    for (const path of DESKTOP) {
      test(`1280 ${path}`, async ({ page }) => { await scanPage(page, path, '1280') })
    }
  })

  test.describe('mobile 390', () => {
    test.use({ viewport: { width: 390, height: 844 } })
    for (const path of DESKTOP) {
      test(`390 ${path}`, async ({ page }) => { await scanPage(page, path, '390') })
    }
  })
})
