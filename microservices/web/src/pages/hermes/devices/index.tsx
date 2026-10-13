// 设备终端：已注册的员工设备、健康状态、生命周期事件与吊销。
// 由原 /ai/hermes 单页的「设备」Tab 拆出。
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Col, Descriptions, Drawer, Dropdown, Popconfirm, Row, Space, Statistic, Table, Tag, Tooltip } from 'antd'
import { DesktopOutlined, EyeOutlined, ReloadOutlined, StopOutlined, ThunderboltOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  getHermesDevice,
  getHermesDeviceUsage,
  listHermesDevices,
  requestHermesDeviceAction,
  revokeHermesDevice,
  type HermesDevice,
  type HermesDeviceAction,
  type HermesDeviceEvent,
  type HermesDeviceUsage,
} from '@/api/ai/hermes'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { devicePageParams, HEALTH_LABELS, stateLabels, stateTone } from '../_shared'

// 与后端 HermesDeviceActions 一一对应。控制面只能下发这三者，不存在自由文本通道。
const ACTION_LABELS: Record<HermesDeviceAction, string> = {
  retry_update: '重试更新',
  apply_policy: '重新应用策略',
  restart: '重启 Hermes',
}

const CONNECTION_LABELS: Record<HermesDevice['connection_status'], string> = {
  never: '从未连接',
  online: '在线',
  offline: '离线',
}

const desiredStateLabel = (state: HermesDevice['desired_state']) =>
  state === 'unknown' ? '未解析' : (stateLabels[state] ?? state)

export default function HermesDevicesPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canAction = hasPerm('ai:hermes:device:action')
  const canRevoke = hasPerm('ai:hermes:device:revoke')
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailDevice, setDetailDevice] = useState<HermesDevice | null>(null)
  const [detailEvents, setDetailEvents] = useState<HermesDeviceEvent[]>([])
  const [detailUsage, setDetailUsage] = useState<HermesDeviceUsage | null>(null)

  const fetcher = useCallback(async () => {
    const result = await listHermesDevices(devicePageParams)
    const list = result.list ?? []
    return { list, total: list.length }
  }, [])
  const { list: devices, loading, reload } = useTableQuery({
    params: {},
    fetcher,
  })

  // 运营页静默轮询：后台暂停、回前台补拉；不闪 table spinner
  useVisibilityInterval(() => void reload({ silent: true }), 30_000, false)

  const activeDevices = useMemo(() => devices.filter((device) => !device.revoked_at).length, [devices])
  const onlineDevices = useMemo(
    () => devices.filter((device) => !device.revoked_at && device.connection_status === 'online').length,
    [devices],
  )

  const openDevice = async (device: HermesDevice) => {
    setDetailOpen(true)
    setDetailLoading(true)
    setDetailDevice(device)
    try {
      const [result, usage] = await Promise.all([
        getHermesDevice(device.device_id),
        // 用量是独立能力，取不到不该挡住设备详情
        getHermesDeviceUsage(device.device_id, 7).catch(() => null),
      ])
      setDetailDevice(result.device)
      setDetailEvents(result.events ?? [])
      setDetailUsage(usage)
    } catch {
      setDetailEvents([])
      setDetailUsage(null)
    } finally {
      setDetailLoading(false)
    }
  }

  const revokeDevice = async (deviceId: string) => {
    try {
      await revokeHermesDevice(deviceId)
      message.success(t('吊销请求已下发，等待设备确认'))
      setDetailOpen(false)
      await reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const sendAction = async (deviceId: string, action: HermesDeviceAction | '') => {
    try {
      await requestHermesDeviceAction(deviceId, action)
      message.success(action ? t('已排入「{{name}}」，设备下次同步时执行', { name: ACTION_LABELS[action] }) : t('已撤销待执行动作'))
      await reload()
    } catch {
      // Request layer already reports the server message.
    }
  }

  const columns: ColumnsType<HermesDevice> = [
    {
      title: t('设备'),
      dataIndex: 'device_name',
      render: (value: string, record) => (
        <div>
          <div>{value || t('未命名设备')}</div>
          <span className="cell-muted">
            {record.platform} / {record.arch}
          </span>
        </div>
      ),
    },
    { title: '员工 ID', dataIndex: 'user_id', width: 90 },
    {
      title: t('期望状态'),
      dataIndex: 'desired_state',
      width: 110,
      render: (value: HermesDevice['desired_state']) => (
        <StatusPill tone={stateTone(value)} label={desiredStateLabel(value)} />
      ),
    },
    {
      title: t('实际状态'),
      dataIndex: 'actual_state',
      width: 110,
      render: (value: string) => <StatusPill tone={stateTone(value)} label={stateLabels[value] ?? value} />,
    },
    {
      title: t('连接'),
      dataIndex: 'connection_status',
      width: 100,
      render: (value: HermesDevice['connection_status']) => (
        <StatusPill
          tone={value === 'online' ? 'success' : value === 'offline' ? 'muted' : 'info'}
          label={CONNECTION_LABELS[value]}
          pulse={false}
        />
      ),
    },
    {
      title: t('健康'),
      dataIndex: 'health_status',
      width: 120,
      render: (value: string) => (
        <StatusPill tone={stateTone(value)} label={HEALTH_LABELS[value as keyof typeof HEALTH_LABELS]?.label ?? value} pulse={false} />
      ),
    },
    { title: 'Manager', dataIndex: 'manager_version', width: 110, render: (value: string) => value || '—' },
    { title: 'Hermes', dataIndex: 'hermes_version', width: 120, render: (value: string) => value || t('未安装') },
    { title: t('策略'), dataIndex: 'applied_policy_revision', width: 80, render: (value: number) => value || t('未应用') },
    {
      title: t('待执行'),
      dataIndex: 'pending_action',
      width: 120,
      render: (value: HermesDeviceAction | '') =>
        value ? <Tag color="processing">{t(ACTION_LABELS[value] ?? value)}</Tag> : <span className="cell-muted">—</span>,
    },
    {
      title: t('最后在线'),
      dataIndex: 'last_seen_at',
      width: 160,
      render: (value?: string) => (value ? formatDateTime(value) : t('从未')),
    },
    {
      title: t('操作'),
      width: 130,
      fixed: 'right',
      render: (_, record) => (
        <Space size={2}>
          <Tooltip title={t('查看设备')}>
            <Button type="text" icon={<EyeOutlined />} onClick={() => void openDevice(record)} />
          </Tooltip>
          {!record.revoked_at && (
            <>
              <Dropdown
                disabled={!canAction}
                menu={{
                  items: [
                    ...(Object.keys(ACTION_LABELS) as HermesDeviceAction[]).map((action) => ({
                      key: action,
                      label: t(ACTION_LABELS[action]),
                    })),
                    ...(record.pending_action
                      ? [{ type: 'divider' as const }, { key: 'cancel', label: t('撤销待执行动作'), danger: true }]
                      : []),
                  ],
                  onClick: ({ key }) =>
                    void sendAction(record.device_id, key === 'cancel' ? '' : (key as HermesDeviceAction)),
                }}
              >
                <Tooltip title={canAction ? t('运维动作') : t('缺少设备运维权限')}>
                  <Button type="text" icon={<ThunderboltOutlined />} disabled={!canAction} />
                </Tooltip>
              </Dropdown>
              <Popconfirm title={t('吊销此设备？')} onConfirm={() => void revokeDevice(record.device_id)} disabled={!canRevoke}>
                <Tooltip title={canRevoke ? t('吊销设备') : t('缺少设备吊销权限')}>
                  <Button type="text" danger icon={<StopOutlined />} disabled={!canRevoke} />
                </Tooltip>
              </Popconfirm>
            </>
          )}
        </Space>
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-devices-page"
      toolbar={
      <TableToolbar
        title="互联运维"
        total={activeDevices}
        icon={<DesktopOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={t('{{n}} 台在线', { n: onlineDevices })}
        extra={
          <Tooltip title={t('刷新')}>
            <Button icon={<ReloadOutlined />} onClick={() => { void reload() }} loading={loading} />
          </Tooltip>
        }
      />}
    >

        <Table
          rowKey="device_id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={devices}
          pagination={{ pageSize: 20, showSizeChanger: true, showQuickJumper: true, pageSizeOptions: [10, 20, 50, 100], showTotal: (total) => t('共 {{n}} 条', { n: total }) }}
          scroll={{ x: 1320 }}
          locale={{ emptyText: <GlassEmpty text="暂无已注册设备" compact /> }}
        />

      <Drawer title={t('Hermes 设备')} width={560} open={detailOpen} onClose={() => setDetailOpen(false)} loading={detailLoading}>
        {detailDevice && (
          <>
            <Descriptions
              column={1}
              size="small"
              bordered
              items={[
                { key: 'name', label: t('设备'), children: detailDevice.device_name || t('未命名设备') },
                { key: 'id', label: 'Device ID', children: <span className="cell-mono">{detailDevice.device_id}</span> },
                {
                  key: 'desired',
                  label: t('期望状态'),
                  children: t(desiredStateLabel(detailDevice.desired_state)),
                },
                {
                  key: 'actual',
                  label: t('实际状态'),
                  children: t(stateLabels[detailDevice.actual_state] ?? detailDevice.actual_state),
                },
                {
                  key: 'connection',
                  label: t('连接状态'),
                  children: t(CONNECTION_LABELS[detailDevice.connection_status]),
                },
                {
                  key: 'fingerprint',
                  label: t('公钥指纹'),
                  children: <span className="cell-mono">{detailDevice.key_fingerprint}</span>,
                },
                { key: 'platform', label: t('平台'), children: `${detailDevice.platform} / ${detailDevice.arch}` },
                {
                  key: 'versions',
                  label: t('版本'),
                  children: `manager ${detailDevice.manager_version || '—'} / Hermes ${detailDevice.hermes_version || t('未安装')}`,
                },
                { key: 'error', label: t('错误码'), children: detailDevice.last_error_code || '—' },
              ]}
            />
            {detailUsage && detailUsage.summary.total_requests > 0 && (
              <>
                <div style={{ marginTop: 20, marginBottom: 8, fontWeight: 500 }}>
                  {t('模型调用（近 7 天）')}
                  <span className="cell-muted" style={{ marginLeft: 8, fontWeight: 400 }}>
                    {t('仅元数据')}
                  </span>
                </div>
                <Row gutter={[12, 12]}>
                  <Col span={6}>
                    <Statistic title={t('调用')} value={detailUsage.summary.total_requests} />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title={t('失败')}
                      value={detailUsage.summary.failed_requests}
                      valueStyle={
                        detailUsage.summary.failed_requests > 0 ? { color: 'var(--ant-color-error)' } : undefined
                      }
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic title="Token" value={detailUsage.summary.total_tokens} />
                  </Col>
                  <Col span={6}>
                    <Statistic title={t('平均耗时')} value={detailUsage.summary.avg_latency_ms} suffix="ms" />
                  </Col>
                </Row>

                {detailUsage.failures.length > 0 && (
                  <Table
                    style={{ marginTop: 12 }}
                    rowKey={(row) => `${row.created_at}-${row.model_name}`}
                    size="small"
                    pagination={false}
                    dataSource={detailUsage.failures}
                    title={() => t('最近失败调用')}
                    columns={[
                      { title: t('模型'), dataIndex: 'model_name', width: 130 },
                      { title: t('错误'), dataIndex: 'error_msg', ellipsis: true, render: (v: string) => v || '—' },
                      { title: t('时间'), dataIndex: 'created_at', width: 150, render: formatDateTime },
                    ]}
                  />
                )}
              </>
            )}

            <Table
              style={{ marginTop: 20 }}
              rowKey="id"
              size="small"
              pagination={false}
              dataSource={detailEvents}
              title={() => t('生命周期事件')}
              locale={{ emptyText: <GlassEmpty text="暂无生命周期事件" compact /> }}
              columns={[
                { title: t('事件'), dataIndex: 'event_type' },
                { title: t('结果'), dataIndex: 'result', width: 90 },
                { title: t('错误码'), dataIndex: 'error_code', render: (value: string) => value || '—' },
                { title: t('时间'), dataIndex: 'occurred_at', width: 150, render: formatDateTime },
              ]}
            />
          </>
        )}
      </Drawer>
    </ListPageShell>
  )
}
