import { expect, test } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'
// 全站原始扫描：en 模式下看 body 里所有可见 CJK 短串（i18n 渐进词典的漏网
// 中文直查）。路由清单由 routes.ts 动态发现，勿手工登记。
import { discoverScanRoutes } from './routes'

const DESKTOP = discoverScanRoutes().desktop

test.describe('全站原始扫描', () => {
  test.use({ storageState: STORAGE_STATE })
  for (const path of DESKTOP) {
    test(`raw ${path}`, async ({ page }) => {
      const errs: string[] = []
      page.on('pageerror', (e) => errs.push(String(e).slice(0, 100)))
      await page.addInitScript(() => { if (!sessionStorage.getItem('c')) { sessionStorage.setItem('c','1'); localStorage.removeItem('app_locale') } })
      await page.goto(path)
      await page.waitForTimeout(1200)
      // 认证页没有 MainLayout；其语言按钮与主题按钮必须明确区分。
      const language = page.locator('.app-lang-trigger, .login-locale-toggle')
      await expect(language).toHaveCount(1)
      await expect(language).toContainText('EN')
      await language.click()
      await expect(language).toContainText('中文')
      await expect.poll(() => page.evaluate(() => localStorage.getItem('app_locale'))).toBe('en')
      await page.waitForTimeout(1200)
      const visibleZh = await page.evaluate(() => {
        const hu = /[一-鿿]/
        const out = []
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
        let n
        while ((n = walker.nextNode())) {
          const t = n.textContent.trim()
          if (!t || t.length < 2 || !hu.test(t)) continue
          const el = n.parentElement
          if (!el) continue
          const r = el.getBoundingClientRect()
          if (r.width < 1 || r.height < 1) continue
          const cs = getComputedStyle(el)
          if (cs.display === 'none' || cs.visibility === 'hidden') continue
          const tag = el.tagName
          const cls = String(el.className).slice(0, 25)
          out.push(`${t.slice(0, 25)} [${tag}.${cls}]`)
        }
        return out
      })
      const real = visibleZh.filter(s => !s.includes('[SPAN.app-lang-trigger]') && !s.includes('[SPAN.app-trigger]'))
      if (real.length) console.log(`\n### ${path} (${real.length})\n` + real.slice(0, 10).join('\n'))
      if (errs.length) console.log(`  ERR: ${errs[0]}`)
    })
  }
})
