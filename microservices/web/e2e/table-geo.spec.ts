import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
const PAGES = ['/system/user', '/system/role', '/system/tenant', '/system/login-log', '/system/audit-log', '/ticket', '/crm/customers', '/bpm/tasks', '/monitor/job']
test.describe('表格几何', () => {
  test.use({ storageState: STORAGE_STATE, viewport: { width: 1440, height: 900 } })
  for (const path of PAGES) {
    test(`tg ${path}`, async ({ page }) => {
      await page.addInitScript(() => { if (!sessionStorage.getItem('c')) { sessionStorage.setItem('c','1'); localStorage.removeItem('app_locale') } })
      await page.goto(path).catch(() => {})
      await page.waitForTimeout(2500)
      const info = await page.evaluate(() => {
        const out = []
        // 表格容器：内容是否被裁（scrollHeight > clientHeight 且 overflow hidden）
        document.querySelectorAll('.ant-table-wrapper, .ant-table-container, .ant-table-body').forEach((el) => {
          const cs = getComputedStyle(el)
          if (el.scrollHeight > el.clientHeight + 5 && (cs.overflowY === 'hidden')) {
            out.push(`表格裁切: ${el.tagName}.${String(el.className).split(' ')[0]} h=${el.clientHeight} scroll=${el.scrollHeight}`)
          }
        })
        // 工具条是否与表格重叠
        const toolbar = document.querySelector('.table-toolbar')?.getBoundingClientRect()
        const table = document.querySelector('.ant-table-wrapper')?.getBoundingClientRect()
        if (toolbar && table && toolbar.bottom > table.top + 5) {
          out.push(`工具条与表格重叠: toolbar底=${Math.round(toolbar.bottom)} 表格顶=${Math.round(table.top)}`)
        }
        return out.slice(0, 5)
      })
      if (info.length) console.log(`\n### ${path}\n` + info.join('\n'))
    })
  }
})
