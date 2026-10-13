import { test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
const PAGES = ['/system/user', '/system/tenant', '/system/login-log', '/ticket', '/crm/customers', '/cc/queues', '/hermes/mine', '/dashboard', '/system/setting']
test.describe('固定元素扫描', () => {
  test.use({ storageState: STORAGE_STATE, viewport: { width: 1440, height: 900 } })
  for (const path of PAGES) {
    test(`fx ${path}`, async ({ page }) => {
      await page.addInitScript(() => { if (!sessionStorage.getItem('c')) { sessionStorage.setItem('c','1'); localStorage.removeItem('app_locale') } })
      await page.goto(path).catch(() => {})
      await page.waitForTimeout(2500)
      const fixed = await page.evaluate(() => {
        const out = []
        document.querySelectorAll('body *').forEach((el) => {
          const cs = getComputedStyle(el)
          if ((cs.position === 'fixed' || cs.position === 'sticky') && el.getBoundingClientRect().width > 50) {
            const r = el.getBoundingClientRect()
            const z = cs.zIndex
            out.push(`${el.tagName}.${String(el.className).split(' ')[0]} pos=${cs.position} z=${z} top=${Math.round(r.top)} h=${Math.round(r.height)}`)
          }
        })
        return out
      })
      console.log(`\n### ${path}\n` + (fixed.length ? fixed.join('\n') : '无 fixed/sticky 元素'))
    })
  }
})
