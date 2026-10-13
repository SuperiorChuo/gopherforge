// 支付中心 M1 冒烟（照 cc-smoke 形态）：管理员登录 → mock 渠道建配 →
// 内部端点建单（幂等复建）→ 收银台匿名取单 → 拉起支付 → mock 支付成功 →
// 重复通知幂等 → 部分退款×2 → 超退被拒 → 关单校验终态锁定 → 清理。
// 需要 PAY_INTERNAL_TOKEN（从测试环境的环境变量读取，不硬编码）。
// 用法：API_BASE_URL=http://<host>:18100/api/v1 PAY_INTERNAL_TOKEN=xxx npm run smoke:pay
import { buildConfig, decodeTextCaptchaCode, jsonObject, getJsonPath } from './api-smoke-lib.mjs'

const config = buildConfig(process.env)
const base = config.apiBaseUrl
const internalToken = (process.env.PAY_INTERNAL_TOKEN || '').trim()
const runId = config.safeRunId

let step = ''
const log = (s) => { step = s; console.log(`[pay-smoke] ${s}`) }
const die = (msg, extra) => {
  console.error(`\n[pay-smoke] FAILED at "${step}": ${msg}`)
  if (extra !== undefined) console.error(JSON.stringify(extra, null, 2).slice(0, 800))
  process.exit(1)
}
const expect = (cond, msg, extra) => { if (!cond) die(msg, extra) }

async function req(method, path, { token, internal, body, expected = 200 } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  if (token) headers.Authorization = `Bearer ${token}`
  if (internal) headers['X-Internal-Token'] = internalToken
  const res = await fetch(`${base}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : jsonObject(body),
  })
  const json = await res.json().catch(() => ({}))
  if (expected !== '*') expect(res.status === expected, `${method} ${path} → ${res.status}（期望 ${expected}）`, json)
  return { status: res.status, json }
}

async function main() {
  expect(internalToken !== '', '缺 PAY_INTERNAL_TOKEN 环境变量')

  log('登录')
  const cap = await req('GET', '/captcha')
  const code = decodeTextCaptchaCode(cap.json.data.image)
  const login = await req('POST', '/login', {
    body: { username: config.username, password: config.password, captcha_id: cap.json.data.key, captcha_code: code },
  })
  const token = getJsonPath(login.json, 'data.access_token')
  expect(typeof token === 'string', '登录未返回 token', login.json)

  // 本冒烟全程依赖 mock 渠道，只能跑在演示/联调环境（PAY_DEMO_MODE 开）。
  // 生产环境 mock 未注册，这里直接给出可读结论，而不是在后面撞"未知渠道类型"。
  log('确认 mock 渠道适配器已注册')
  const codes = await req('GET', '/pay/channel-codes', { token })
  expect(
    (codes.json.data || []).includes('mock'),
    'mock 渠道未注册：本冒烟依赖 mock 渠道，目标环境需 PAY_DEMO_MODE=true（生产默认关闭，收银台模拟支付端点不存在）',
    codes.json,
  )

  log('确保 mock 渠道存在且启用')
  const chs = await req('GET', '/pay/channels', { token })
  let mock = (chs.json.data || []).find((c) => c.code === 'mock')
  if (!mock) {
    const created = await req('POST', '/pay/channels', { token, body: { code: 'mock', name: '冒烟Mock渠道', status: 1 } })
    mock = created.json.data
  }
  expect(mock && mock.status === 1, 'mock 渠道不可用', mock)

  log('内部端点建单')
  const bizId = `smoke-${runId}`
  const createBody = { biz_type: 'smoke_demo', biz_id: bizId, subject: '支付冒烟订单', amount_cents: 990 }
  const order = await req('POST', '/pay/internal/orders', { internal: true, body: createBody })
  const orderNo = getJsonPath(order.json, 'data.order_no')
  const cashierPath = getJsonPath(order.json, 'data.cashier_path')
  expect(orderNo && cashierPath, '建单未返回订单号/收银台路径', order.json)
  const cashierToken = cashierPath.split('/').pop()

  log('建单幂等（同 biz 返回同单）')
  const dup = await req('POST', '/pay/internal/orders', { internal: true, body: createBody })
  expect(getJsonPath(dup.json, 'data.order_no') === orderNo, '幂等建单返回了不同订单', dup.json)

  log('内部端点鉴权（坏 token 401）')
  {
    const res = await fetch(`${base}/pay/internal/orders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Internal-Token': 'bad-token' },
      body: jsonObject(createBody),
    })
    expect(res.status === 401, `坏 internal token 应 401，得 ${res.status}`)
  }

  log('收银台匿名取单')
  const cashier = await req('GET', `/pay/cashier/${cashierToken}`)
  expect(Number(getJsonPath(cashier.json, 'data.amount_cents')) === 990, '收银台金额不符', cashier.json)
  expect((cashier.json.data.channels || []).some((c) => c.code === 'mock'), '收银台未见 mock 渠道', cashier.json)
  // 前端「模拟支付」按钮只认这个字段（生产为 false，按钮与端点一起消失）
  expect(cashier.json.data.mock_enabled === true, '收银台未返回 mock_enabled=true', cashier.json)

  log('拉起支付（mock）')
  const pay = await req('POST', `/pay/cashier/${cashierToken}/pay`, { body: { channel_code: 'mock' } })
  expect(getJsonPath(pay.json, 'data.status') === 'paying', '拉起后应为 paying', pay.json)

  log('mock 支付成功 → paid')
  const paid = await req('POST', `/pay/cashier/${cashierToken}/mock`, { body: { succeed: true } })
  expect(getJsonPath(paid.json, 'data.status') === 'paid', 'mock 成功后应 paid', paid.json)

  log('重复通知幂等（仍 paid 且 200）')
  const dup2 = await req('POST', `/pay/cashier/${cashierToken}/mock`, { body: { succeed: true } })
  expect(getJsonPath(dup2.json, 'data.status') === 'paid', '重复通知破坏了状态', dup2.json)

  log('管理面可见订单与回调日志')
  const detail = await req('GET', `/pay/orders/${orderNo}`, { token })
  expect(getJsonPath(detail.json, 'data.status') === 'paid', '管理面订单状态不符', detail.json)
  const logs = await req('GET', '/pay/notify-logs', { token })
  expect((logs.json.data || []).some((l) => l.order_no === orderNo && l.result === 'applied'), '回调日志缺 applied 记录')

  log('部分退款 500')
  const r1 = await req('POST', '/pay/refunds', { token, body: { order_no: orderNo, amount_cents: 500, reason: '冒烟部分退' } })
  expect(getJsonPath(r1.json, 'data.status') === 'refunded', 'mock 退款应立即 refunded', r1.json)

  log('再退 490 恰好退满')
  const r2 = await req('POST', '/pay/refunds', { token, body: { order_no: orderNo, amount_cents: 490, reason: '冒烟退尾款' } })
  expect(getJsonPath(r2.json, 'data.status') === 'refunded', '二次退款失败', r2.json)

  log('超退 1 分被拒（400）')
  await req('POST', '/pay/refunds', { token, body: { order_no: orderNo, amount_cents: 1, reason: '超退' }, expected: 400 })

  log('终态锁定：paid 单关单被拒（400）')
  await req('POST', `/pay/orders/${orderNo}/close`, { token, expected: 400 })

  log('统计口径')
  const stats = await req('GET', '/pay/stats', { token })
  expect((stats.json.data?.paid ?? 0) >= 1, '统计缺已支付单', stats.json)

  log('清理冒烟数据（订单/退款/日志按 biz 前缀删除）')
  // 冒烟数据保留审计价值有限，直接物理清理，保证可重复执行
  // （管理面无删单接口——钱的记录不许删，这里走 internal 反查确认后由 DB 侧清理属运维行为；
  //   为保持纯 API 冒烟，此处只把 biz_id 加时间戳保证下轮不撞幂等锚点，不做删除。）
  console.log(`[pay-smoke] ok: 支付中心冒烟通过（${base}）；订单 ${orderNo} 留档（paid+全额退款）`)
}

main().catch((e) => die(e?.message || String(e)))
