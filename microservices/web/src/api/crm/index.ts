import request from '@/utils/request'

import type { CrmTag } from './tag'
// ---------- 类型 ----------

export type CrmLead = {
  id: number
  name: string
  company: string
  phone: string
  email: string
  source: string
  status: 'new' | 'following' | 'converted' | 'invalid' | string
  owner_user_id: number
  department_id?: number
  im_visitor_id?: number
  note?: string
  customer_id?: number
  created_at: string
}

export type CrmCustomer = {
  id: number
  name: string
  industry: string
  level: string
  phone: string
  address: string
  source: string
  owner_user_id: number
  im_visitor_id?: number
  last_follow_at?: string
  pooled_at?: string
  note?: string
  tags?: CrmTag[]
  created_at: string
  // 卡片视图聚合（ListCustomerCards）
  opp_count?: number
  open_opp_count?: number
  contract_count?: number
  contract_cents?: number
}

export type CrmContact = {
  id: number
  customer_id: number
  name: string
  title: string
  phone: string
  email: string
  is_primary: boolean
}

export type CrmStage = 'initial' | 'demand' | 'proposal' | 'negotiation' | 'won' | 'lost'

export type CrmOpportunity = {
  id: number
  customer_id: number
  name: string
  amount_cents: number
  stage: CrmStage | string
  probability: number
  expected_close_at?: string
  owner_user_id: number
  lost_reason?: string
  won_at?: string
  lost_at?: string
  note?: string
  created_at: string
}

export type CrmStageLog = {
  id: number
  from_stage: string
  to_stage: string
  note?: string
  by_user_id: number
  created_at: string
}

export type CrmContract = {
  id: number
  no: string
  title: string
  customer_id: number
  opportunity_id?: number
  amount_cents: number
  status: 'draft' | 'active' | 'void' | string
  signed_at?: string
  start_at?: string
  end_at?: string
  note?: string
  created_at: string
  paid_cents?: number
}

export type CrmPayment = {
  id: number
  contract_id: number
  amount_cents: number
  paid_at: string
  method: string
  note?: string
  /** BPM 回款审批（M2）：审批中/已确认/已拒绝；空值=未走审批的存量，按已确认兼容展示 */
  status?: 'pending_approval' | 'confirmed' | 'rejected' | string
}

export type CrmFollowUp = {
  id: number
  target_type: string
  target_id: number
  content: string
  next_at?: string
  by_user_id: number
  created_at: string
}

export type CrmOwnerStat = {
  owner_user_id: number
  display_name?: string
  won_count: number
  won_amount_cents: number
  open_count: number
  contract_cents: number
  paid_cents: number
}

export type CrmStats = {
  funnel: { stage: string; count: number; amount_cents: number }[]
  new_leads: number
  converted_leads: number
  won_count: number
  won_amount_cents: number
  contract_cents: number
  paid_cents: number
  customer_count: number
  open_opportunity: number
  followups_overdue: number
  owners: CrmOwnerStat[]
  trend?: { month: string; leads: number; won: number; revenue: number }[]
}

export type CrmTimelineEvent = {
  type: string
  content: string
  by_user_name?: string
  created_at: string
}

export type CustomerDetail = {
  customer: CrmCustomer
  contacts: CrmContact[]
  opportunities: CrmOpportunity[]
  contracts: CrmContract[]
  follow_ups: CrmFollowUp[]
  tags: CrmTag[]
}

/** 来电弹屏号码识别结果（CC×CRM） */
export type PhoneLookup = {
  matched: 'customer' | 'lead' | 'none' | string
  customer?: CrmCustomer
  contact?: CrmContact
  lead?: CrmLead
}

type PageResult<T> = { list: T[]; total: number }

// ---------- 金额工具（后端存分） ----------

export const fenToYuan = (cents?: number) => ((cents ?? 0) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2 })
export const yuanToFen = (yuan?: number | null) => Math.round((yuan ?? 0) * 100)

// ---------- API ----------

export function listLeads(params: Record<string, unknown>) {
  return request.get('/api/v1/crm/leads', { params }) as Promise<PageResult<CrmLead>>
}
export function createLead(body: Partial<CrmLead>) {
  return request.post('/api/v1/crm/leads', body) as Promise<CrmLead>
}
export function updateLead(id: number, body: Partial<CrmLead>) {
  return request.put(`/api/v1/crm/leads/${id}`, body) as Promise<CrmLead>
}
export function convertLead(id: number, body: { opportunity_name?: string; amount_cents?: number }) {
  return request.post(`/api/v1/crm/leads/${id}/convert`, body) as Promise<{
    customer: CrmCustomer
    opportunity?: CrmOpportunity
  }>
}

export function listCustomers(params: Record<string, unknown>) {
  return request.get('/api/v1/crm/customers', { params }) as Promise<PageResult<CrmCustomer>>
}
/** 按当前筛选条件导出客户 CSV（UTF-8 BOM，Excel 可直接打开；同数据权限） */
export async function exportCustomers(params: Record<string, unknown>) {
  const blob = await request.get<unknown, Blob>('/api/v1/crm/customers/export', {
    params,
    responseType: 'blob',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `crm_customers_${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}
export function createCustomer(body: Partial<CrmCustomer>) {
  return request.post('/api/v1/crm/customers', body) as Promise<CrmCustomer>
}
export function updateCustomer(id: number, body: Partial<CrmCustomer>) {
  return request.put(`/api/v1/crm/customers/${id}`, body) as Promise<CrmCustomer>
}
export function getCustomerDetail(id: number) {
  return request.get(`/api/v1/crm/customers/${id}`) as Promise<CustomerDetail>
}

export function getCustomerTimeline(id: number) {
  return request.get(`/api/v1/crm/customers/${id}/timeline`) as Promise<{ events: CrmTimelineEvent[] }>
}
export function createContact(customerId: number, body: Partial<CrmContact>) {
  return request.post(`/api/v1/crm/customers/${customerId}/contacts`, body) as Promise<CrmContact>
}
export function deleteContact(id: number) {
  return request.delete(`/api/v1/crm/contacts/${id}`) as Promise<{ deleted: boolean }>
}

export function listOpportunities(params: Record<string, unknown>) {
  return request.get('/api/v1/crm/opportunities', { params }) as Promise<PageResult<CrmOpportunity>>
}
export function createOpportunity(body: Record<string, unknown>) {
  return request.post('/api/v1/crm/opportunities', body) as Promise<CrmOpportunity>
}
export function getOpportunity(id: number) {
  return request.get(`/api/v1/crm/opportunities/${id}`) as Promise<{
    opportunity: CrmOpportunity
    stage_logs: CrmStageLog[]
  }>
}
export function advanceStage(id: number, body: { stage: string; note?: string; lost_reason?: string }) {
  return request.post(`/api/v1/crm/opportunities/${id}/stage`, body) as Promise<CrmOpportunity>
}

export function listContracts(params: Record<string, unknown>) {
  return request.get('/api/v1/crm/contracts', { params }) as Promise<PageResult<CrmContract>>
}
export function createContract(body: Record<string, unknown>) {
  return request.post('/api/v1/crm/contracts', body) as Promise<CrmContract>
}
export function updateContract(id: number, body: Record<string, unknown>) {
  return request.put(`/api/v1/crm/contracts/${id}`, body) as Promise<CrmContract>
}
export function getContract(id: number) {
  return request.get(`/api/v1/crm/contracts/${id}`) as Promise<{
    contract: CrmContract
    payments: CrmPayment[]
    paid_cents: number
  }>
}
export function addPayment(contractId: number, body: Record<string, unknown>) {
  return request.post(`/api/v1/crm/contracts/${contractId}/payments`, body) as Promise<CrmPayment>
}

export function listFollowUps(targetType: string, targetId: number) {
  return request.get('/api/v1/crm/followups', {
    params: { target_type: targetType, target_id: targetId },
  }) as Promise<{ list: CrmFollowUp[] }>
}
export function createFollowUp(body: Record<string, unknown>) {
  return request.post('/api/v1/crm/followups', body) as Promise<CrmFollowUp>
}

/** 按号码识别 CRM 归属：客户 / 线索 / 无 */
export function lookupPhone(phone: string) {
  return request.get('/api/v1/crm/lookup', { params: { phone } }) as Promise<PhoneLookup>
}

export function getCrmStats(days = 30) {
  return request.get('/api/v1/crm/stats', { params: { days } }) as Promise<{ stats: CrmStats; days: number }>
}

// ---------- 展示辅助 ----------

export const STAGE_META: Record<string, { label: string; color: string }> = {
  initial: { label: '初步接洽', color: 'default' },
  demand: { label: '需求确认', color: 'blue' },
  proposal: { label: '方案报价', color: 'geekblue' },
  negotiation: { label: '商务谈判', color: 'purple' },
  won: { label: '赢单', color: 'success' },
  lost: { label: '输单', color: 'error' },
}

export const LEAD_STATUS_META: Record<string, { label: string; color: string }> = {
  new: { label: '未处理', color: 'orange' },
  following: { label: '跟进中', color: 'processing' },
  converted: { label: '已转化', color: 'success' },
  invalid: { label: '无效', color: 'default' },
}

export const CONTRACT_STATUS_META: Record<string, { label: string; color: string }> = {
  draft: { label: '草拟', color: 'default' },
  active: { label: '生效中', color: 'processing' },
  void: { label: '作废', color: 'error' },
}

/** 回款审批状态（BPM M2）；空值按 confirmed 兼容展示 */
export const PAYMENT_STATUS_META: Record<string, { label: string; color: string }> = {
  pending_approval: { label: '审批中', color: 'processing' },
  confirmed: { label: '已确认', color: 'success' },
  rejected: { label: '已拒绝', color: 'error' },
}

// ---------- P1 名片分配 / P2 公海引擎 ----------

export type OrgDept = { id: number; name: string }
export type OrgUser = { id: number; name: string; department_id: number }

export function getCrmOrg() {
  return request.get('/api/v1/crm/org') as Promise<{ departments: OrgDept[]; users: OrgUser[] }>
}

export function assignLeads(ids: number[], departmentId?: number, ownerUserId?: number) {
  return request.post('/api/v1/crm/leads/assign', {
    ids, department_id: departmentId || 0, owner_user_id: ownerUserId || 0,
  }) as Promise<{ assigned: number }>
}

export function claimLead(id: number) {
  return request.post(`/api/v1/crm/leads/${id}/claim`) as Promise<CrmLead>
}

export type CrmPoolRule = {
  level: string // * | A | B | C
  idle_days: number
  protect_active: boolean
  enabled: boolean
}

export function getPoolRules() {
  return request.get('/api/v1/crm/pool/rules') as Promise<{ rules: CrmPoolRule[] }>
}

export function savePoolRule(rule: CrmPoolRule) {
  return request.put('/api/v1/crm/pool/rules', rule) as Promise<CrmPoolRule>
}

export function claimCustomer(id: number) {
  return request.post(`/api/v1/crm/customers/${id}/claim`) as Promise<CrmCustomer>
}

export function releaseCustomer(id: number, note?: string) {
  return request.post(`/api/v1/crm/customers/${id}/release`, { note }) as Promise<{ released: boolean }>
}

export type CrmAssignLog = {
  id: number
  target_type: string
  target_id: number
  action: 'assign' | 'claim' | 'release' | 'recycle' | string
  from_owner: number
  to_owner: number
  to_dept: number
  by_user_id: number
  note?: string
  created_at: string
}

export function listAssignLogs(targetType: string, targetId: number) {
  return request.get('/api/v1/crm/assign-logs', {
    params: { target_type: targetType, target_id: targetId },
  }) as Promise<{ list: CrmAssignLog[] }>
}

// ---------- 公海规则页实时态 ----------

export type PoolPreviewRow = {
  id: number
  name: string
  level: string
  owner_user_id: number
  protected: boolean
}

export type PoolStats = {
  pool_count: number
  today_recycled: number
  preview_count: number
  preview: PoolPreviewRow[] | null
}

export function getPoolStats() {
  return request.get('/api/v1/crm/pool/stats') as Promise<PoolStats>
}

export function recyclePoolNow() {
  return request.post('/api/v1/crm/pool/recycle') as Promise<{ recycled: number; ids: number[] }>
}

export type CrmMyScope = {
  scope: 'all' | 'department_and_children' | 'self'
  visible_users: number
}

export function getMyScope() {
  return request.get('/api/v1/crm/my-scope') as Promise<CrmMyScope>
}

// ---------- 客户合并去重 ----------

export type CrmDupGroup = {
  key: string
  kind: 'phone' | 'name'
  customers: CrmCustomer[]
}

export function findDupGroups() {
  return request.get('/api/v1/crm/dup-groups') as Promise<{ groups: CrmDupGroup[] | null; total: number }>
}

export function mergeCustomers(primaryId: number, sourceIds: number[]) {
  return request.post('/api/v1/crm/customers/merge', {
    primary_id: primaryId, source_ids: sourceIds,
  }) as Promise<{ merged: number; moved: Record<string, number> }>
}
