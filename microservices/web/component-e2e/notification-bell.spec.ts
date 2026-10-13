import { expect, test, type WebSocketRoute } from '@playwright/test'

test('announcement unread survives inbox polls; opening only clears announcements', async ({ page }) => {
  const errors: string[] = []
  const unexpected: string[] = []
  const sockets: WebSocketRoute[] = []
  let inboxCount = 2
  let countRequests = 0
  page.on('pageerror', error => errors.push(error.message))
  await page.clock.install()
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const path = new URL(route.request().url()).pathname
    let data: unknown
    if (path === '/api/v1/ws/notifications/ticket') data = { ticket: 'test-notification-ticket' }
    else if (path === '/api/v1/notify/messages') data = { list: [], total: 0 }
    else if (path === '/api/v1/notify/messages/unread-count') {
      countRequests++
      data = { count: inboxCount }
    } else if (path === '/api/v1/notify/messages/read-all') {
      inboxCount = 0
      data = { updated: 2 }
    } else {
      unexpected.push(path)
      await route.abort()
      return
    }
    await route.fulfill({ json: { code: 200, message: 'ok', data } })
  })
  await page.routeWebSocket('**/api/v1/ws/notifications?*', socket => sockets.push(socket))
  await page.goto('/component-e2e/fixtures/notification-bell.html')
  const badge = page.locator('.ant-badge-count')
  await expect(badge).toHaveAttribute('title', '2')
  await expect.poll(() => sockets.length).toBe(1)
  await page.clock.fastForward(2_000) // Leave the bounded initial announcement replay window.
  sockets[0].send(JSON.stringify({
    id: 'test-announcement', type: 'announcement', title: 'Test notice', content: 'Unread notice',
    created_at: new Date().toISOString(),
  }))
  await expect(badge).toHaveAttribute('title', '3')

  let before = countRequests
  await page.clock.fastForward(45_000)
  await expect.poll(() => countRequests).toBeGreaterThan(before)
  await expect(badge).toHaveAttribute('title', '3')

  inboxCount = 0
  before = countRequests
  await page.clock.fastForward(45_000)
  await expect.poll(() => countRequests).toBeGreaterThan(before)
  await expect(badge).toHaveAttribute('title', '1')

  inboxCount = 2
  before = countRequests
  await page.clock.fastForward(45_000)
  await expect.poll(() => countRequests).toBeGreaterThan(before)
  await expect(badge).toHaveAttribute('title', '3')
  await page.locator('.app-trigger').click()
  await expect(badge).toHaveAttribute('title', '2')
  await page.getByRole('button', { name: '全部已读', exact: true }).click()
  await expect(badge).toHaveCount(0)
  expect(unexpected).toEqual([])
  expect(errors).toEqual([])
})
