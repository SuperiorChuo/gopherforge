import { expect, test, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const account = {
  id: 151,
  tenant_id: 1,
  app_id: 'wx-e2e-articles',
  name: 'E2E 图文服务号',
  type: 'service',
  gh_id: 'gh_e2e_articles',
  secret_ref: 'MP_E2E_ARTICLE_SECRET',
  aes_key_ref: '',
  encrypt_mode: 'plain',
  auto_publish: false,
  status: 'active',
  created_at: '2026-08-05T08:00:00Z',
  updated_at: '2026-08-05T08:00:00Z',
}

const article = {
  id: 551,
  tenant_id: 1,
  account_id: account.id,
  title: 'E2E 待发布图文',
  author: 'E2E 作者',
  digest: '发布前保存测试摘要',
  content_html: '<p>保存前正文</p>',
  cover_url: '',
  thumb_media_id: '',
  draft_media_id: '',
  publish_id: '',
  article_url: '',
  status: 'draft_ai',
  ai_prompt: '生成一篇用于 E2E 的文章',
  created_by: 1,
  created_at: '2026-08-05T08:10:00Z',
  updated_at: '2026-08-05T08:10:00Z',
}

type RecordedRequest = {
  method: 'PUT' | 'POST'
  path: string
  body: Record<string, unknown>
}

function success(data: unknown) {
  return { code: 200, message: 'success', data }
}

async function fulfill(route: Route, data: unknown) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(success(data)),
  })
}

async function fulfillError(route: Route, message: string) {
  await route.fulfill({
    status: 500,
    contentType: 'application/json',
    body: JSON.stringify({ code: 500, message, data: null }),
  })
}

function deferred() {
  let release = () => {}
  const promise = new Promise<void>((resolve) => { release = resolve })
  return { promise, release }
}

async function installArticleRoutes(
  page: Page,
  options: { deferSave?: boolean; failSave?: boolean; deferCover?: boolean } = {},
) {
  let currentArticle = { ...article }
  const requests: RecordedRequest[] = []
  const saveGate = deferred()
  const coverGate = deferred()

  await page.route('**/api/v1/mp/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const method = request.method()

    if (path === '/api/v1/mp/accounts' && method === 'GET') {
      await fulfill(route, { list: [account], total: 1, page: 1, page_size: 100 })
      return
    }
    if (path === '/api/v1/mp/articles' && method === 'GET') {
      await fulfill(route, { list: [currentArticle], total: 1, page: 1, page_size: 50 })
      return
    }
    if (path === `/api/v1/mp/articles/${article.id}` && method === 'PUT') {
      const body = request.postDataJSON() as Record<string, unknown>
      requests.push({ method, path, body })
      if (options.deferSave) await saveGate.promise
      if (options.failSave) {
        await fulfillError(route, '模拟保存失败')
        return
      }
      currentArticle = {
        ...currentArticle,
        ...body,
        updated_at: '2026-08-05T08:20:00Z',
      } as typeof article
      await fulfill(route, currentArticle)
      return
    }
    if (path === `/api/v1/mp/articles/${article.id}/cover` && method === 'POST') {
      const body = request.postDataJSON() as Record<string, unknown>
      requests.push({ method, path, body })
      if (options.deferCover) await coverGate.promise
      currentArticle = {
        ...currentArticle,
        cover_url: 'data:image/png;base64,iVBORw0KGgo=',
        updated_at: '2026-08-05T08:20:30Z',
      }
      await fulfill(route, { cover_url: currentArticle.cover_url })
      return
    }
    if (path === `/api/v1/mp/articles/${article.id}/publish` && method === 'POST') {
      const body = request.postDataJSON() as Record<string, unknown>
      requests.push({ method, path, body })
      currentArticle = {
        ...currentArticle,
        status: 'drafted',
        draft_media_id: 'DRAFT-E2E-551',
        updated_at: '2026-08-05T08:21:00Z',
      }
      await fulfill(route, currentArticle)
      return
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ code: 404, message: `unexpected ${method} ${path}`, data: null }),
    })
  })

  return {
    getArticle: () => currentArticle,
    getRequests: () => [...requests],
    getSaveCount: () => requests.filter((request) => request.method === 'PUT').length,
    getCoverCount: () => requests.filter((request) => request.path.endsWith('/cover')).length,
    getPublishCount: () => requests.filter((request) => request.path.endsWith('/publish')).length,
    releaseSave: saveGate.release,
    releaseCover: coverGate.release,
  }
}

test.describe('公众号 M5 图文发布前保存', () => {
  test('未单独保存的正文先完成 PUT 再进入公众号草稿箱', async ({ page }) => {
    const state = await installArticleRoutes(page, { deferSave: true })
    const editedContent = '<p>发布时的新正文</p><img src="data:image/png;base64,aW1hZ2U=" alt="内嵌图" />'

    await page.goto('/mp/articles')
    await expect(page.getByRole('cell', { name: article.title })).toBeVisible()
    await page.getByRole('button', { name: '编辑' }).click()

    const drawer = page.getByRole('dialog', { name: '编辑图文' })
    const contentInput = drawer.getByLabel('正文（HTML）')
    await expect(contentInput).toHaveValue(article.content_html)
    await contentInput.fill(editedContent)

    const publishButton = drawer.getByRole('button', { name: '发布 / 存草稿箱' })
    await publishButton.click()
    await expect.poll(() => state.getSaveCount()).toBe(1)

    expect(state.getPublishCount()).toBe(0)
    expect(state.getRequests()).toHaveLength(1)
    expect(state.getRequests()[0]).toMatchObject({
      method: 'PUT',
      path: `/api/v1/mp/articles/${article.id}`,
      body: { content_html: editedContent },
    })
    await expect(publishButton).toBeDisabled()
    await expect(contentInput).toBeDisabled()

    state.releaseSave()
    await expect.poll(() => state.getPublishCount()).toBe(1)

    const requests = state.getRequests()
    expect(requests.map(({ method, path }) => `${method} ${path}`)).toEqual([
      `PUT /api/v1/mp/articles/${article.id}`,
      `POST /api/v1/mp/articles/${article.id}/publish`,
    ])
    expect(requests[0].body).toMatchObject({ content_html: editedContent })
    expect(requests[1].body).toEqual({})
    expect(state.getArticle()).toMatchObject({ content_html: editedContent, status: 'drafted' })

    await expect(page.getByText('已存入公众号草稿箱', { exact: true })).toBeVisible()
    await expect(drawer.getByText('已入草稿箱', { exact: true })).toBeVisible()
    await expect(contentInput).toHaveValue(editedContent)
    await expect(contentInput).toBeEnabled()
    await expect(publishButton).toBeEnabled()
  })

  test('PUT 保存失败时不发布并保留抽屉输入', async ({ page }) => {
    const state = await installArticleRoutes(page, { failSave: true })
    const editedContent = '<p>保存失败后仍需保留</p><img src="data:image/png;base64,ZmFpbA==" />'

    await page.goto('/mp/articles')
    await expect(page.getByRole('cell', { name: article.title })).toBeVisible()
    await page.getByRole('button', { name: '编辑' }).click()

    const drawer = page.getByRole('dialog', { name: '编辑图文' })
    const contentInput = drawer.getByLabel('正文（HTML）')
    await contentInput.fill(editedContent)

    const publishButton = drawer.getByRole('button', { name: '发布 / 存草稿箱' })
    await publishButton.click()
    await expect.poll(() => state.getSaveCount()).toBe(1)
    await expect(page.getByText('模拟保存失败', { exact: true }).first()).toBeVisible()

    expect(state.getPublishCount()).toBe(0)
    expect(state.getRequests().map(({ method }) => method)).toEqual(['PUT'])
    await expect(drawer).toBeVisible()
    await expect(contentInput).toHaveValue(editedContent)
    await expect(contentInput).toBeEnabled()
    await expect(publishButton).toBeEnabled()
    await expect(drawer.getByRole('button', { name: /保\s*存/ })).toBeEnabled()
  })

  test('手动保存请求在途时锁定发布和编辑', async ({ page }) => {
    const state = await installArticleRoutes(page, { deferSave: true })

    await page.goto('/mp/articles')
    await expect(page.getByRole('cell', { name: article.title })).toBeVisible()
    await page.getByRole('button', { name: '编辑' }).click()

    const drawer = page.getByRole('dialog', { name: '编辑图文' })
    const contentInput = drawer.getByLabel('正文（HTML）')
    const saveButton = drawer.getByRole('button', { name: /保\s*存/ })
    const publishButton = drawer.getByRole('button', { name: '发布 / 存草稿箱' })
    await contentInput.fill('<p>手动保存中的正文</p>')
    await saveButton.click()
    await expect.poll(() => state.getSaveCount()).toBe(1)

    await expect(saveButton).toBeDisabled()
    await expect(publishButton).toBeDisabled()
    await expect(contentInput).toBeDisabled()
    await page.keyboard.press('Escape')
    await expect(drawer).toBeVisible()
    expect(state.getPublishCount()).toBe(0)

    state.releaseSave()
    await expect(page.getByText('已保存', { exact: true })).toBeVisible()
    await expect(saveButton).toBeEnabled()
    await expect(publishButton).toBeEnabled()
    await expect(contentInput).toBeEnabled()
  })

  test('生成封面请求在途时锁定保存、发布和编辑', async ({ page }) => {
    const state = await installArticleRoutes(page, { deferCover: true })

    await page.goto('/mp/articles')
    await expect(page.getByRole('cell', { name: article.title })).toBeVisible()
    await page.getByRole('button', { name: '编辑' }).click()

    const drawer = page.getByRole('dialog', { name: '编辑图文' })
    const contentInput = drawer.getByLabel('正文（HTML）')
    const saveButton = drawer.getByRole('button', { name: /保\s*存/ })
    const coverButton = drawer.getByRole('button', { name: 'AI 生成封面' })
    const publishButton = drawer.getByRole('button', { name: '发布 / 存草稿箱' })
    await coverButton.click()
    await expect.poll(() => state.getCoverCount()).toBe(1)

    await expect(saveButton).toBeDisabled()
    await expect(coverButton).toBeDisabled()
    await expect(publishButton).toBeDisabled()
    await expect(contentInput).toBeDisabled()
    await page.keyboard.press('Escape')
    await expect(drawer).toBeVisible()
    expect(state.getSaveCount()).toBe(0)
    expect(state.getPublishCount()).toBe(0)

    state.releaseCover()
    await expect(page.getByText('封面已生成', { exact: true })).toBeVisible()
    await expect(saveButton).toBeEnabled()
    await expect(coverButton).toBeEnabled()
    await expect(publishButton).toBeEnabled()
    await expect(contentInput).toBeEnabled()
  })
})
