// 集群总览：设备在线/健康分布、版本分布、策略应用情况。
//
// 取舍：这里在前端对 listHermesDevices 的结果做聚合（page_size=100，与拆分前
// 单页的口径一致）。设备规模上来后应改为后端聚合接口，把 group by 下推 SQL，
// 否则会退化成拉全量设备再在浏览器里数。
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Row, Table, Tooltip } from 'antd'
import { DashboardOutlined, ReloadOutlined } from '@ant-design/icons'
import {
  getHermesUsageOverview,
  listHermesDevices,
  listHermesEntitlements,
  type HermesDevice,
  type HermesEntitlement,
  type HermesUsageOverview,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid from '@/components/common/StatsGrid'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import { fetchAllHermesPages } from '../admin-contract'
import { stateLabels, stateTone } from '../_shared'

interface Bucket {
  key: string
  label: string
  count: number
}

function bucketize(devices: HermesDevice[], pick: (d: HermesDevice) => string, labeler?: (k: string) => string): Bucket[] {
  const map = new Map<string, number>()
  devices.forEach((d) => {
    const k = pick(d) || '—'
    map.set(k, (map.get(k) ?? 0) + 1)
  })
  return [...map.entries()]
    .map(([key, count]) => ({ key, label: labeler ? labeler(key) : key, count }))
    .sort((a, b) => b.count - a.count)
}

export default function HermesOverviewPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [devices, setDevices] = useState<HermesDevice[]>([])
  const [entitlements, setEntitlements] = useState<HermesEntitlement[]>([])
  const [usage, setUsage] = useState<HermesUsageOverview | null>(null)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      // 聚合分布需要全量数据：逐页真实拉取，不放大 page_size 伪装全量
      const [deviceResult, entitlementResult, usageResult] = await Promise.all([
        fetchAllHermesPages<HermesDevice>((page, pageSize) => listHermesDevices({ page, page_size: pageSize })),
        fetchAllHermesPages<HermesEntitlement>((page, pageSize) => listHermesEntitlements({ page, page_size: pageSize })),
        // 代理用量是独立能力，拿不到不该拖垮整页
        getHermesUsageOverview({ days: 7, top: 5 }).catch(() => null),
      ])
      setDevices(deviceResult)
      setEntitlements(entitlementResult)
      // 同上：usage 非空但内部数组为 null 时，下面的 .length 会打穿整页。
      setUsage(
        usageResult
          ? { ...usageResult, daily: usageResult.daily ?? [], devices: usageResult.devices ?? [] }
          : null,
      )
    } catch {
      // Request layer already reports the server message.
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const live = useMemo(() => devices.filter((d) => !d.revoked_at), [devices])
  const online = useMemo(() => live.filter((d) => d.actual_state === 'active').length, [live])
  const unhealthy = useMemo(
    () => live.filter((d) => d.health_status === 'unhealthy' || d.health_status === 'policy_drift').length,
    [live],
  )
  const enabledUsers = useMemo(() => entitlements.filter((e) => e.enabled).length, [entitlements])

  const stateBuckets = useMemo(
    () => bucketize(live, (d) => d.actual_state, (k) => stateLabels[k] ?? k),
    [live],
  )
  const healthBuckets = useMemo(() => bucketize(live, (d) => d.health_status), [live])
  const versionBuckets = useMemo(
    () => bucketize(live, (d) => d.hermes_version, (k) => (k === '—' ? '未安装' : k)),
    [live],
  )
  const platformBuckets = useMemo(
    () => bucketize(live, (d) => `${d.platform}/${d.arch}`),
    [live],
  )
  const policyBuckets = useMemo(
    () =>
      bucketize(
        live,
        (d) => String(d.applied_policy_revision || 0),
        (k) => (k === '0' ? '未应用' : `R${k}`),
      ),
    [live],
  )

  const bucketCard = (title: string, buckets: Bucket[], tone?: boolean) => (
    <Card className="list-main-card" title={t(title)} size="small" style={{ height: '100%' }}>
      {buckets.length === 0 ? (
        <GlassEmpty text="暂无数据" compact />
      ) : (
        <Table
          rowKey="key"
          className="list-table"
          scroll={{ x: 800 }}
          size="small"
          pagination={false}
          showHeader={false}
          dataSource={buckets}
          columns={[
            {
              dataIndex: 'label',
              render: (label: string, row: Bucket) =>
                tone ? <StatusPill tone={stateTone(row.key)} label={label} pulse={false} /> : t(label),
            },
            { dataIndex: 'count', width: 70, align: 'right' as const },
          ]}
        />
      )}
    </Card>
  )

  return (
    <div className="page-list">
      <TableToolbar
        title="仪表盘"
        total={live.length}
        icon={<DashboardOutlined />}
        gradient="linear-gradient(135deg, #22c55e, #0f766e)"
        glow="rgba(15, 118, 110, 0.35)"
        description={t('{{a}} 台运行中 · {{b}} 台需关注 · {{c}} 名员工已开通', { a: online, b: unhealthy, c: enabledUsers })}
        extra={
          <Tooltip title={t('刷新')}>
            <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading} />
          </Tooltip>
        }
      />

      {usage && (
        <Card
          className="list-main-card"
          title={t('模型代理用量（近 7 天）')}
          size="small"
          style={{ marginBottom: 16 }}
          extra={<span className="cell-muted">{t('仅元数据，不含对话内容')}</span>}
        >
          <div style={{ marginBottom: usage.devices.length ? 16 : 0 }}>
            <StatsGrid>
              <MetricCard label={t('调用次数')} value={usage.summary.total_requests.toLocaleString()} />
              <MetricCard
                label={t('失败')}
                value={usage.summary.failed_requests.toLocaleString()}
                valueClassName={usage.summary.failed_requests > 0 ? 'metric-card-value-danger' : undefined}
              />
              <MetricCard label="Token" value={usage.summary.total_tokens.toLocaleString()} />
              <MetricCard label={t('费用')} value={`¥${usage.summary.total_cost.toFixed(2)}`} />
              <MetricCard label={t('平均耗时')} value={`${usage.summary.avg_latency_ms.toLocaleString()}ms`} />
              <MetricCard label={t('活跃设备')} value={usage.summary.active_devices} />
            </StatsGrid>
          </div>

          {usage.devices.length > 0 && (
            <Table
              rowKey="device_id"
              className="list-table"
              scroll={{ x: 800 }}
              size="small"
              pagination={false}
              dataSource={usage.devices}
              columns={[
                {
                  title: t('设备'),
                  dataIndex: 'device_id',
                  ellipsis: true,
                  render: (value: string) => <span className="cell-mono">{value}</span>,
                },
                { title: t('员工 ID'), dataIndex: 'user_id', width: 90 },
                { title: t('调用'), dataIndex: 'total_requests', width: 80, align: 'right' as const },
                {
                  title: t('失败'),
                  dataIndex: 'failed_requests',
                  width: 80,
                  align: 'right' as const,
                  render: (value: number) =>
                    value > 0 ? <span style={{ color: 'var(--ant-color-error)' }}>{value}</span> : value,
                },
                { title: 'Token', dataIndex: 'total_tokens', width: 100, align: 'right' as const },
                {
                  title: t('费用'),
                  dataIndex: 'total_cost',
                  width: 90,
                  align: 'right' as const,
                  render: (value: number) => `¥${(value ?? 0).toFixed(2)}`,
                },
              ]}
            />
          )}
        </Card>
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} md={12} xl={8}>{bucketCard('实际状态', stateBuckets, true)}</Col>
        <Col xs={24} md={12} xl={8}>{bucketCard('健康状态', healthBuckets, true)}</Col>
        <Col xs={24} md={12} xl={8}>{bucketCard('Hermes 版本', versionBuckets)}</Col>
        <Col xs={24} md={12} xl={8}>{bucketCard('平台 / 架构', platformBuckets)}</Col>
        <Col xs={24} md={12} xl={8}>{bucketCard('已应用策略', policyBuckets)}</Col>
      </Row>
    </div>
  )
}
