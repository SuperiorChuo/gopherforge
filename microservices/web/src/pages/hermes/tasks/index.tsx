import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Dropdown, Segmented, Space, Statistic, Table, Tag, Tooltip } from 'antd'
import { ForkOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listHermesDevices,
  requestHermesDeviceAction,
  type HermesDevice,
  type HermesDeviceAction,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { devicePageParams, stateLabels, stateTone } from '../_shared'

const ACTION_LABELS: Record<HermesDeviceAction, string> = {
  retry_update: '重试更新',
  apply_policy: '重新应用策略',
  restart: '重启 Hermes',
}

type ViewMode = 'pending' | 'all'

export default function HermesTasksPage() {
  const { t } = useTranslation()
  const [view, setView] = useState<ViewMode>('pending')
  const { hasPerm } = usePermission()
  const canDispatch = hasPerm('ai:hermes:device:action')

  const fetcher = useCallback(async () => {
    const result = await listHermesDevices(devicePageParams)
    const list = (result.list ?? []).filter((device) => !device.revoked_at)
    return { list, total: list.length }
  }, [])
  const { list: devices, loading, reload } = useTableQuery({
    params: {},
    fetcher,
  })

  // 运营页静默轮询：后台暂停、回前台补拉；不闪 table spinner
  useVisibilityInterval(() => void reload({ silent: true }), 30_000, false)

  const pending = useMemo(() => devices.filter((device) => device.pending_action), [devices])
  const abnormal = useMemo(
    () => devices.filter((device) => device.actual_state === 'error' || device.health_status === 'unhealthy').length,
    [devices],
  )
  const dataSource = view === 'pending' ? pending : devices

  const dispatch = async (device: HermesDevice, action: HermesDeviceAction | '') => {
    try {
      await requestHermesDeviceAction(device.device_id, action)
      message.success(action ? t('已向「{{name}}」派发{{action}}', { name: device.device_name || device.device_id, action: ACTION_LABELS[action] }) : t('已撤销待执行任务'))
      await reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const columns: ColumnsType<HermesDevice> = [
    {
      title: t('目标节点'),
      dataIndex: 'device_name',
      render: (value: string, record) => (
        <div>
          <div>{value || t('未命名节点')}</div>
          <span className="cell-muted">{record.platform} / {record.arch}</span>
        </div>
      ),
    },
    {
      title: t('运行状态'),
      dataIndex: 'actual_state',
      width: 110,
      render: (value: string) => <StatusPill tone={stateTone(value)} label={stateLabels[value] ?? value} />,
    },
    {
      title: t('任务'),
      dataIndex: 'pending_action',
      width: 150,
      render: (value: HermesDeviceAction | '') =>
        value ? <Tag color="processing">{t(ACTION_LABELS[value] ?? value)}</Tag> : <span className="cell-muted">{t('待派发')}</span>,
    },
    {
      title: t('派发时间'),
      dataIndex: 'pending_action_at',
      width: 170,
      render: (value?: string) => (value ? formatDateTime(value) : '—'),
    },
    {
      title: t('最后同步'),
      dataIndex: 'last_seen_at',
      width: 170,
      render: (value?: string) => (value ? formatDateTime(value) : t('从未')),
    },
    {
      title: t('操作'),
      width: 90,
      fixed: 'right',
      render: (_, record) => (
        <Dropdown
          disabled={!canDispatch}
          menu={{
            items: [
              ...(Object.keys(ACTION_LABELS) as HermesDeviceAction[]).map((action) => ({
                key: action,
                label: t(ACTION_LABELS[action]),
              })),
              ...(record.pending_action
                ? [{ type: 'divider' as const }, { key: 'cancel', label: t('撤销当前任务'), danger: true }]
                : []),
            ],
            onClick: ({ key }) => void dispatch(record, key === 'cancel' ? '' : (key as HermesDeviceAction)),
          }}
        >
          <Tooltip title={canDispatch ? t('派发受控运维动作') : t('缺少设备运维权限')}>
            <Button type="text" icon={<ThunderboltOutlined />} disabled={!canDispatch} />
          </Tooltip>
        </Dropdown>
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-tasks-page"
      toolbar={
      <TableToolbar
        title="设备动作队列"
        total={pending.length}
        icon={<ForkOutlined />}
        gradient="linear-gradient(135deg, #0ea5e9, #0f766e)"
        glow="rgba(14, 165, 233, 0.28)"
        description={t('{{a}} 个待执行动作 · {{b}} 个异常节点', { a: pending.length, b: abnormal })}
        extra={
          <Space>
            <Segmented<ViewMode>
              value={view}
              onChange={setView}
              options={[{ label: t('待执行'), value: 'pending' }, { label: t('全部节点'), value: 'all' }]}
            />
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }} loading={loading} />
            </Tooltip>
          </Space>
        }
      />}
    >

        <Space size={28} wrap style={{ marginBottom: 18 }}>
          <Statistic title={t('待执行')} value={pending.length} />
          <Statistic title={t('可派发节点')} value={devices.length} />
          <Statistic title={t('异常节点')} value={abnormal} valueStyle={abnormal ? { color: 'var(--ant-color-error)' } : undefined} />
        </Space>
        <Table
          rowKey="device_id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={dataSource}
          pagination={{ pageSize: 20, showSizeChanger: true, showQuickJumper: true, pageSizeOptions: [10, 20, 50, 100], showTotal: (total) => t('共 {{n}} 条', { n: total }) }}
          scroll={{ x: 920 }}
          locale={{ emptyText: <GlassEmpty text={view === 'pending' ? '当前没有待执行动作' : '暂无节点'} compact /> }}
        />
    </ListPageShell>
  )
}
