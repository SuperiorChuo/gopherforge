// 工位详情与编辑抽屉：展示员工/设备心跳信息，编辑态支持工位号、朝向、员工绑定与移除。
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Descriptions, Drawer, Input, InputNumber, Popconfirm, Select, Space } from 'antd'
import { DeleteOutlined } from '@ant-design/icons'
import { seatStatus } from '@/api/ai/hermes-office'
import type { SystemUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import StatusPill from '@/components/common/StatusPill'
import { STATUS_META } from './constants'
import type { SeatDraft } from './useSeatEditor'

export interface SeatDetailDrawerProps {
  seat: SeatDraft | null
  editing: boolean
  users: SystemUser[]
  deptName: Map<number, string>
  userDeptId: Map<number, number>
  onClose: () => void
  onRemoveSeat: (seatId: number) => void
  onSetLabel: (seatId: number, label: string) => void
  onRotateSeat: (seatId: number, deg: number) => void
  onBindUser: (seatId: number, userId: number, user: SeatDraft['user']) => void
}

export function SeatDetailDrawer({
  seat,
  editing,
  users,
  deptName,
  userDeptId,
  onClose,
  onRemoveSeat,
  onSetLabel,
  onRotateSeat,
  onBindUser,
}: SeatDetailDrawerProps) {
  const { t } = useTranslation()
  const userOptions = useMemo(
    () => users.map((u) => ({ value: u.id, label: `${u.nickname || u.username} (${u.username})` })),
    [users],
  )

  return (
    <Drawer
      title={seat?.seat.label || t('工位详情')}
      open={!!seat}
      size={420}
      onClose={onClose}
      destroyOnHidden
      extra={
        editing && seat ? (
          <Popconfirm
            title={t('从布局中移除此工位？')}
            onConfirm={() => {
              onRemoveSeat(seat.seat.id)
              onClose()
            }}
          >
            <Button danger size="small" icon={<DeleteOutlined />}>
              {t('移除')}
            </Button>
          </Popconfirm>
        ) : null
      }
    >
      {seat && (
        <>
          {editing && (
            <Space direction="vertical" style={{ width: '100%', marginBottom: 20 }}>
              <div>
                <div className="cell-muted" style={{ marginBottom: 4 }}>
                  {t('工位号')}
                </div>
                <Input
                  value={seat.seat.label}
                  onChange={(e) => onSetLabel(seat.seat.id, e.target.value)}
                  maxLength={40}
                />
              </div>
              <div>
                <div className="cell-muted" style={{ marginBottom: 4 }}>
                  {t('朝向（度）')}
                </div>
                <InputNumber
                  style={{ width: '100%' }}
                  min={0}
                  max={359}
                  step={15}
                  value={seat.seat.rotation}
                  onChange={(v) => onRotateSeat(seat.seat.id, Number(v ?? 0))}
                />
              </div>
              <div>
                <div className="cell-muted" style={{ marginBottom: 4 }}>
                  {t('绑定员工')}
                </div>
                <Select
                  style={{ width: '100%' }}
                  allowClear
                  showSearch
                  optionFilterProp="label"
                  placeholder={t('空位')}
                  value={seat.seat.user_id || undefined}
                  onChange={(v) => {
                    const u = users.find((item) => item.id === v)
                    onBindUser(
                      seat.seat.id,
                      v ?? 0,
                      u ? { id: u.id, username: u.username, nickname: u.nickname ?? '' } : null,
                    )
                  }}
                  options={userOptions}
                />
              </div>
            </Space>
          )}

          <Descriptions column={1} size="small" bordered>
            <Descriptions.Item label={t('状态')}>
              <StatusPill
                tone={STATUS_META[seatStatus(seat)].tone}
                label={STATUS_META[seatStatus(seat)].label}
                pulse={false}
              />
            </Descriptions.Item>
            <Descriptions.Item label={t('坐标')}>
              {seat.seat.x.toFixed(1)} , {seat.seat.y.toFixed(1)}
            </Descriptions.Item>
            <Descriptions.Item label={t('员工')}>
              {seat.user ? seat.user.nickname || seat.user.username : t('空位')}
            </Descriptions.Item>
            <Descriptions.Item label={t('部门')}>
              {seat.user ? (deptName.get(userDeptId.get(seat.user.id) ?? 0) ?? '—') : '—'}
            </Descriptions.Item>
            <Descriptions.Item label={t('设备')}>{seat.device?.device_id || t('未注册')}</Descriptions.Item>
            <Descriptions.Item label={t('Hermes 版本')}>{seat.device?.hermes_version || '—'}</Descriptions.Item>
            <Descriptions.Item label={t('最近心跳')}>
              {seat.device?.last_seen_at ? formatDateTime(seat.device.last_seen_at) : '—'}
            </Descriptions.Item>
          </Descriptions>
        </>
      )}
    </Drawer>
  )
}
