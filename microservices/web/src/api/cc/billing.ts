import request from '@/utils/request'

// ---- 计费：费率 / 账户余额 / 话费流水（金额字段一律「分」，展示除 100）----

export type CcRate = {
  id: number
  tenant_id: number
  prefix: string
  name: string
  rate_per_min: number // 每分钟费率（分）
  billing_step: number // 计费步长（秒）
  connect_fee: number // 接通费（分）
  created_at: string
  updated_at: string
}

export type CcRatePayload = {
  prefix: string
  name?: string
  rate_per_min: number // 分
  billing_step: number // 秒
  connect_fee: number // 分
}

export type CcBillingAccount = {
  id: number
  tenant_id: number
  balance: number // 余额（分，可负）
  credit_limit: number // 透支额度（分）
  status: 'normal' | 'frozen'
  created_at: string
  updated_at: string
}

export type CcAccountResult = {
  account: CcBillingAccount
  month_spent: number // 本月消费合计（分）
  today_spent: number // 今日消费合计（分）
}

export type CcCallCharge = {
  id: number
  tenant_id: number
  call_id: string
  caller: string
  callee: string
  bill_sec: number
  rate_id: number
  amount: number // 本次扣费（分）
  balance_after: number // 扣费后余额（分）
  created_at: string
}

export type CcChargeListResult = {
  list: CcCallCharge[]
  total: number
  page: number
  page_size: number
}

export function listCcRates() {
  return request.get('/api/v1/cc/billing/rates') as Promise<{ list: CcRate[] }>
}

export function upsertCcRate(data: CcRatePayload) {
  return request.post('/api/v1/cc/billing/rates', data) as Promise<CcRate>
}

export function deleteCcRate(id: number) {
  return request.delete(`/api/v1/cc/billing/rates/${id}`) as Promise<{ deleted: boolean }>
}

export function getCcAccount() {
  return request.get('/api/v1/cc/billing/account') as Promise<CcAccountResult>
}

// amountFen 正=充值，负=调减（分）；remark 留痕备注进资金流水
export function rechargeCcAccount(amountFen: number, remark?: string) {
  return request.post('/api/v1/cc/billing/recharge', { amount_fen: amountFen, remark }) as Promise<CcBillingAccount>
}

// ---- 资金流水（充值/调减审计） ----

export type CcAccountTxn = {
  id: number
  tenant_id: number
  kind: 'recharge' | 'adjust'
  amount: number // 有符号（分）：正=入账、负=调减
  balance_after: number
  operator_id: number
  remark: string
  created_at: string
}

export function listCcAccountTxns(params: { page: number; page_size: number }) {
  return request.get('/api/v1/cc/billing/txns', { params }) as Promise<{
    list: CcAccountTxn[]
    total: number
  }>
}

// ---- 月账单 ----

export type CcBill = {
  id: number
  tenant_id: number
  period: string // YYYY-MM
  call_count: number
  total_bill_sec: number
  total_amount: number // 分
  generated_at: string
}

// 平台租户返回全部账单，其他租户只返回自己的
export function listCcBills(period?: string) {
  return request.get('/api/v1/cc/billing/bills', { params: { period } }) as Promise<{
    list: CcBill[]
    total: number
  }>
}

/** 下载账单明细 CSV（UTF-8 BOM，Excel 可直接打开；照 CRM 导出手法走 blob） */
export async function exportCcBill(bill: CcBill) {
  const blob = await request.get<unknown, Blob>(`/api/v1/cc/billing/bills/${bill.id}/export`, {
    responseType: 'blob',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `cc-bill-${bill.tenant_id}-${bill.period}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

// ---- 平台侧账务（仅 tenant=1，其他租户 403） ----

export type CcAccountOverviewRow = CcBillingAccount & {
  month_spent: number // 分
  today_spent: number // 分
}

export function listCcAccountsOverview() {
  return request.get('/api/v1/cc/billing/admin/overview') as Promise<{
    list: CcAccountOverviewRow[]
    total: number
  }>
}

export function adminRechargeCcAccount(tenantId: number, amountFen: number, remark?: string) {
  return request.post('/api/v1/cc/billing/admin/recharge', {
    tenant_id: tenantId, amount_fen: amountFen, remark,
  }) as Promise<CcBillingAccount>
}

export function adminSetCcAccountStatus(tenantId: number, status: 'normal' | 'frozen') {
  return request.post('/api/v1/cc/billing/admin/status', {
    tenant_id: tenantId, status,
  }) as Promise<CcBillingAccount>
}

export function adminGenerateCcBills(period: string) {
  return request.post('/api/v1/cc/billing/admin/bills/generate', { period }) as Promise<{
    period: string
    generated: number
  }>
}

export function listCcCharges(params: {
  page: number
  page_size: number
  from?: string
  to?: string
}) {
  return request.get('/api/v1/cc/billing/charges', { params }) as Promise<CcChargeListResult>
}

// 分转元字符串（2 位小数），分金额一律用它展示
export const ccFenToYuan = (fen?: number) =>
  ((fen ?? 0) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

// 元转分（四舍五入到整数分）
export const ccYuanToFen = (yuan?: number | null) => Math.round((yuan ?? 0) * 100)
