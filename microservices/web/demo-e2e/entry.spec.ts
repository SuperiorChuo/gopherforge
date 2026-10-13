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
  await page.getByPlaceholder('用户名').fill('demo-release')
  await page.getByPlaceholder('密码', { exact: true }).fill('demo-only')
  await page.getByPlaceholder('验证码').fill('1234')
  await page.getByRole('button', { name: /登\s*录/, exact: true }).click()
  await expect(page).toHaveURL(/\/gopherforge\/dashboard$/)

  const users = await page.goto('system/user')
  expect(users?.status()).toBe(200)
  await expect(page.getByRole('main').getByText('张三', { exact: true })).toBeVisible()
  expect(errors).toEqual([])
  expect(backendRequests).toEqual([])
})
