import { expect, test } from '@playwright/test'

for (const width of [390, 1280]) {
  test(`品牌工具栏在 ${width}px 下不撑破页面且操作可达`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 })
    const backendRequests: string[] = []
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.route(/\/(?:api|im)\//, route => {
      backendRequests.push(route.request().url())
      return route.abort()
    })
    await page.routeWebSocket('**/*', socket => {
      backendRequests.push(socket.url())
      socket.close()
    })
    await page.goto('login')
    await expect(page.getByRole('img', { name: '图形验证码', exact: true })).toBeVisible()
    await page.getByPlaceholder('用户名').fill('demo-layout')
    await page.getByPlaceholder('密码', { exact: true }).fill('demo-only')
    await page.getByPlaceholder('验证码').fill('1234')
    await page.getByRole('button', { name: /登\s*录/, exact: true }).click()
    await expect(page).toHaveURL(/\/gopherforge\/dashboard$/)

    await page.goto('visibility/brands')
    await expect(page.locator('.table-toolbar-heading')).toContainText('品牌与竞品')
    await expect(page.locator('.ant-spin-spinning')).toHaveCount(0)
    const geometry = await page.evaluate(() => ({
      width: document.documentElement.clientWidth,
      scrollWidth: document.documentElement.scrollWidth,
    }))
    expect(geometry.scrollWidth, 'brand toolbar must not overflow the document').toBeLessThanOrEqual(geometry.width + 2)
    const toolbar = page.locator('.table-toolbar')
    for (const name of [/刷\s*新/, /命名分析/, /AI\s*识别/, /新\s*增/]) {
      await expect(toolbar.getByRole('button', { name })).toBeInViewport()
    }
    await page.screenshot({ path: testInfo.outputPath(`brands-${width}.png`), fullPage: true })
    await toolbar.getByRole('button', { name: /新\s*增/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    expect(errors).toEqual([])
    expect(backendRequests).toEqual([])
  })
}
