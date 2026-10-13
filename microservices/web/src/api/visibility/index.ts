import request from '@/utils/request'

export type VisBrand = {
  id: number
  name: string
  aliases?: string
  kind: 'brand' | 'competitor' | string
  website?: string
  note?: string
  status: number
  created_at?: string
}

export type VisKeyword = {
  id: number
  keyword: string
  locale?: string
  target_url?: string
  rank: number
  prev_rank: number
  search_volume: number
  status: string
  note?: string
  checked_at?: string
  bing_rank: number
  bing_prev_rank: number
  baidu_rank: number
  baidu_prev_rank: number
  google_rank: number
  google_prev_rank: number
  // 扩展引擎排名 JSON 串：{"sogou":{"rank":3,"prev_rank":5},...}
  extra_ranks?: string
  serp_checked_at?: string
  created_at?: string
}

export type VisGeoPrompt = {
  id: number
  category: string
  question: string
  engine: string
  status: number
  created_at?: string
}

export type VisAdviceItem = {
  id: string
  title: string
  detail?: string
  priority?: string
  related?: string
  done?: boolean
}

export type VisGeoRun = {
  id: number
  mode: string
  engine?: string // chatgpt | selfhost | stub；老 run 为空视为 selfhost
  status: string
  prompt_count: number
  done_count: number
  mention_count: number
  note?: string
  error?: string
  advice?: string
  advice_items_json?: string
  finished_at?: string
  created_at: string
}

export type VisGeoCompare = {
  current_id: number
  previous_id: number
  current_rate: number
  previous_rate: number
  rate_delta: number
  current_mentions: number
  previous_mentions: number
  mention_delta: number
  current_prompts: number
  previous_prompts: number
  newly_mentioned: string[]
  newly_missed: string[]
  still_missing: string[]
  still_mentioned: number
}

export type VisGeoSource = {
  position: number
  title?: string
  url: string
  cited: boolean
  own_domain: boolean
}

export type VisGeoResult = {
  id: number
  run_id: number
  prompt_id: number
  question: string
  answer: string
  mention_brand: boolean
  mention_competitor: boolean
  recommended: boolean
  has_link: boolean
  matched_brands?: string
  engine?: string
  sentiment?: string
  list_position: number
  analyze_by?: string
  sources?: VisGeoSource[]
  created_at?: string
}

export type VisSerpTask = {
  id: number
  trigger: string
  keyword_id: number
  status: string
  total: number
  done_count: number
  ok_count: number
  fail_count: number
  error?: string
  finished_at?: string
  created_at: string
}

export type VisRankSnapshot = {
  id: number
  keyword_id: number
  engine: string
  rank: number
  serp_url?: string
  error?: string
  task_id: number
  created_at: string
}

export type VisOverview = {
  brand_count: number
  competitor_count: number
  keyword_count: number
  keyword_top10: number
  keyword_no_rank: number
  bing_top10: number
  baidu_top10: number
  google_top10: number
  geo_prompt_count: number
  geo_run_count: number
  geo_result_count: number
  geo_mention_count: number
  geo_mention_rate: number
  recent_runs: number
}

type PageResult<T> = { list: T[]; total: number }

export function getVisibilityOverview(days = 30) {
  return request.get('/api/v1/visibility/overview', { params: { days } }) as Promise<{
    stats: VisOverview
    days: number
    // demo_mode 后端是否允许演示假数据（VISIBILITY_DEMO_MODE，生产默认关）。
    // 演示入口一律以它为准——生产下 seed-demo 路由根本不存在。
    demo_mode?: boolean
  }>
}

export function seedVisibilityDemo() {
  return request.post('/api/v1/visibility/seed-demo', {}) as Promise<{ seeded: boolean }>
}

export function listBrands(params?: { kind?: string; page?: number; page_size?: number }) {
  return request.get('/api/v1/visibility/brands', { params }) as Promise<PageResult<VisBrand>>
}

export function createBrand(body: Partial<VisBrand>) {
  return request.post('/api/v1/visibility/brands', body) as Promise<VisBrand>
}

export function updateBrand(id: number, body: Partial<VisBrand>) {
  return request.put(`/api/v1/visibility/brands/${id}`, body) as Promise<VisBrand>
}

export function deleteBrand(id: number) {
  return request.delete(`/api/v1/visibility/brands/${id}`) as Promise<{ deleted: boolean }>
}

export type BrandCandidate = {
  name: string
  aliases?: string
  kind: 'brand' | 'competitor' | string
  website?: string
}

export function generateBrands(body?: { site_url?: string; hint?: string }) {
  return request.post('/api/v1/visibility/brands/generate', body || {}, { timeout: 90_000 }) as Promise<{
    items: BrandCandidate[]
    site_fetched?: boolean
  }>
}

export function batchCreateBrands(items: BrandCandidate[]) {
  return request.post('/api/v1/visibility/brands/batch', { items }) as Promise<{ created: number; skipped: number }>
}

export function listKeywords(params?: { status?: string; keyword?: string; page?: number; page_size?: number }) {
  return request.get('/api/v1/visibility/keywords', { params }) as Promise<PageResult<VisKeyword>>
}

export function createKeyword(body: Partial<VisKeyword>) {
  return request.post('/api/v1/visibility/keywords', body) as Promise<VisKeyword>
}

export function updateKeyword(id: number, body: Partial<VisKeyword>) {
  return request.put(`/api/v1/visibility/keywords/${id}`, body) as Promise<{ updated: boolean }>
}

export function deleteKeyword(id: number) {
  return request.delete(`/api/v1/visibility/keywords/${id}`) as Promise<{ deleted: boolean }>
}

export type KeywordCandidate = {
  keyword: string
  locale: string
  target_url?: string
}

export function generateKeywords(body?: { site_url?: string; hint?: string; count?: number }) {
  return request.post('/api/v1/visibility/keywords/generate', body || {}, { timeout: 90_000 }) as Promise<{
    items: KeywordCandidate[]
    site_fetched?: boolean
  }>
}

export function batchCreateKeywords(items: KeywordCandidate[]) {
  return request.post('/api/v1/visibility/keywords/batch', { items }) as Promise<{ created: number; skipped: number }>
}

export function listGeoPrompts(params?: { page?: number; page_size?: number }) {
  return request.get('/api/v1/visibility/geo/prompts', { params }) as Promise<PageResult<VisGeoPrompt>>
}

export function createGeoPrompt(body: Partial<VisGeoPrompt>) {
  return request.post('/api/v1/visibility/geo/prompts', body) as Promise<VisGeoPrompt>
}

export function updateGeoPrompt(id: number, body: Partial<VisGeoPrompt>) {
  return request.put(`/api/v1/visibility/geo/prompts/${id}`, body) as Promise<{ updated: boolean }>
}

export function deleteGeoPrompt(id: number) {
  return request.delete(`/api/v1/visibility/geo/prompts/${id}`) as Promise<{ deleted: boolean }>
}

export type GeneratedPrompt = { category: string; question: string }

export type SuggestedBrand = { name: string; aliases?: string }

export function generateGeoPrompts(body?: { site_url?: string; hint?: string; count?: number }) {
  return request.post('/api/v1/visibility/geo/prompts/generate', body || {}, { timeout: 90_000 }) as Promise<{
    items: GeneratedPrompt[]
    suggested_brand?: SuggestedBrand | null
    site_fetched?: boolean
  }>
}

export function batchCreateGeoPrompts(items: GeneratedPrompt[]) {
  return request.post('/api/v1/visibility/geo/prompts/batch', { items }) as Promise<{ created: number; skipped: number }>
}

export function listGeoRuns(params?: { page?: number; page_size?: number }) {
  // demo_mode：后端是否接受 mode=stub 的模拟检测，决定要不要渲染「模拟演示」按钮。
  return request.get('/api/v1/visibility/geo/runs', { params }) as Promise<
    PageResult<VisGeoRun> & { demo_mode?: boolean }
  >
}

export function createGeoRun(body?: { mode?: 'stub' | 'llm'; engine?: 'chatgpt' | 'selfhost' | 'stub'; note?: string }) {
  return request.post('/api/v1/visibility/geo/runs', body || {}) as Promise<{
    run: VisGeoRun
    reused?: boolean
  }>
}

export function getGeoRun(id: number) {
  return request.get(`/api/v1/visibility/geo/runs/${id}`) as Promise<{
    run: VisGeoRun
    results: VisGeoResult[]
    advice_items?: VisAdviceItem[]
  }>
}

export type GeoLeaderboardItem = { name: string; kind: string; mentions: number }

export type GeoLeaderboard = {
  runs_used: number
  mode: string
  total_questions: number
  items: GeoLeaderboardItem[]
  own_name: string
  own_rank: number
  own_mentions: number
  mention_rate: number
  avg_position: number
  best_position: number
}

export function getGeoLeaderboard() {
  return request.get('/api/v1/visibility/geo/leaderboard') as Promise<GeoLeaderboard>
}

export type NamingCollision = { name: string; risk: 'high' | 'medium' | 'low' | string; reason: string }

export type NamingSuggestion = { name: string; reason: string }

export type NamingAnalysis = {
  summary: string
  collisions?: NamingCollision[] | null
  suggestions?: NamingSuggestion[] | null
}

export function analyzeBrandNaming() {
  // 长文分析，后端上限 150 秒
  return request.post('/api/v1/visibility/brands/name-analysis', {}, { timeout: 160_000 }) as Promise<NamingAnalysis>
}

export function generateGeoAdvice(runId: number) {
  // 长文生成，后端上限 3 分钟
  return request.post(`/api/v1/visibility/geo/runs/${runId}/advice`, {}, { timeout: 190_000 }) as Promise<{
    advice: string
    advice_items?: VisAdviceItem[]
  }>
}

export function saveGeoAdviceItems(runId: number, items: VisAdviceItem[]) {
  return request.put(`/api/v1/visibility/geo/runs/${runId}/advice-items`, { items }) as Promise<{
    advice_items: VisAdviceItem[]
  }>
}

export function compareGeoRun(runId: number, withId?: number) {
  return request.get(`/api/v1/visibility/geo/runs/${runId}/compare`, {
    params: withId ? { with: withId } : undefined,
  }) as Promise<{
    has_previous: boolean
    compare: VisGeoCompare | null
    previous?: VisGeoRun
  }>
}

export function createSerpCheck(body?: { keyword_id?: number }) {
  return request.post('/api/v1/visibility/serp/checks', body || {}) as Promise<{
    task: VisSerpTask
    reused?: boolean
  }>
}

export function getSerpTask(id: number) {
  return request.get(`/api/v1/visibility/serp/checks/${id}`) as Promise<{ task: VisSerpTask }>
}

export function listSerpTasks(params?: { page?: number; page_size?: number }) {
  return request.get('/api/v1/visibility/serp/checks', { params }) as Promise<PageResult<VisSerpTask>>
}

export function getKeywordHistory(id: number, params?: { engine?: string; days?: number }) {
  return request.get(`/api/v1/visibility/keywords/${id}/history`, { params }) as Promise<{
    list: VisRankSnapshot[]
  }>
}
