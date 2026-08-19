import type { ReactNode } from 'react'
import './metrics.css'

export type StatsGridProps = {
  children: ReactNode
  className?: string
}

/** 统计指标布局：负责响应式换行，不承载任何业务统计语义。 */
export default function StatsGrid({ children, className }: StatsGridProps) {
  return <div className={['stats-grid', className].filter(Boolean).join(' ')}>{children}</div>
}

/** 统计指标之间的分隔线；窄屏由统一样式自动隐藏。 */
export function StatsGridDivider() {
  return <div className="stats-grid-divider" aria-hidden="true" />
}
