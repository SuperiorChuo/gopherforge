// 3D 办公室底部三栏统计：实时概览 / 部门分布 / 优先关注。
import { useTranslation } from 'react-i18next'
import { useCountUp } from '@/hooks/useCountUp'
import {
  DEPT_FALLBACK,
  type DeptStat,
  type RiskSeatItem,
  type StatTileItem,
} from './constants'

/** 统计 tile 的数值滚动动画（useCountUp 是 hook，只能包在组件里逐个用） */
function StatValue({ value }: { value: number }) {
  const display = useCountUp(value)
  return <div className="hermes-stat-value">{display}</div>
}

export interface OfficeMetricsProps {
  statTiles: StatTileItem[]
  deptStats: DeptStat[]
  staffedCount: number
  activePodKey: string
  themeMode: 'light' | 'dark'
  visibleRiskSeats: RiskSeatItem[]
  onSelectPod: (podKey: string) => void
  onFocusRiskSeat: (seat: { id: number; podKey: string }) => void
}

export function OfficeMetrics({
  statTiles,
  deptStats,
  staffedCount,
  activePodKey,
  themeMode,
  visibleRiskSeats,
  onSelectPod,
  onFocusRiskSeat,
}: OfficeMetricsProps) {
  const { t } = useTranslation()

  return (
    <div className="hermes-office-metrics">
      <div className="hermes-metric-panel hermes-metric-panel-wide">
        <div className="hermes-metric-title">{t('实时概览')}</div>
        <div className="hermes-stat-row">
          {statTiles.map((tile) => (
            <div key={tile.key} className={`hermes-stat-tile is-${tile.key}`}>
              <div className="hermes-stat-label">
                {tile.dot && <span className="hermes-seat-pill-dot" style={{ background: tile.dot }} />}
                {t(tile.label)}
              </div>
              <StatValue value={tile.value} />
            </div>
          ))}
        </div>
      </div>

      <div className="hermes-metric-panel">
        <div className="hermes-metric-title">{t('部门分布')}</div>
        {deptStats.length ? (
          <div className="hermes-dept-list">
            {deptStats.slice(0, 6).map((d) => {
              const podKey = d.deptId ? `dept:${d.deptId}` : 'unassigned'
              return (
                <button
                  key={d.deptId}
                  type="button"
                  className={`hermes-dept-row hermes-dept-row-btn hermes-dept-filter${activePodKey === podKey ? ' is-active' : ''}`}
                  onClick={() => onSelectPod(podKey)}
                >
                  <span className="hermes-seat-pill-dot" style={{ background: d.color }} />
                  <span className="hermes-dept-copy">
                    <span className="hermes-dept-name">{d.name}</span>
                    <span className="hermes-dept-track" aria-hidden="true">
                      <span
                        style={{
                          width: `${Math.round((d.count / Math.max(1, staffedCount)) * 100)}%`,
                          background: d.color,
                        }}
                      />
                    </span>
                  </span>
                  <span className="hermes-dept-count">{d.count}</span>
                </button>
              )
            })}
            {deptStats.length > 6 && (
              <div className="hermes-dept-row">
                <span className="hermes-seat-pill-dot" style={{ background: DEPT_FALLBACK[themeMode] }} />
                <span className="hermes-dept-name">{t('其他')}</span>
                <span className="hermes-dept-count">{deptStats.slice(6).reduce((sum, d) => sum + d.count, 0)}</span>
              </div>
            )}
          </div>
        ) : (
          <div className="cell-muted">{t('还没有工位绑定员工')}</div>
        )}
      </div>

      <div className="hermes-metric-panel">
        <div className="hermes-metric-title">{t('优先关注')}</div>
        {visibleRiskSeats.length ? (
          <div className="hermes-dept-list">
            {visibleRiskSeats.slice(0, 4).map((r) => (
              <button
                key={r.id}
                type="button"
                className="hermes-dept-row hermes-dept-row-btn"
                onClick={() => onFocusRiskSeat(r)}
              >
                <span className="hermes-seat-pill-dot" style={{ background: r.color }} />
                <span className="hermes-priority-copy">
                  <span className="hermes-dept-name">{r.name}</span>
                  <span className="hermes-priority-dept">{r.dept || t('共享工位')}</span>
                </span>
                <span className="hermes-dept-count">{t(r.statusLabel)}</span>
              </button>
            ))}
          </div>
        ) : (
          <div className="cell-muted">{t('当前区域状态正常')}</div>
        )}
      </div>
    </div>
  )
}
