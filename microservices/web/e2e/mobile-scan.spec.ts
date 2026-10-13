import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
// 管理员移动端壳（/m 子树）布局回归：路由清单由 routes.ts 从 router 真源动态
// 发现；移动壳只在手机视口使用，扫 390 单档即可。
// 运行：E2E_BASE_URL=http://localhost:8000 npx playwright test mobile-scan
import { discoverScanRoutes } from './routes'
import { scanPage } from './scan-lib'

const MOBILE = discoverScanRoutes().mobile

test.describe('移动端壳布局扫描', () => {
  test.use({ storageState: STORAGE_STATE })

  test.describe('mobile shell 390', () => {
    test.use({ viewport: { width: 390, height: 844 } })
    for (const path of MOBILE) {
      test(`390 ${path}`, async ({ page }) => { await scanPage(page, path, '390') })
    }
  })
})
