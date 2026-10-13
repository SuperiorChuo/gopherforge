import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
const PAGES = ['/system/user', '/system/role', '/system/tenant', '/system/menu', '/system/department', '/system/login-log',
  '/system/operation-log', '/system/audit-log', '/system/notice', '/system/file', '/system/sms', '/system/dict',
  '/monitor/server', '/monitor/job', '/bpm/tasks', '/bpm/instances', '/bpm/definitions', '/crm/customers', '/crm/leads',
  '/ticket', '/pay', '/cc/queues', '/cc/trunks', '/cc/billing', '/hermes/overview', '/hermes/mine', '/mp/messages', '/mp/fans']
test.describe('列表裁切/遮挡扫描', () => {
  test.use({ storageState: STORAGE_STATE, viewport: { width: 1440, height: 900 } })
  for (const path of PAGES) {
    test(`clip ${path}`, async ({ page }) => {
      await page.addInitScript(() => { if (!sessionStorage.getItem('c')) { sessionStorage.setItem('c','1'); localStorage.removeItem('app_locale') } })
      await page.goto(path).catch(() => {})
      await page.waitForTimeout(2500)
      const issues = await page.evaluate(() => {
        const out = []
        const all = document.querySelectorAll('*')
        for (const el of all) {
          const r = el.getBoundingClientRect()
          if (r.width < 5 || r.height < 5) continue
          // 1. 内容溢出被裁：scrollWidth > clientWidth 且 overflow hidden/auto（文本被截断）
          if (el.scrollWidth > el.clientWidth + 2 && (getComputedStyle(el).overflow === 'hidden' || getComputedStyle(el).overflowX === 'hidden')) {
            if (el.textContent.trim().length > 0 && el.clientWidth > 0) {
              out.push(`裁切: ${el.tagName}.${String(el.className).split(' ')[0]} ${el.clientWidth}px < scroll ${el.scrollWidth}px 文本「${el.textContent.trim().slice(0, 15)}」`)
            }
          }
        }
        // 2. 遮挡：检测 z-index 大的浮层是否盖住表格
        document.querySelectorAll('.ant-table').forEach(tbl => {
          const tr = tbl.getBoundingClientRect()
          document.querySelectorAll('*').forEach(el => {
            const z = parseInt(getComputedStyle(el).zIndex || '0')
            if (z >= 100 && el.getBoundingClientRect && el !== tbl) {
              const er = el.getBoundingClientRect()
              const overlap = Math.min(tr.right, er.right) - Math.max(tr.left, er.left)
              if (overlap > 100 && er.top < tr.bottom && er.top > tr.top && !el.closest('.ant-table')) {
                out.push(`遮挡: ${el.tagName}.${String(el.className).split(' ')[0]} z=${z} 盖在表格上`)
              }
            }
          })
        })
        return out.slice(0, 8)
      })
      if (issues.length) console.log(`\n### ${path} (${issues.length})\n` + issues.join('\n'))
    })
  }
})
