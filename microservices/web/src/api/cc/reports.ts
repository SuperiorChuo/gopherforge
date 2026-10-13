import request from '@/utils/request'

// ---- 呼叫报表（纯 cc_calls 聚合，一个端点全返回）----

export type CcReportHourBucket = {
  hour: number
  inbound: number
  answered: number
}

export type CcReportAgentRow = {
  agent_ext: string
  answered: number
  total_talk_sec: number
  avg_talk_sec: number
}

export type CcReportQueueRow = {
  queue: string
  entered: number
  answered: number
  abandoned: number
}

export type CcReportOverview = {
  from: string
  to: string
  total_inbound: number
  total_outbound: number
  total: number
  answered: number
  answer_rate: number
  abandoned: number
  avg_talk_sec: number
  avg_wait_sec: number
  hour_buckets: CcReportHourBucket[]
  agent_leaderboard: CcReportAgentRow[]
  queue_leaderboard: CcReportQueueRow[]
}

// from/to 为 YYYY-MM-DD（含当天），不传默认今天
export function getCcReportOverview(params: { from?: string; to?: string }) {
  return request.get('/api/v1/cc/reports/overview', { params }) as Promise<CcReportOverview>
}
