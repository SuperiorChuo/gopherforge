import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Input, Select, Space, Table, Tooltip } from 'antd'
import { CloudServerOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listHermesDevices, type HermesDevice } from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid from '@/components/common/StatsGrid'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { formatDateTime } from '@/utils/format'
import { devicePageParams, HEALTH_LABELS, stateLabels, stateTone } from '../_shared'

export default function HermesRuntimesPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [devices, setDevices] = useState<HermesDevice[]>([])
  const [keyword, setKeyword] = useState('')
  const [platform, setPlatform] = useState('all')
  const [state, setState] = useState('all')

  const loadData = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true)
    try {
      const result = await listHermesDevices(devicePageParams)
      setDevices((result.list ?? []).filter((device) => !device.revoked_at))
    } catch {
      // Request layer already reports the server message.
    } finally {
      if (!quiet) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData()
  }, [loadData])

  // 运营页静默轮询：后台暂停、回前台补拉；不闪 table spinner
  useVisibilityInterval(() => void loadData(true), 30_000, false)

  const filtered = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    return devices.filter((device) => {
      if (platform !== 'all' && device.platform !== platform) return false
      if (state !== 'all' && device.actual_state !== state) return false
      if (!needle) return true
      return `${device.device_name} ${device.device_id} ${device.hermes_version} ${device.manager_version}`
        .toLowerCase()
        .includes(needle)
    })
  }, [devices, keyword, platform, state])

  const active = devices.filter((device) => device.actual_state === 'active').length
  const drift = devices.filter((device) => device.health_status === 'policy_drift').length
  const errors = devices.filter((device) => device.actual_state === 'error' || device.health_status === 'unhealthy').length

  const columns: ColumnsType<HermesDevice> = [
    {
      title: t('运行节点'),
      dataIndex: 'device_name',
      render: (value: string, record) => (
        <div>
          <div>{value || t('未命名节点')}</div>
          <span className="cell-mono cell-muted">{record.device_id}</span>
        </div>
      ),
    },
    { title: t('宿主环境'), width: 150, render: (_, record) => `${record.platform} / ${record.arch}` },
    { title: 'Manager', dataIndex: 'manager_version', width: 120, render: (value: string) => value || t('未上报') },
    { title: 'Hermes', dataIndex: 'hermes_version', width: 130, render: (value: string) => value || t('未安装') },
    {
      title: t('进程状态'),
      dataIndex: 'actual_state',
      width: 120,
      render: (value: string) => <StatusPill tone={stateTone(value)} label={stateLabels[value] ?? value} />,
    },
    {
      title: t('健康'),
      dataIndex: 'health_status',
      width: 130,
      render: (value: string) => (
        <StatusPill tone={stateTone(value)} label={HEALTH_LABELS[value as keyof typeof HEALTH_LABELS]?.label ?? value} pulse={false} />
      ),
    },
    { title: t('策略'), dataIndex: 'applied_policy_revision', width: 90, render: (value: number) => value ? `R${value}` : t('未应用') },
    { title: t('最后心跳'), dataIndex: 'last_seen_at', width: 170, render: (value?: string) => value ? formatDateTime(value) : t('从未') },
  ]

  return (
    <div className="page-list">
      <TableToolbar
        title="运行节点"
        total={devices.length}
        icon={<CloudServerOutlined />}
        gradient="linear-gradient(135deg, #14b8a6, #2563eb)"
        glow="rgba(20, 184, 166, 0.28)"
        description={t('{{a}} 个运行中 · {{b}} 个异常', { a: active, b: errors })}
        extra={
          <Tooltip title={t('刷新')}>
            <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading} />
          </Tooltip>
        }
      />

      <Card className="list-main-card" style={{ marginBottom: 16 }}>
        <StatsGrid>
          <MetricCard label={t('节点总数')} value={devices.length} />
          <MetricCard label={t('运行中')} value={active} valueClassName="metric-card-value-success" />
          <MetricCard label={t('策略漂移')} value={drift} />
          <MetricCard
            label={t('异常')}
            value={errors}
            valueClassName={errors ? 'metric-card-value-danger' : undefined}
          />
        </StatsGrid>
      </Card>

      <Card className="list-main-card">
        <Space wrap style={{ marginBottom: 16 }}>
          <Input.Search
            allowClear
            placeholder={t('搜索节点、版本或 Device ID')}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            style={{ width: 280, maxWidth: '100%' }}
          />
          <Select
            value={platform}
            onChange={setPlatform}
            style={{ width: 140 }}
            options={[
              { value: 'all', label: t('全部平台') },
              { value: 'darwin', label: 'macOS' },
              { value: 'windows', label: 'Windows' },
              { value: 'linux', label: 'Linux' },
            ]}
          />
          <Select
            value={state}
            onChange={setState}
            style={{ width: 140 }}
            options={[
              { value: 'all', label: t('全部状态') },
              // 吊销设备已被过滤（页面只显示未吊销），该选项永远匹配不到，去掉
              ...Object.entries(stateLabels)
                .filter(([value]) => value !== 'revoked')
                .map(([value, label]) => ({ value, label: t(label) })),
            ]}
          />
        </Space>
        <Table
          rowKey="device_id"
          className="list-table"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={filtered}
          pagination={{ pageSize: 20, showSizeChanger: true, showQuickJumper: true, pageSizeOptions: [10, 20, 50, 100], showTotal: (total) => t('共 {{n}} 条', { n: total }) }}
          scroll={{ x: 1120 }}
          locale={{ emptyText: <GlassEmpty text="没有符合条件的节点" compact /> }}
        />
      </Card>
    </div>
  )
}
