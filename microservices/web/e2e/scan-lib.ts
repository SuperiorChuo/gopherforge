// 布局扫描公共库：单页检查逻辑抽出来供 layout-scan（桌面）与 mobile-scan
// （移动壳 /m 子树）共用。扫描口径：
// ① body 横向溢出 ② 元素超出视口右侧/左侧 ③ console error / pageerror
// ④ {{}}/undefined/NaN 展示标记泄漏。
import { expect } from '@playwright/test'

export async function scanPage(page: import('@playwright/test').Page, path: string, viewport: string): Promise<void> {
  const errs: string[] = []
  page.on('console', (m) => {
    if (m.type() !== 'error') return
    const t = m.text()
    // 429 = 网关限流保护（全站并发扫描触发），非页面 bug
    if (t.includes('429')) return
    errs.push(t.slice(0, 120))
  })
  page.on('pageerror', (e) => errs.push(String(e).slice(0, 120)))
  page.on('response', (r) => { if (r.status() === 404 && r.request().resourceType() !== 'document') errs.push(`404: ${r.url().slice(0, 100)}`) })
  await page.goto(path).catch(() => {})
  await page.waitForTimeout(1500)
  const report = await page.evaluate(() => {
    const doc = document.documentElement
    const issues: string[] = []
    // ① body 横向溢出
    if (doc.scrollWidth > doc.clientWidth + 2) {
      issues.push(`BODY-OVERFLOW scrollWidth=${doc.scrollWidth} clientWidth=${doc.clientWidth}`)
    }
    // ② 超出视口的元素（排除 fixed 定位——贴边/抽屉属正常；排除表格横向滚动容器内部）
    const vw = window.innerWidth
    const bad: string[] = []
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT)
    let n
    while ((n = walker.nextNode())) {
      const el = n as HTMLElement
      const r = el.getBoundingClientRect()
      if (r.width <= 0 || r.height <= 0) continue
      const cs = getComputedStyle(el)
      if (cs.position === 'fixed' || cs.position === 'absolute') continue
      // SVG 内部元素（图表 path/g 等）几何可超视口属正常渲染，跳过
      if (el.ownerSVGElement) continue
      // 横向滚动容器内的元素超宽属设计模式（表格 ant-table-content、kanban
      // overflowX 列、tabs 滚动），跳过——滚动容器负责裁剪，不算页面变形
      let anc = el
      let inScroll = false
      while (anc && anc !== document.body) {
        if (anc.classList && anc.classList.contains('ant-table-content')) { inScroll = true; break }
        const acs = getComputedStyle(anc)
        if ((acs.overflowX === 'auto' || acs.overflowX === 'scroll') && (acs.overflowY !== 'hidden' || acs.overflowX === 'scroll')) {
          inScroll = true; break
        }
        anc = anc.parentElement
      }
      if (inScroll) continue
      if (r.right > vw + 2 || r.left < -2) {
        bad.push(`${el.tagName}.${String(el.className).slice(0, 35)} right=${Math.round(r.right)} left=${Math.round(r.left)} w=${Math.round(r.width)}`)
        if (bad.length >= 10) break
      }
    }
    // ③ 展示标记泄漏（{{}}/undefined/NaN 渲染在可见文本里）
    const leak = (() => {
      const marks = ['{{', 'undefined', 'NaN', 'null']  // null 仅匹配渲染出来的字面
      const out: string[] = []
      const walker2 = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
      let t
      while ((t = walker2.nextNode())) {
        const txt = t.textContent.trim()
        if (!txt) continue
        // 跳过脚本/样式内容与表格空态
        if (marks.some((m) => txt.includes(m)) && !(txt.length > 80)) {
          const el = t.parentElement
          if (el && el.getBoundingClientRect().width > 1) {
            out.push(`${txt.slice(0, 40)} [${el.tagName}.${String(el.className).slice(0, 20)}]`)
          }
        }
      }
      return out.slice(0, 6)
    })()
    return { issues, bad, leak }
  })
  const problems = [...report.issues, ...report.bad, ...report.leak, ...errs.map((e) => `CONSOLE-ERR: ${e}`)]
  if (problems.length) {
    console.log(`\n### ${viewport} ${path} (${problems.length})\n` + problems.slice(0, 8).join('\n'))
  }
  expect(report.issues.length, `${viewport} ${path} body overflow`).toBe(0)
}
