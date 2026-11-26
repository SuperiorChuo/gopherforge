// 3D 办公室场景左上浮动 HUD：楼层切换、在线率摘要、楼层增删改与布局编辑入口。
import { useTranslation } from 'react-i18next'
import { Button, Popconfirm, Select, Tooltip } from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  MinusOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons'
import type { HermesOfficeFloor } from '@/api/ai/hermes-office'

export interface OfficeHudProps {
  floor: HermesOfficeFloor
  floors: HermesOfficeFloor[]
  floorId: number | null
  seatCount: number
  onlineCount: number
  onlineRate: number
  riskCount: number
  collapsed: boolean
  canEdit: boolean
  editing: boolean
  loading: boolean
  saving: boolean
  dirty: boolean
  onToggleCollapsed: (collapsed: boolean) => void
  onSelectFloor: (floorId: number) => void
  onCreateFloor: () => void
  onEditFloor: (floor: HermesOfficeFloor) => void
  onRemoveFloor: () => void
  onRefresh: () => void
  onStartEditing: () => void
  onSaveSeats: () => void
  onLeaveEditing: () => void
}

export function OfficeHud({
  floor,
  floors,
  floorId,
  seatCount,
  onlineCount,
  onlineRate,
  riskCount,
  collapsed,
  canEdit,
  editing,
  loading,
  saving,
  dirty,
  onToggleCollapsed,
  onSelectFloor,
  onCreateFloor,
  onEditFloor,
  onRemoveFloor,
  onRefresh,
  onStartEditing,
  onSaveSeats,
  onLeaveEditing,
}: OfficeHudProps) {
  const { t } = useTranslation()

  if (collapsed) {
    return (
      <button
        type="button"
        className="hermes-hud-chip"
        onClick={() => onToggleCollapsed(false)}
        title={t('展开办公室面板')}
        aria-label={t('展开办公室面板')}
      >
        <span className="hermes-hud-chip-dot" aria-hidden="true" />
        <span className="hermes-hud-chip-name">{floor.name}</span>
        {riskCount > 0 && <span className="hermes-hud-chip-risk">{riskCount}</span>}
      </button>
    )
  }

  return (
    <div className="hermes-hud hermes-hud-tl">
      <button
        type="button"
        className="hermes-hud-collapse"
        onClick={() => onToggleCollapsed(true)}
        aria-label={t('收起面板')}
        title={t('收起面板')}
      >
        <MinusOutlined />
      </button>
      <div className="hermes-hud-card">
        <div className="hermes-hud-eyebrow">HERMES OFFICE</div>
        <div className="hermes-hud-title">{floor.name}</div>
        <div className="hermes-hud-sub">
          {t('{{a}} 个工位 · {{b}} 在线 · {{c}} 风险', { a: seatCount, b: onlineCount, c: riskCount })}
        </div>
        <div className="hermes-health-row">
          <span>{t('在线率')}</span>
          <div className="hermes-health-track" aria-hidden="true">
            <span style={{ width: `${onlineRate}%` }} />
          </div>
          <strong>{onlineRate}%</strong>
        </div>
        <div className="hermes-floor-controls">
          <Select
            size="small"
            value={floorId ?? undefined}
            placeholder={t('选择楼层')}
            onChange={onSelectFloor}
            options={floors.map((item) => ({ value: item.id, label: item.name }))}
          />
          {canEdit && !editing && (
            <Tooltip title={t('新建楼层')}>
              <Button size="small" icon={<PlusOutlined />} aria-label={t('新建楼层')} onClick={onCreateFloor} />
            </Tooltip>
          )}
          {canEdit && !editing && (
            <Tooltip title={t('编辑楼层')}>
              <Button
                size="small"
                icon={<EditOutlined />}
                aria-label={t('编辑楼层')}
                onClick={() => onEditFloor(floor)}
              />
            </Tooltip>
          )}
          {canEdit && !editing && (
            <Popconfirm title={t('删除此楼层？该层工位会一并删除。')} onConfirm={onRemoveFloor}>
              <Tooltip title={t('删除楼层')}>
                <Button size="small" danger icon={<DeleteOutlined />} aria-label={t('删除楼层')} />
              </Tooltip>
            </Popconfirm>
          )}
          <Tooltip title={t('刷新数据')}>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              aria-label={t('刷新数据')}
              loading={loading}
              onClick={onRefresh}
            />
          </Tooltip>
        </div>
        {canEdit && (
          <div className="hermes-edit-actions">
            {editing ? (
              <>
                <Button
                  size="small"
                  type="primary"
                  icon={<SaveOutlined />}
                  loading={saving}
                  disabled={!dirty}
                  onClick={onSaveSeats}
                >
                  {t('保存布局')}
                </Button>
                <Button size="small" icon={<EyeOutlined />} onClick={onLeaveEditing}>
                  {t('退出编辑')}
                </Button>
              </>
            ) : (
              <Button size="small" type="primary" icon={<EditOutlined />} onClick={onStartEditing}>
                {t('编辑布局')}
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
