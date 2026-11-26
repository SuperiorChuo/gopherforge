export type GeoRunEngine = 'chatgpt' | 'selfhost'

export const catLabel = (v: string) =>
  ({ category: '品类词', comparison: '对比题', scenario: '场景题', other: '其他' }[v] ?? v)

// hostnameOf 取域名用于来源展示，解析失败时退回原串（仓库无现成工具）。
export const hostnameOf = (raw: string) => {
  try {
    return new URL(raw).hostname.replace(/^www\./, '')
  } catch {
    return raw
  }
}
