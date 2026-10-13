import { expect, test, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const account = {
  id: 101,
  tenant_id: 1,
  app_id: 'wx-e2e-account',
  name: 'E2E 服务号',
  type: 'service',
  gh_id: 'gh_e2e',
  secret_ref: 'MP_E2E_SECRET',
  aes_key_ref: '',
  encrypt_mode: 'plain',
  auto_publish: false,
  status: 'active',
  created_at: '2026-08-01T08:00:00Z',
  updated_at: '2026-08-01T08:00:00Z',
}

const inbound = {
  id: 501,
  account_id: account.id,
  open_id: 'openid-e2e-safe',
  direction: 'in',
  msg_type: 'text',
  content: '请问今天营业吗？',
  msg_id: 'wx-msg-501',
  event: '',
  created_at: new Date().toISOString(),
}

function success(data: unknown) {
  return { code: 200, message: 'success', data }
}

async function fulfill(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(status === 200 ? success(data) : data),
  })
}

async function installMpRoutes(page: Page, options: { deferUpload?: boolean } = {}) {
  let menu = { account_id: account.id, buttons: [] as unknown[], published: false, updated_at: undefined as string | undefined }
  const replies: Array<Record<string, unknown>> = []
  let uploads = 0
  let releaseUpload: (() => void) | undefined

  await page.route('**/api/v1/mp/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const method = request.method()
    const path = url.pathname

    if (path === '/api/v1/mp/accounts' && method === 'GET') {
      await fulfill(route, { list: [account], total: 1, page: 1, page_size: 100 })
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/menu` && method === 'GET') {
      await fulfill(route, menu)
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/menu` && method === 'PUT') {
      const body = request.postDataJSON() as { buttons: unknown[] }
      menu = { account_id: account.id, buttons: body.buttons, published: false, updated_at: new Date().toISOString() }
      await fulfill(route, menu)
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/menu/publish` && method === 'POST') {
      menu = { ...menu, published: true, updated_at: new Date().toISOString() }
      await fulfill(route, menu)
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/menu` && method === 'DELETE') {
      menu = { account_id: account.id, buttons: [], published: false, updated_at: undefined }
      await fulfill(route, { deleted: true })
      return
    }
    if (path === '/api/v1/mp/messages' && method === 'GET') {
      await fulfill(route, { list: [inbound, ...replies], total: 1 + replies.length, page: 1, page_size: 10 })
      return
    }
    if (path === `/api/v1/mp/messages/${inbound.id}/reply` && method === 'POST') {
      const body = request.postDataJSON() as { msg_type?: string; content?: string; media_id?: string }
      const reply = {
        id: 600 + replies.length,
        account_id: account.id,
        open_id: inbound.open_id,
        direction: 'out',
        msg_type: body.msg_type || 'text',
        content: body.msg_type === 'image' ? body.media_id : body.content,
        msg_id: '',
        event: '',
        created_at: new Date().toISOString(),
      }
      replies.push(reply)
      await fulfill(route, { sent: true, archived: true, message: reply })
      return
    }
    if (path === `/api/v1/mp/accounts/${account.id}/customer-media` && method === 'POST') {
      if (options.deferUpload) await new Promise<void>((resolve) => { releaseUpload = resolve })
      uploads += 1
      await fulfill(route, { media_id: `MEDIA-E2E-${uploads}` })
      return
    }
    await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ code: 404, message: `unexpected ${method} ${path}` }) })
  })

  return {
    replies,
    getMenu: () => menu,
    getUploads: () => uploads,
    releaseUpload: () => releaseUpload?.(),
  }
}

test.describe('公众号 M4-A 管理闭环', () => {
  test('结构化菜单可保存、发布并删除', async ({ page }) => {
    const state = await installMpRoutes(page)
    await page.goto('/mp/menu')

    await expect(page.locator('.table-toolbar-heading')).toContainText('自定义菜单')
    await expect(page.getByText('E2E 服务号').first()).toBeVisible()
    await page.getByRole('button', { name: '添加一级菜单' }).click()

    await page.getByLabel('一级菜单 1 名称').fill('服务')
    await page.getByLabel('一级菜单 1 事件 Key').fill('CUSTOMER_SERVICE')

    page.once('dialog', async (dialog) => {
      expect(dialog.message()).toContain('未保存修改')
      await dialog.dismiss()
    })
    await page.getByRole('menuitem', { name: /消息记录/ }).click()
    await expect(page).toHaveURL(/\/mp\/menu/)

    await page.getByRole('button', { name: '重载' }).click()
    await expect(page.getByRole('dialog', { name: '重新加载菜单' })).toBeVisible()
    await page.getByRole('button', { name: '继续编辑' }).click()
    await expect(page.getByLabel('一级菜单 1 名称')).toHaveValue('服务')

    await page.getByRole('button', { name: '保存草稿' }).click()
    await expect(page.locator('.mp-menu-account-filter .status-pill')).toHaveText('草稿')
    expect(state.getMenu().buttons).toEqual([{ name: '服务', type: 'click', key: 'CUSTOMER_SERVICE' }])

    await page.getByRole('button', { name: /发布$/ }).click()
    await expect(page.locator('.mp-menu-account-filter .status-pill')).toHaveText('已发布')
    expect(state.getMenu().published).toBe(true)

    await page.getByRole('button', { name: /删除$/ }).click()
    await page.getByRole('dialog', { name: '删除微信自定义菜单' }).getByRole('button', { name: /删\s*除/ }).click()
    await expect(page.getByRole('button', { name: '添加一级菜单' })).toBeVisible()
    expect(state.getMenu().buttons).toEqual([])
  })

  test('入站消息可安全派生文本与图片回复目标', async ({ page }) => {
    const state = await installMpRoutes(page)
    await page.goto('/mp/messages')

    await expect(page.getByText(inbound.content)).toBeVisible()
    await page.getByRole('button', { name: '回复' }).click()
    await page.getByLabel('回复内容').fill('今天正常营业。')
    await page.getByRole('button', { name: /发送$/ }).click()
    await expect.poll(() => state.replies.length).toBe(1)
    expect(state.replies[0]).toMatchObject({ open_id: inbound.open_id, msg_type: 'text', content: '今天正常营业。' })

    await page.getByRole('button', { name: '回复' }).click()
    await page.getByText('图片', { exact: true }).click()
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64')
    await page.locator('input[type="file"]').setInputFiles({ name: 'reply.png', mimeType: 'image/png', buffer: png })
    await expect(page.getByLabel('图片素材')).toHaveValue('MEDIA-E2E-1')
    await page.getByRole('button', { name: /发送$/ }).click()
    await expect.poll(() => state.replies.length).toBe(2)
    expect(state.getUploads()).toBe(1)
    expect(state.replies[1]).toMatchObject({ open_id: inbound.open_id, msg_type: 'image', content: 'MEDIA-E2E-1' })
  })

  test('图片上传期间锁定回复目标与弹窗关闭', async ({ page }) => {
    const state = await installMpRoutes(page, { deferUpload: true })
    await page.goto('/mp/messages')
    await page.getByRole('button', { name: '回复' }).click()
    await page.getByText('图片', { exact: true }).click()
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=', 'base64')
    await page.locator('input[type="file"]').setInputFiles({ name: 'reply.png', mimeType: 'image/png', buffer: png })

    await expect(page.getByRole('button', { name: /取\s*消/ })).toBeDisabled()
    await expect(page.locator('.ant-modal-close')).toHaveCount(0)
    state.releaseUpload()
    await expect(page.getByLabel('图片素材')).toHaveValue('MEDIA-E2E-1')
    await expect(page.getByRole('button', { name: /取\s*消/ })).toBeEnabled()
  })
})
