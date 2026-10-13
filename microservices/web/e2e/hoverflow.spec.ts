import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
const PAGES = ['/system/user', '/system/role', '/system/tenant', '/system/menu', '/system/login-log', '/system/audit-log',
  '/system/notice', '/system/file', '/system/sms', '/monitor/server', '/monitor/job', '/bpm/tasks', '/crm/customers',
  '/ticket', '/pay', '/cc/queues', '/hermes/mine', '/mp/fans']
test.describe('横向溢出扫描', () => {
  test.use({ storageState: STORAGE_STATE, viewport: { width: 1440, height: 900 } })
  for (const path of PAGES) {
    test(`ho ${path}`, async ({ page }) => {
      await page.addInitScript(() => { if (!sessionStorage.getItem('c')) { sessionStorage.setItem('c','1'); localStorage.removeItem('app_locale') } })
      await page.goto(path).catch(() => {})
      await page.waitForTimeout(2500)
      const info = await page.evaluate(() => {
        const bodyScroll = document.body.scrollWidth
        const viewport = window.innerWidth
        // 找最右超出视口的元素
        const over = []
        document.querySelectorAll('body *').forEach((el) => {
          const r = el.getBoundingClientRect()
          if (r.width > 0 && r.right > viewport + 3 && !el.closest('.ant-table-content') && !el.closest('.ant-menu') && !el.closest('svg')) {
            over.push(`${el.tagName}.${String(el.className).split(' ')[0]} 右缘=${Math.round(r.right)}`)
          }
        })
        return { bodyScroll, viewport, over: over.slice(0, 5) }
      })
      const overflow = info.bodyScroll > info.viewport + 10
      if (overflow || info.over.length) {
        console.log(`\n### ${path} bodyScroll=${info.bodyScroll} viewport=${info.viewport}\n` + (info.over.length ? info.over.join('\n') : '无元素超右缘但 body 超宽'))
      }
    })
  }
})
