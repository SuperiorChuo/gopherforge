import type { ReactNode } from 'react'

export type MetricCardProps = {
  label: ReactNode
  value: ReactNode
  className?: string
  valueClassName?: string
}

/** 单个统计指标：统一标签/数值语义，数值内容由业务页决定。 */
export default function MetricCard({ label, value, className, valueClassName }: MetricCardProps) {
  return (
    <div className={['metric-card', className].filter(Boolean).join(' ')}>
      <span className="metric-card-label">{label}</span>
      <span className={['metric-card-value', valueClassName].filter(Boolean).join(' ')}>{value}</span>
    </div>
  )
}
