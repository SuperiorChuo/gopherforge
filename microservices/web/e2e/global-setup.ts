import { chromium, type FullConfig } from '@playwright/test'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
// 复用冒烟库的验证码解码器与登录 body 构造，避免在浏览器里解验证码像素（不稳）。
// @ts-expect-error —— .mjs 无类型声明，运行期可用
import { buildConfig, decodeTextCaptchaCode, jsonObject } from '../../tests/api-smoke-lib.mjs'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
export const STORAGE_STATE = path.join(__dirname, '.auth', 'admin.json')

// 通过 API 登录拿到 access_token（验证码在 Node 侧解码），写进 storageState 的
// localStorage。后续测试直接以已登录态加载页面，不必每次走验证码表单。
async function apiLogin(baseURL: string): Promise<{ access: string; refresh: string }> {
  const cfg = buildConfig({ ...process.env, API_BASE_URL: `${baseURL}/api/v1` })
  const api = cfg.apiBaseUrl

  const capRes = await fetch(`${api}/captcha`)
  const cap = (await capRes.json()).data
  const code = decodeTextCaptchaCode(cap.image)

  const loginRes = await fetch(`${api}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: jsonObject({
      username: cfg.username,
      password: cfg.password,
      captcha_id: cap.key,
      captcha_code: code,
    }),
  })
  const body = await loginRes.json()
  if (body?.code !== 200 || !body?.data?.access_token) {
    throw new Error(`E2E login failed: HTTP ${loginRes.status} ${JSON.stringify(body).slice(0, 200)}`)
  }
  return { access: body.data.access_token, refresh: body.data.refresh_token }
}

export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0]?.use?.baseURL || process.env.E2E_BASE_URL || 'http://127.0.0.1:8000'
  const { access, refresh } = await apiLogin(baseURL)

  // 用一个浏览器上下文把 token 写进目标 origin 的 localStorage，再导出 storageState。
  const browser = await chromium.launch()
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  await page.goto(baseURL)
  await page.evaluate(
    ([a, r]) => {
      localStorage.setItem('access_token', a)
      localStorage.setItem('refresh_token', r)
    },
    [access, refresh],
  )
  await ctx.storageState({ path: STORAGE_STATE })
  await browser.close()
}
