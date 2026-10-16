import { expect, test } from '@playwright/test'

test('Pages subpath retains demo login and deep links without backend requests', async ({ page }) => {
  const errors: string[] = []
  const backendRequests: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route(/\/(?:api|im)\//, route => {
    backendRequests.push(route.request().url())
    return route.abort()
  })
  await page.routeWebSocket('**/*', socket => {
    backendRequests.push(socket.url())
    socket.close()
  })

  const login = await page.goto('login')
  expect(login?.status()).toBe(200)
  await expect(page.getByText('演示模式 · 纯前端假数据 · 任意账号可登录')).toBeVisible()
  await expect(page.getByRole('img', { name: '图形验证码', exact: true })).toBeVisible()
  await expect(page.locator('.login-shell')).toHaveCSS('animation-name', 'none')
  await page.getByPlaceholder('用户名').fill('demo-release')
  await page.getByPlaceholder('密码', { exact: true }).fill('demo-only')
  await page.getByPlaceholder('验证码').fill('1234')
  await page.getByRole('button', { name: /登\s*录/, exact: true }).click()
  await expect(page).toHaveURL(/\/gopherforge\/dashboard$/, { timeout: 20_000 })

  const users = await page.goto('system/user')
  expect(users?.status()).toBe(200)
  await expect(page.getByRole('main').getByText('张三', { exact: true })).toBeVisible()
  expect(errors).toEqual([])
  expect(backendRequests).toEqual([])
})

test('guard redirect returns to the source page without double-counting the Pages base', async ({ page }) => {
  // 未登录深链 → 守卫把来源（含部署 base）拼进 ?redirect= → 登录后必须回到来源页，
  // 不能双计 base 落到 /404。
  const entry = await page.goto('system/user')
  expect(entry?.status()).toBe(200)
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('img', { name: '图形验证码', exact: true })).toBeVisible()
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
  await expect(page.locator('.login-shell')).toHaveCSS('animation-name', 'none')
  await page.getByPlaceholder('用户名').fill('demo-release')
  await page.getByPlaceholder('密码', { exact: true }).fill('demo-only')
  await page.getByPlaceholder('验证码').fill('1234')
  await page.getByRole('button', { name: /登\s*录/, exact: true }).click()
  await expect(page).toHaveURL(/\/gopherforge\/system\/user$/, { timeout: 20_000 })
  await expect(page.getByRole('main').getByText('张三', { exact: true })).toBeVisible()
})
