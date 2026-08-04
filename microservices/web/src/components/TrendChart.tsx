import { useEffect, useMemo, useRef, useState } from 'react'

export interface TrendPoint {
  t: number
  value: number
}

interface TrendChartProps {
  points: TrendPoint[]
  height?: number
  unit?: string
  loading?: boolean
}

const PAD = { top: 14, right: 14, bottom: 26, left: 46 }

/**
 * Lightweight self-drawn SVG line chart for metric trends. No chart
 * dependency — theme-aware via the semantic CSS variables (--c-cyan line,
 * --text-tertiary labels, --card-bg tooltip).
 */
export default function TrendChart({ points, height = 168, unit = '', loading }: TrendChartProps) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)

  useEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const update = () => setWidth(el.clientWidth)
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const chart = useMemo(() => {
    if (!points.length || width <= 0) return null
    const values = points.map((p) => p.value)
    let min = Math.min(...values)
    let max = Math.max(...values)
    if (max === min) {
      // 平线也渲染在中间
      max += 1
      min -= 1
    }
    const span = max - min || 1
    const plotW = width - PAD.left - PAD.right
    const plotH = height - PAD.top - PAD.bottom
    const x = (i: number) => PAD.left + (i / (points.length - 1)) * plotW
    const y = (v: number) => PAD.top + (1 - (v - min) / span) * plotH
    const line = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(p.value).toFixed(1)}`).join(' ')
    const area = `${line} L${x(points.length - 1).toFixed(1)},${(height - PAD.bottom).toFixed(1)} L${PAD.left},${(height - PAD.bottom).toFixed(1)} Z`
    return { min, max, span, plotW, plotH, x, y, line, area, last: points[points.length - 1] }
  }, [points, width, height])

  const [hover, setHover] = useState<number | null>(null)

  const handleMove = (e: React.MouseEvent<SVGSVGElement>) => {
    if (!points.length) return
    const rect = e.currentTarget.getBoundingClientRect()
    const px = e.clientX - rect.left
    const i = Math.round(((px - PAD.left) / (rect.width - PAD.left - PAD.right)) * (points.length - 1))
    setHover(Math.max(0, Math.min(points.length - 1, i)))
  }

  if (loading) {
    return <div style={{ height, display: 'grid', placeItems: 'center' }} className="cell-muted">加载中…</div>
  }
  if (!chart || !points.length) {
    return <div style={{ height, display: 'grid', placeItems: 'center' }} className="cell-muted">暂无采样数据</div>
  }

  const gridY = [0, 0.5, 1]
  const hoverPoint = hover !== null ? points[hover] : null

  return (
    <div ref={wrapRef} style={{ width: '100%' }}>
      <svg
        width={width}
        height={height}
        onMouseMove={handleMove}
        onMouseLeave={() => setHover(null)}
        style={{ display: 'block' }}
      >
        {gridY.map((f) => {
          const gy = chart.y(chart.min + f * chart.span)
          const val = chart.min + f * chart.span
          return (
            <g key={f}>
              <line x1={PAD.left} x2={width - PAD.right} y1={gy} y2={gy} stroke="var(--c-border, rgba(148,163,184,.16))" strokeDasharray="3 3" />
              <text x={PAD.left - 6} y={gy + 4} textAnchor="end" fontSize={11} fill="var(--text-tertiary)">{fmtValue(val, unit)}</text>
            </g>
          )
        })}
        <path d={chart.area} fill="var(--c-cyan)" opacity={0.1} />
        <path d={chart.line} fill="none" stroke="var(--c-cyan)" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" />
        {hover !== null && hoverPoint && (
          <g>
            <line x1={chart.x(hover)} x2={chart.x(hover)} y1={PAD.top} y2={height - PAD.bottom} stroke="var(--text-tertiary)" strokeDasharray="2 3" />
            <circle cx={chart.x(hover)} cy={chart.y(hoverPoint.value)} r={4} fill="var(--c-cyan)" stroke="var(--app-bg)" strokeWidth={2} />
            <text x={chart.x(hover)} y={PAD.top - 4} textAnchor="middle" fontSize={11} fill="var(--text-secondary)">
              {fmtValue(hoverPoint.value, unit)} · {fmtTime(hoverPoint.t)}
            </text>
          </g>
        )}
      </svg>
    </div>
  )
}

function fmtValue(v: number, unit: string): string {
  const rounded = Math.abs(v) >= 1000 ? v.toFixed(0) : v.toFixed(1)
  return `${rounded}${unit ? ` ${unit}` : ''}`
}

function fmtTime(t: number): string {
  const d = new Date(t)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}
