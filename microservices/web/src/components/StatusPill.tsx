// 玻璃状态胶囊：呼吸点 + 文字，取代列表页里千篇一律的绿 Tag。
// tone 决定点和文字的颜色语义；on=true 时点会呼吸(活性)。
export type StatusTone = 'success' | 'muted' | 'danger' | 'info' | 'warning'

interface StatusPillProps {
  tone: StatusTone
  label: string
  /** 呼吸动画，默认 tone==='success' 时开 */
  pulse?: boolean
}

export default function StatusPill({ tone, label, pulse }: StatusPillProps) {
  const active = pulse ?? tone === 'success'
  return (
    <span className={`status-pill status-pill-${tone}`}>
      <span className={`status-pill-dot ${active ? 'status-pill-dot-pulse' : ''}`} />
      {label}
    </span>
  )
}

/** 最常见的 启用(1)/禁用(0) 二态 */
export function EnableStatusPill({ value }: { value: number }) {
  return value === 1 ? (
    <StatusPill tone="success" label="启用" />
  ) : (
    <StatusPill tone="muted" label="禁用" />
  )
}
