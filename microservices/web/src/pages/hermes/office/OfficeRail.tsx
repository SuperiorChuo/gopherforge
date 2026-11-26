// 3D 办公室右侧运维边栏：实时视角重置、设备健康度堆叠条、区域筛选、风险区域与异常优先工位。
import { useTranslation } from 'react-i18next'
import { Button, Tooltip } from 'antd'
import { AimOutlined } from '@ant-design/icons'
import type { SeatStatus } from '@/api/ai/hermes-office'
import {
  SEAT_STATUS_COLORS,
  type OfficeDepartmentPod,
  type RiskAreaItem,
  type RiskSeatItem,
} from './constants'

export interface OfficeRailProps {
  seatCount: number
  counts: Record<SeatStatus, number>
  onlineRate: number
  riskCount: number
  editing: boolean
  dirty: boolean
  activePodKey: string
  departmentPods: OfficeDepartmentPod[]
  riskAreas: RiskAreaItem[]
  visibleRiskSeats: RiskSeatItem[]
  onResetCamera: () => void
  onSelectPod: (podKey: string) => void
  onFocusRiskSeat: (seat: { id: number; podKey: string }) => void
}

export function OfficeRail({
  seatCount,
  counts,
  onlineRate,
  riskCount,
  editing,
  dirty,
  activePodKey,
  departmentPods,
  riskAreas,
  visibleRiskSeats,
  onResetCamera,
  onSelectPod,
  onFocusRiskSeat,
}: OfficeRailProps) {
  const { t } = useTranslation()

  return (
    <aside className="hermes-office-rail">
      <div className="hermes-rail-overview">
        <div className="hermes-view-controls">
          <div className="hermes-live-status">
            <span />
            {t('实时')}
          </div>
          <Tooltip title={t('重置视角')}>
            <Button size="small" icon={<AimOutlined />} aria-label={t('重置视角')} onClick={onResetCamera} />
          </Tooltip>
        </div>

        <div className="hermes-rail-health" aria-label={t('设备在线率 {{n}}%', { n: onlineRate })}>
          <div className="hermes-rail-health-head">
            <span>{t('设备健康度')}</span>
            <strong>{onlineRate}%</strong>
          </div>
          <div className="hermes-status-stack" aria-hidden="true">
            {(
              [
                ['online', counts.online],
                ['error', counts.error],
                ['unprovisioned', counts.unprovisioned],
                ['offline', counts.offline],
                ['empty', counts.empty],
              ] as Array<[SeatStatus, number]>
            ).map(
              ([status, count]) =>
                count > 0 && (
                  <span
                    key={status}
                    style={{
                      width: `${(count / Math.max(1, seatCount)) * 100}%`,
                      background: SEAT_STATUS_COLORS[status],
                    }}
                  />
                ),
            )}
          </div>
          <div className="hermes-rail-health-meta">
            <span>
              <i style={{ background: SEAT_STATUS_COLORS.online }} />
              {t('{{n}} 在线', { n: counts.online })}
            </span>
            <span>
              <i style={{ background: riskCount ? SEAT_STATUS_COLORS.error : SEAT_STATUS_COLORS.empty }} />
              {t('{{n}} 风险', { n: riskCount })}
            </span>
            <span>
              <i style={{ background: SEAT_STATUS_COLORS.offline }} />
              {t('{{n}} 离线', { n: counts.offline })}
            </span>
          </div>
        </div>
      </div>

      {!editing && (
        <>
          <div className="hermes-rail-section">
            <div className="hermes-hud-rail-title">{t('区域')}</div>
            <div className="hermes-region-list">
              <button
                type="button"
                className={`hermes-region-option${activePodKey === 'all' ? ' is-active' : ''}`}
                onClick={() => onSelectPod('all')}
              >
                {t('全景')}
                <span className="hermes-region-count">{seatCount}</span>
              </button>
              {departmentPods.map((pod) => (
                <button
                  key={pod.key}
                  type="button"
                  className={`hermes-region-option${activePodKey === pod.key ? ' is-active' : ''}`}
                  onClick={() => onSelectPod(pod.key)}
                >
                  <span className="hermes-seat-pill-dot" style={{ background: pod.color }} />
                  <span>{pod.name}</span>
                  <span className="hermes-region-count">{pod.seatIds.length}</span>
                </button>
              ))}
            </div>
          </div>

          <div className="hermes-rail-section hermes-risk-area-section">
            <div className="hermes-hud-rail-title">{t('风险区域')}</div>
            {riskAreas.length ? (
              <div className="hermes-risk-area-list">
                {riskAreas.slice(0, 4).map((area) => (
                  <button
                    key={area.key}
                    type="button"
                    className={`hermes-risk-area${activePodKey === area.key ? ' is-active' : ''}`}
                    onClick={() => onSelectPod(area.key)}
                  >
                    <span className="hermes-seat-pill-dot" style={{ background: area.color }} />
                    <span className="hermes-risk-name">{area.name}</span>
                    <span className="hermes-risk-count">{t('{{n}} 风险', { n: area.count })}</span>
                    <span className="hermes-risk-area-sub">{t('{{n}} 异常', { n: area.errors })}</span>
                  </button>
                ))}
              </div>
            ) : (
              <div className="hermes-risk-clear">
                <span className="hermes-seat-pill-dot" />
                {t('当前区域状态正常')}
              </div>
            )}
          </div>

          <div className="hermes-rail-section hermes-priority-section">
            <div className="hermes-hud-rail-title">{t('异常优先工位')}</div>
            {visibleRiskSeats.length ? (
              <div className="hermes-priority-list">
                {visibleRiskSeats.slice(0, 5).map((seat) => (
                  <button
                    key={seat.id}
                    type="button"
                    className="hermes-risk-chip"
                    onClick={() => onFocusRiskSeat(seat)}
                  >
                    <span className="hermes-seat-pill-dot" style={{ background: seat.color }} />
                    <span className="hermes-risk-name">{seat.name}</span>
                    <span className="hermes-risk-meta">{t(seat.statusLabel)}</span>
                  </button>
                ))}
                {visibleRiskSeats.length > 5 && (
                  <div className="hermes-risk-more">{t('还有 {{n}} 个', { n: visibleRiskSeats.length - 5 })}</div>
                )}
              </div>
            ) : (
              <div className="hermes-risk-empty">{t('暂无优先项')}</div>
            )}
          </div>
        </>
      )}

      {editing && (
        <div className="hermes-edit-mode-panel">
          <div className="hermes-hud-rail-title">{t('编辑模式')}</div>
          <div className="hermes-edit-mode-state">{dirty ? t('布局有未保存改动') : t('布局尚未修改')}</div>
        </div>
      )}
    </aside>
  )
}
