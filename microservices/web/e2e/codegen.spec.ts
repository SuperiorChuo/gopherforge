import { expect, test, type Page, type Route } from '@playwright/test'
import { STORAGE_STATE } from './global-setup'

test.use({ storageState: STORAGE_STATE })

const ok = (data: unknown) => ({ code: 200, message: 'success', data })

async function fulfillJSON(route: Route, data: unknown) {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(ok(data)) })
}

type CodegenMockOptions = {
  writeEnabled?: boolean
  artifacts?: Array<Record<string, unknown>>
  diagnostics?: Array<Record<string, unknown>>
  onWrite?: (body: unknown) => void
  onDownload?: (body: unknown) => void
  staleDownload?: boolean
}

const readyArtifacts = [
  { path: 'microservices/services/system/internal/model/users.go', operation: 'create', content: 'package model', result_hash: 'hash-1', status: 'ready' },
  { path: 'microservices/web/src/api/users.ts', operation: 'create', content: 'export {}', result_hash: 'hash-2', status: 'ready' },
  { path: 'microservices/web/src/router/index.tsx', operation: 'patch', diff: '--- a/router\n+++ b/router', expected_hash: 'before', result_hash: 'after', status: 'ready' },
]

async function installCodegenMocks(page: Page, onPreview: (body: unknown) => void, options: CodegenMockOptions = {}) {
  await page.route('**/api/v1/codegen/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path.endsWith('/capabilities')) {
      return fulfillJSON(route, { preview_enabled: true, download_enabled: true, write_enabled: options.writeEnabled ?? false })
    }
    if (path.endsWith('/tables')) {
      return fulfillJSON(route, {
        list: [
          { name: 'users', comment: '用户', primary_key: 'id', column_count: 3, relation_count: 1 },
          { name: 'roles', comment: '角色', primary_key: 'id', column_count: 2, relation_count: 1 },
        ],
        total: 2,
      })
    }
    if (path.endsWith('/tables/users/schema')) {
      return fulfillJSON(route, {
        name: 'users', comment: '用户', primary_key: 'id', foreign_keys: [],
        columns: [
          { name: 'id', db_type: 'integer', go_type: 'int64', ts_type: 'number', nullable: false, primary_key: true, go_field: 'ID', label: 'ID', comment: '' },
          { name: 'username', db_type: 'text', go_type: 'string', ts_type: 'string', nullable: false, primary_key: false, go_field: 'Username', label: '用户名', comment: '' },
          { name: 'status', db_type: 'integer', go_type: 'int64', ts_type: 'number', nullable: false, primary_key: false, go_field: 'Status', label: '状态', comment: '' },
        ],
        relations: [{ kind: 'many_to_many', source_table: 'users', target_table: 'roles', join_table: 'user_roles', fk_field: 'user_id', target_fk: 'role_id' }],
      })
    }
    if (path.endsWith('/tables/roles/schema')) {
      return fulfillJSON(route, {
        name: 'roles', comment: '角色', primary_key: 'id', foreign_keys: [], relations: [],
        columns: [
          { name: 'id', db_type: 'integer', go_type: 'int64', ts_type: 'number', nullable: false, primary_key: true, go_field: 'ID', label: 'ID', comment: '' },
          { name: 'name', db_type: 'text', go_type: 'string', ts_type: 'string', nullable: false, primary_key: false, go_field: 'Name', label: '名称', comment: '' },
        ],
      })
    }
    if (path.endsWith('/preview')) {
      onPreview(request.postDataJSON())
      return fulfillJSON(route, {
        digest: 'preview-digest', request: request.postDataJSON(), schemas: [],
        diagnostics: options.diagnostics || [], artifacts: options.artifacts || readyArtifacts,
      })
    }
    if (path.endsWith('/write')) {
      options.onWrite?.(request.postDataJSON())
      return fulfillJSON(route, { digest: 'preview-digest', created: ['users.go'], patched: ['router.tsx'] })
    }
    if (path.endsWith('/download')) {
      options.onDownload?.(request.postDataJSON())
      if (options.staleDownload) {
        return route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ code: 409, message: '生成计划已变化' }) })
      }
      return route.fulfill({ status: 200, contentType: 'application/zip', body: 'PK mock zip' })
    }
    await route.fallback()
  })
  await page.route('**/api/v1/dict-types**', (route) => fulfillJSON(route, {
    list: [{ id: 1, name: '通用状态', code: 'sys_common_status', status: 1 }], total: 1,
  }))
}

test('字典和多对多配置进入预检请求', async ({ page }) => {
  let previewBody: Record<string, unknown> | undefined
  await installCodegenMocks(page, (body) => { previewBody = body as Record<string, unknown> })
  await page.goto('/system/codegen')

  await page.getByPlaceholder('搜索表名或注释').fill('users')
  await page.getByRole('button', { name: '配置' }).click()
  await page.getByLabel('status 字典').click()
  await page.getByText('通用状态 (sys_common_status)').click()
  await page.getByRole('button', { name: '添加多对多关系' }).click()
  await page.getByRole('button', { name: '下一步' }).click()
  await page.getByRole('button', { name: '预检代码' }).click()

  await expect.poll(() => Boolean(previewBody)).toBe(true)
  expect(previewBody?.table).toBe('users')
  expect(previewBody?.fields).toEqual(expect.arrayContaining([
    expect.objectContaining({ name: 'status', dict_type: 'sys_common_status' }),
  ]))
  expect(previewBody?.m2ms).toEqual(expect.arrayContaining([
    expect.objectContaining({ join_table: 'user_roles', target_table: 'roles', display_field: 'name' }),
  ]))
})

async function openPreparedPreview(page: Page) {
  await page.goto('/system/codegen')
  await page.getByPlaceholder('搜索表名或注释').fill('users')
  await page.getByRole('button', { name: '配置' }).click()
  await page.getByRole('button', { name: '下一步' }).click()
  await page.getByRole('button', { name: '预检代码' }).click()
}

test('分组展示计划并在生产能力下禁用直写', async ({ page }) => {
  await installCodegenMocks(page, () => {})
  await openPreparedPreview(page)
  await expect(page.getByRole('tab', { name: /新增文件 2/ })).toBeVisible()
  await expect(page.getByRole('tab', { name: /接入修改 1/ })).toBeVisible()
  await expect(page.getByRole('button', { name: '写入仓库' })).toBeDisabled()
  await expect(page.getByRole('button', { name: '下载 ZIP' })).toBeEnabled()
})

test('冲突计划禁用两种输出', async ({ page }) => {
  await installCodegenMocks(page, () => {}, {
    artifacts: [{ ...readyArtifacts[0], status: 'conflict' }],
    diagnostics: [{ severity: 'error', code: 'create_target_exists', message: '目标文件已存在', path: readyArtifacts[0].path }],
  })
  await openPreparedPreview(page)
  await expect(page.getByText('目标文件已存在')).toBeVisible()
  await expect(page.getByRole('button', { name: '下载 ZIP' })).toBeDisabled()
  await expect(page.getByRole('button', { name: '写入仓库' })).toBeDisabled()
})

test('直写确认必须等于模块名', async ({ page }) => {
  let writeBody: Record<string, unknown> | undefined
  await installCodegenMocks(page, () => {}, { writeEnabled: true, onWrite: (body) => { writeBody = body as Record<string, unknown> } })
  await openPreparedPreview(page)
  await page.getByRole('button', { name: '写入仓库' }).click()
  const confirmation = page.getByLabel('确认模块名')
  await confirmation.fill('wrong')
  await expect(page.getByRole('button', { name: '确认写入' })).toBeDisabled()
  await confirmation.fill('users')
  await page.getByRole('button', { name: '确认写入' }).click()
  await expect.poll(() => writeBody).toMatchObject({ expected_digest: 'preview-digest', confirmation: 'users' })
})

test('ZIP 下载复用预检摘要和模块文件名', async ({ page }) => {
  let downloadBody: Record<string, unknown> | undefined
  await installCodegenMocks(page, () => {}, { onDownload: (body) => { downloadBody = body as Record<string, unknown> } })
  await openPreparedPreview(page)
  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: '下载 ZIP' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toBe('codegen-users.zip')
  expect(downloadBody).toMatchObject({ expected_digest: 'preview-digest' })
})

test('下载摘要过期后丢弃旧计划', async ({ page }) => {
  await installCodegenMocks(page, () => {}, { staleDownload: true })
  await openPreparedPreview(page)
  await page.getByRole('button', { name: '下载 ZIP' }).click()
  await expect(page.getByRole('tab', { name: /新增文件 2/ })).not.toBeVisible()
  await expect(page.getByRole('button', { name: '预检代码' })).toBeVisible()
})
