import { memo, useCallback, useMemo, useRef, useState } from 'react'
import { normalizeProvince } from '@/utils/chinaGeo'
import chinaRaw from '@/assets/china-provinces.json'
import worldRaw from '@/assets/world-countries.json'

// 深空风格纯 SVG 世界地图：全球国家版图打底（太平洋居中，中国在视觉中心）+
// 中国省界精细层（省级热度染色）+ 城市涟漪光点 + 汇聚枢纽的飞线动画。
// 不依赖外部瓦片（内网可用）、不依赖 maplibre（其 GeoJSON worker 管线在本
// 项目构建下静默失败，已踩坑）。视口按数据落点自适应：只有国内登录时贴合
// 中国，出现海外来源时自动拉远到对应大洲。
// 图层拆成 memo 子组件：tooltip 随 mousemove 高频更新时，34 条巨型省界
// path 与全部光点不参与重渲染。

export interface GeoMapPoint {
  name: string
  lng: number
  lat: number
  total: number
  failed: number
  abroad?: boolean
}

interface GeoMapProps {
  points: GeoMapPoint[]
  // 省级登录量（key 为归一化短名：广东/北京），用于版图热度染色
  provinceTotals?: Record<string, number>
  height?: number
}

interface GeoFeature {
  properties: { name: string }
  geometry: { coordinates: [number, number][][][] }
}

// ---- 投影：太平洋居中 Web Mercator，10 单位/经度 ----
const RAD = Math.PI / 180
// 世界数据已把美洲平移到 [180,330]；运行时坐标（点）在标准 [-180,180]，同规则平移
const shiftLng = (lng: number) => (lng < -30 ? lng + 360 : lng)
const mercDeg = (lat: number) => Math.log(Math.tan(Math.PI / 4 + (lat * RAD) / 2)) / RAD
const TOP_LAT = 74
const px = (lng: number) => (shiftLng(lng) + 30) * 10
const py = (lat: number) => (mercDeg(TOP_LAT) - mercDeg(lat)) * 10

function toPaths(features: GeoFeature[]) {
  return features.map((f) => ({
    name: f.properties.name,
    short: normalizeProvince(f.properties.name),
    d: f.geometry.coordinates
      .map((poly) =>
        poly
          .map(
            (ring) =>
              'M' + ring.map(([lng, lat]) => `${px(lng).toFixed(1)} ${py(lat).toFixed(1)}`).join('L') + 'Z',
          )
          .join(''),
      )
      .join(''),
  }))
}

const WORLD = toPaths((worldRaw as unknown as { features: GeoFeature[] }).features)
const CHINA = toPaths((chinaRaw as unknown as { features: GeoFeature[] }).features)

// 中国 bbox（viewBox 基线：无海外来源时视口就是它；17.5°N 截断南海远礁避免大片空海）
const CHINA_BOUNDS = { minX: px(73), maxX: px(135.2), minY: py(53.7), maxY: py(17.5) }

type TipHandler = (e: React.MouseEvent, title: string, detail: string) => void

const WorldLayer = memo(function WorldLayer() {
  return (
    <g>
      {WORLD.map((c) => (
        <path key={c.name} d={c.d} className="geo-country" />
      ))}
    </g>
  )
})

const ChinaLayer = memo(function ChinaLayer({
  provinceTotals,
  maxHeat,
  onTip,
  onTipHide,
}: {
  provinceTotals?: Record<string, number>
  maxHeat: number
  onTip: TipHandler
  onTipHide: () => void
}) {
  return (
    <g>
      {CHINA.map((p) => {
        const heat = provinceTotals?.[p.short] ?? 0
        return (
          <path
            key={p.name}
            d={p.d}
            className="geo-province"
            style={
              heat > 0
                ? ({ '--heat': 0.06 + 0.3 * Math.sqrt(heat / maxHeat) } as React.CSSProperties)
                : undefined
            }
            onMouseMove={(e) => onTip(e, p.name, heat > 0 ? `${heat} 次登录` : '暂无登录')}
            onMouseLeave={onTipHide}
          />
        )
      })}
    </g>
  )
})

const FlightsLayer = memo(function FlightsLayer({
  flights,
  k,
}: {
  flights: { id: string; d: string; abroad: boolean }[]
  k: number
}) {
  return (
    <g>
      {flights.map((f, i) => (
        <g key={f.id} className={f.abroad ? 'geo-flight geo-flight-abroad' : 'geo-flight'}>
          <path className="geo-flight-track" d={f.d} />
          <path
            className="geo-flight-dash"
            d={f.d}
            style={
              {
                strokeDasharray: `${5 * k} ${11 * k}`,
                '--dash-period': `${-16 * k}px`,
                animationDelay: `${(i % 6) * 0.4}s`,
              } as React.CSSProperties
            }
          />
          <circle className="geo-flight-comet" r={2.4 * k}>
            <animateMotion dur={`${2.6 + (i % 4) * 0.5}s`} begin={`${(i % 6) * 0.45}s`} repeatCount="indefinite" path={f.d} />
          </circle>
        </g>
      ))}
    </g>
  )
})

const PointsLayer = memo(function PointsLayer({
  points,
  k,
  labelled,
  onTip,
  onTipHide,
}: {
  points: GeoMapPoint[]
  k: number
  labelled: Set<string>
  onTip: TipHandler
  onTipHide: () => void
}) {
  return (
    <g>
      {points.map((pt, i) => {
        const x = px(pt.lng)
        const y = py(pt.lat)
        const r = Math.min(4 + Math.sqrt(pt.total) * 1.8, 15) * k
        const alarm = pt.total > 0 && pt.failed / pt.total > 0.5
        const cls = alarm ? 'geo-point geo-point-alarm' : pt.abroad ? 'geo-point geo-point-abroad' : 'geo-point'
        const detail = pt.failed > 0 ? `${pt.total} 次登录 · 失败 ${pt.failed}` : `${pt.total} 次登录`
        return (
          <g key={pt.name} className={cls} onMouseMove={(e) => onTip(e, pt.name, detail)} onMouseLeave={onTipHide}>
            <circle className="geo-point-ripple" cx={x} cy={y} r={r} style={{ animationDelay: `${(i % 5) * 0.55}s` }} />
            <circle className="geo-point-core" cx={x} cy={y} r={r} />
            {labelled.has(pt.name) && (
              <text className="geo-point-label" x={x} y={y - r - 7 * k} style={{ fontSize: 13 * k }}>
                {pt.name}
              </text>
            )}
          </g>
        )
      })}
    </g>
  )
})

export default function GeoMap({ points, provinceTotals, height = 420 }: GeoMapProps) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const [tip, setTip] = useState<{ x: number; y: number; below: boolean; title: string; detail: string } | null>(null)

  const maxHeat = useMemo(() => {
    const values = Object.values(provinceTotals ?? {})
    return values.length ? Math.max(...values) : 1
  }, [provinceTotals])

  // 视口：中国 bbox ∪ 所有落点，加 8% 边距
  const view = useMemo(() => {
    let { minX, maxX, minY, maxY } = CHINA_BOUNDS
    for (const p of points) {
      const x = px(p.lng)
      const y = py(p.lat)
      if (x < minX) minX = x
      if (x > maxX) maxX = x
      if (y < minY) minY = y
      if (y > maxY) maxY = y
    }
    const padX = (maxX - minX) * 0.08
    const padY = (maxY - minY) * 0.08
    minX -= padX
    maxX += padX
    minY -= padY
    maxY += padY
    // 落点大小/线宽/字号随视口缩放系数换算，保持像素观感稳定
    const k = (maxX - minX) / 700
    return { minX, minY, w: maxX - minX, h: maxY - minY, k }
  }, [points])

  // 飞线枢纽 = 登录量最大的国内城市（各来源city → 枢纽，讲"登录汇聚"的故事）
  const hub = useMemo(() => {
    const domestic = points.filter((p) => !p.abroad)
    if (domestic.length < 1) return null
    return domestic.reduce((a, b) => (b.total > a.total ? b : a))
  }, [points])

  const flights = useMemo(() => {
    if (!hub) return []
    return points
      .filter((p) => p !== hub)
      .map((p) => {
        const x1 = px(p.lng)
        const y1 = py(p.lat)
        const x2 = px(hub.lng)
        const y2 = py(hub.lat)
        // 二次贝塞尔：控制点取中点向上抬，弧度随距离增大
        const mx = (x1 + x2) / 2
        const my = (y1 + y2) / 2 - Math.hypot(x2 - x1, y2 - y1) * 0.22
        return { id: p.name, d: `M${x1.toFixed(1)} ${y1.toFixed(1)}Q${mx.toFixed(1)} ${my.toFixed(1)} ${x2.toFixed(1)} ${y2.toFixed(1)}`, abroad: !!p.abroad }
      })
  }, [points, hub])

  // 标注：登录量前 5 + 所有海外来源（国名信息量大）
  const labelled = useMemo(() => {
    const top = [...points].sort((a, b) => b.total - a.total).slice(0, 5).map((p) => p.name)
    return new Set([...top, ...points.filter((p) => p.abroad).map((p) => p.name)])
  }, [points])

  // 容器 overflow:hidden 会裁掉贴边的 tip：x 按估算半宽夹取，顶部翻转到光标下方
  const showTip = useCallback((e: React.MouseEvent, title: string, detail: string) => {
    const rect = wrapRef.current?.getBoundingClientRect()
    if (!rect) return
    const halfTip = Math.min(84, rect.width / 2)
    const xPx = Math.min(Math.max(e.clientX - rect.left, halfTip), rect.width - halfTip)
    const yPx = e.clientY - rect.top
    setTip({
      x: (xPx / rect.width) * 100,
      y: (yPx / rect.height) * 100,
      below: yPx < 72,
      title,
      detail,
    })
  }, [])

  const hideTip = useCallback(() => setTip(null), [])

  const { k } = view
  return (
    <div ref={wrapRef} className="geo-map" style={{ height }}>
      <svg
        viewBox={`${view.minX.toFixed(0)} ${view.minY.toFixed(0)} ${view.w.toFixed(0)} ${view.h.toFixed(0)}`}
        preserveAspectRatio="xMidYMid meet"
        role="img"
        aria-label="登录地域分布图"
      >
        <WorldLayer />
        <ChinaLayer provinceTotals={provinceTotals} maxHeat={maxHeat} onTip={showTip} onTipHide={hideTip} />
        <FlightsLayer flights={flights} k={k} />
        <PointsLayer points={points} k={k} labelled={labelled} onTip={showTip} onTipHide={hideTip} />
      </svg>
      {tip && (
        <div
          className={tip.below ? 'geo-map-tip geo-map-tip-below' : 'geo-map-tip'}
          style={{ left: `${tip.x}%`, top: `${tip.y}%` }}
        >
          <div className="geo-map-popup-title">{tip.title}</div>
          <div className="geo-map-popup-detail">{tip.detail}</div>
        </div>
      )}
    </div>
  )
}
