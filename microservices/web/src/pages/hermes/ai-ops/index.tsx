import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Col, Row, Segmented, Table, Tag, Tooltip } from 'antd'
import { ReloadOutlined, RobotOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  getHermesUsageOverview,
  listHermesPolicies,
  type HermesPolicy,
  type HermesUsageByDevice,
  type HermesUsageDaily,
  type HermesUsageOverview,
} from '@/api/ai/hermes'
import { getEnabledAIModels, type AIModel } from '@/api/ai/model'
import GlassEmpty from '@/components/common/GlassEmpty'
import MetricCard from '@/components/common/MetricCard'
import StatsGrid from '@/components/common/StatsGrid'
import StatusPill from '@/components/common/StatusPill'
import TableToolbar from '@/components/common/TableToolbar'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import { formatDateTime } from '@/utils/format'
import { fetchAllHermesPages } from '../admin-contract'
import { stateLabels, stateTone } from '../_shared'

const emptyUsage: HermesUsageOverview = {
  days: 7,
  summary: {
    total_requests: 0,
    failed_requests: 0,
    total_tokens: 0,
    total_cost: 0,
    avg_latency_ms: 0,
    active_devices: 0,
  },
  daily: [],
  devices: [],
}

export default function HermesAIOpsPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [days, setDays] = useState<7 | 30>(7)
  const [usage, setUsage] = useState<HermesUsageOverview>(emptyUsage)
  const [usageLoaded, setUsageLoaded] = useState(false)
  const [usageError, setUsageError] = useState(false)
  const [policies, setPolicies] = useState<HermesPolicy[]>([])
  const [policyError, setPolicyError] = useState(false)
  const [models, setModels] = useState<AIModel[]>([])
  const [modelError, setModelError] = useState(false)

  const loadData = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true)
    try {
      // allSettled：policies 缺权限（403）或 models 失败时，用量主表不受影响
      const [usageResult, policyResult, modelResult] = await Promise.allSettled([
        getHermesUsageOverview({ days, top: 20 }),
        // 模型引用计数需要全量策略：逐页真实拉取，不放大 page_size 伪装全量
        fetchAllHermesPages<HermesPolicy>((page, pageSize) => listHermesPolicies({ page, page_size: pageSize })),
        getEnabledAIModels(),
      ])
      if (usageResult.status === 'fulfilled') {
        const value = usageResult.value
        setUsage({ ...value, daily: value.daily ?? [], devices: value.devices ?? [] })
        setUsageLoaded(true)
        setUsageError(false)
      } else {
        setUsageError(true)
      }
      if (policyResult.status === 'fulfilled') {
        setPolicies(policyResult.value)
        setPolicyError(false)
      } else {
        setPolicyError(true)
      }
      if (modelResult.status === 'fulfilled') {
        setModels(modelResult.value ?? [])
        setModelError(false)
      } else {
        setModelError(true)
      }
    } catch {
      // Request layer already reports the server message.
    } finally {
      if (!quiet) setLoading(false)
    }
  }, [days])

  useEffect(() => {
    void loadData()
  }, [loadData])

  // 运营页静默轮询：后台暂停、回前台补拉；不闪 table spinner
  useVisibilityInterval(() => void loadData(true), 30_000, false)

  const modelPolicyCounts = useMemo(() => {
    const counts = new Map<number, number>()
    policies.forEach((policy) => {
      if (policy.model_id) counts.set(policy.model_id, (counts.get(policy.model_id) ?? 0) + 1)
    })
    return counts
  }, [policies])

  const successRate = usage.summary.total_requests
    ? ((usage.summary.total_requests - usage.summary.failed_requests) / usage.summary.total_requests) * 100
    : 0

  const dailyColumns: ColumnsType<HermesUsageDaily> = [
    { title: t('日期'), dataIndex: 'day', width: 130 },
    { title: t('调用'), dataIndex: 'total_requests', width: 90, align: 'right' },
    {
      title: t('失败'),
      dataIndex: 'failed_requests',
      width: 90,
      align: 'right',
      render: (value: number) => value ? <span style={{ color: 'var(--ant-color-error)' }}>{value}</span> : 0,
    },
    { title: 'Token', dataIndex: 'total_tokens', width: 120, align: 'right' },
    { title: t('费用'), dataIndex: 'total_cost', width: 100, align: 'right', render: (value: number) => `¥${(value ?? 0).toFixed(2)}` },
  ]

  const deviceColumns: ColumnsType<HermesUsageByDevice> = [
    { title: t('节点'), dataIndex: 'device_id', ellipsis: true, render: (value: string) => <span className="cell-mono">{value}</span> },
    { title: t('调用'), dataIndex: 'total_requests', width: 90, align: 'right' },
    { title: t('失败'), dataIndex: 'failed_requests', width: 90, align: 'right' },
    { title: 'Token', dataIndex: 'total_tokens', width: 110, align: 'right' },
    { title: t('最近调用'), dataIndex: 'last_call_at', width: 170, render: (value?: string) => value ? formatDateTime(value) : '—' },
  ]

  return (
    <div className="page-list">
      <TableToolbar
        title="模型用量"
        total={usage.summary.active_devices}
        icon={<RobotOutlined />}
        gradient="linear-gradient(135deg, #8b5cf6, #0ea5e9)"
        glow="rgba(139, 92, 246, 0.25)"
        description={t('{{a}} 次模型调用 · {{b}} 个启用模型', { a: usage.summary.total_requests, b: models.length })}
        extra={
          <div style={{ display: 'flex', gap: 8 }}>
            <Segmented
              value={days}
              onChange={(value) => setDays(value as 7 | 30)}
              options={[{ label: t('7 天'), value: 7 }, { label: t('30 天'), value: 30 }]}
            />
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => void loadData()} loading={loading} />
            </Tooltip>
          </div>
        }
      />

      {(usageError || policyError || modelError) && (
        <Alert
          type="warning"
          showIcon
          message={t('部分模型运营数据暂不可用')}
          description={usageError && usageLoaded
            ? t('当前保留上次成功的 {{n}} 天用量数据；其他失败数据源也不会覆盖旧结果。', { n: usage.days })
            : t('失败的数据源不会覆盖上次成功结果，请重试刷新。')}
          action={<Button size="small" onClick={() => void loadData()}>{t('重试')}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      <Card className="list-main-card" style={{ marginBottom: 16 }} loading={loading && (!usageLoaded || usage.days !== days)}>
        {usageLoaded ? (
          <StatsGrid>
            <MetricCard label={t('模型调用')} value={usage.summary.total_requests.toLocaleString()} />
            <MetricCard label={t('成功率')} value={`${successRate.toFixed(1)}%`} />
            <MetricCard label="Token" value={usage.summary.total_tokens.toLocaleString()} />
            <MetricCard label={t('费用')} value={`¥${usage.summary.total_cost.toFixed(2)}`} />
            <MetricCard label={t('平均耗时')} value={`${usage.summary.avg_latency_ms.toLocaleString()}ms`} />
            <MetricCard label={t('有调用设备')} value={usage.summary.active_devices} />
          </StatsGrid>
        ) : (
          <GlassEmpty text="模型用量暂不可用" compact />
        )}
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={14}>
          <Card className="list-main-card" title={t('调用趋势')} size="small">
            <Table
              rowKey="day"
              className="list-table"
              size="small"
              loading={loading}
              columns={dailyColumns}
              dataSource={usage.daily}
              pagination={false}
              scroll={{ x: 560 }}
              locale={{ emptyText: <GlassEmpty text={usageError && !usageLoaded ? t('模型用量暂不可用') : t('暂无 Hermes 模型调用')} compact /> }}
            />
          </Card>
        </Col>
        <Col xs={24} xl={10}>
          <Card className="list-main-card" title={t('模型与策略')} size="small">
            <Table
              rowKey="id"
              className="list-table"
              size="small"
              loading={loading}
              dataSource={models}
              pagination={false}
              columns={[
                { title: t('模型'), render: (_, model) => model.display_name || model.name },
                { title: t('供应商'), dataIndex: 'provider', width: 110, render: (value: string) => <Tag>{value}</Tag> },
                { title: t('策略引用'), width: 90, align: 'right', render: (_, model) => modelPolicyCounts.get(model.id) ?? 0 },
              ]}
              locale={{ emptyText: <GlassEmpty text={modelError ? t('模型配置暂不可用') : t('暂无启用模型')} compact /> }}
            />
            {policies[0] && (
              <div style={{ marginTop: 12 }}>
                <StatusPill
                  tone={stateTone(policies[0].desired_state)}
                  label={t('最新策略 R{{rev}} · {{state}}', {
                    rev: policies[0].revision,
                    state: stateLabels[policies[0].desired_state] ?? policies[0].desired_state,
                  })}
                  pulse={false}
                />
              </div>
            )}
          </Card>
        </Col>
        <Col span={24}>
          <Card className="list-main-card" title={t('节点用量')} size="small">
            <Table
              rowKey="device_id"
              className="list-table"
              size="small"
              loading={loading}
              columns={deviceColumns}
              dataSource={usage.devices}
              pagination={false}
              scroll={{ x: 720 }}
              locale={{ emptyText: <GlassEmpty text={usageError && !usageLoaded ? t('节点用量暂不可用') : t('暂无活跃节点用量')} compact /> }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
