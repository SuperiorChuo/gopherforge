import { Card, Progress } from 'antd'
import { useThemeMode } from '@/theme/ThemeContext'

// 用量三档色,亮色主题用更深一号保证对比度
const USAGE_COLORS = {
  dark: { high: '#f87171', mid: '#fbbf24', low: '#818cf8' },
  light: { high: '#dc2626', mid: '#d97706', low: '#2563eb' },
}

// 玻璃透光色:同色但极低透明度,由 .stat-card 的 --tint 消费
const USAGE_TINTS = {
  high: 'rgba(248, 113, 113, 0.14)',
  mid: 'rgba(251, 191, 36, 0.12)',
  low: 'rgba(99, 102, 241, 0.12)',
}

function usageLevel(pct: number): 'high' | 'mid' | 'low' {
  return pct >= 90 ? 'high' : pct >= 70 ? 'mid' : 'low'
}

interface MonitorGaugeCardProps {
  title: string
  icon: React.ReactNode
  percent: number
  footer: React.ReactNode
  index: number
  /** 用量分档阈值,默认 90/70;连接数等敏感指标可传更低阈值 */
  level?: 'high' | 'mid' | 'low'
}

export default function MonitorGaugeCard({ title, icon, percent, footer, index, level }: MonitorGaugeCardProps) {
  const { mode } = useThemeMode()
  const pct = Math.round(percent)
  const lv = level ?? usageLevel(pct)
  const color = USAGE_COLORS[mode][lv]
  return (
    <Card
      className="monitor-gauge-card stat-card glass-rise"
      style={{ '--tint': USAGE_TINTS[lv], '--i': index } as React.CSSProperties}
    >
      <div className="monitor-gauge-head">
        <span className="monitor-gauge-icon" style={{ color }}>
          {icon}
        </span>
        <span className="monitor-gauge-title">{title}</span>
      </div>
      <div className="monitor-gauge-body">
        {/* 表盘背后同色辉光,玻璃内侧被仪表照亮的感觉 */}
        <div className="monitor-gauge-halo" style={{ '--halo': color } as React.CSSProperties}>
          <Progress
            type="dashboard"
            percent={pct}
            strokeColor={color}
            size={140}
            format={(p) => (
              <span className="monitor-gauge-value" style={{ color }}>{p}%</span>
            )}
          />
        </div>
      </div>
      <div className="monitor-gauge-foot">{footer}</div>
    </Card>
  )
}
