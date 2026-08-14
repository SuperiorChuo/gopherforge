import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Descriptions, Button, Row, Col, Progress, Space, Spin, Tag } from 'antd'
import {
  ReloadOutlined,
  ThunderboltOutlined,
  TeamOutlined,
  DatabaseOutlined,
  KeyOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { getRedisInfo } from '@/api/monitor'
import { formatDuration } from '@/utils/format'
import CountUpValue from '@/components/common/CountUpValue'
import MetricTrendCard from '@/components/monitor/MetricTrendCard'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import '../monitor-page.css'

function metricNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function metricText(value: unknown): string {
  return value === null || value === undefined || value === '' ? '--' : String(value)
}

function metricCount(value: number | null): string {
  return value === null ? '--' : value.toLocaleString()
}

function updatedTime(value: Date): string {
  return value.toLocaleTimeString('zh-CN', { hour12: false })
}

function metricDuration(value: number | null, t: (s: string) => string): string {
  if (value === null) return '--'
  return value === 0 ? t('0分') : formatDuration(value)
}

interface MiniStat {
  label: string
  value: React.ReactNode
  icon: React.ReactNode
  gradient: string
  shadow: string
  tint: string
}

export default function RedisMonitorPage() {
  const { t } = useTranslation()
  const [data, setData] = useState<Record<string, unknown> | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [requestFailed, setRequestFailed] = useState(false)
  const [lastSuccessAt, setLastSuccessAt] = useState<Date | null>(null)
  const requestInFlightRef = useRef(false)

  const fetchData = useCallback(async () => {
    if (requestInFlightRef.current) return
    requestInFlightRef.current = true
    setRefreshing(true)
    try {
      const res = await getRedisInfo()
      setData(res)
      setRequestFailed(false)
      setLastSuccessAt(new Date())
    } catch {
      setRequestFailed(true)
    } finally {
      requestInFlightRef.current = false
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  // 首拉 + 10s 轮询；后台标签页暂停，回前台立即补一次
  useVisibilityInterval(fetchData, 10000)

  const server = data?.server as Record<string, unknown> | undefined
  const memory = data?.memory as Record<string, unknown> | undefined
  const stats = data?.stats as Record<string, unknown> | undefined
  const clients = data?.clients as Record<string, unknown> | undefined
  const keyspace = data?.keyspace as Record<string, unknown> | undefined

  const hits = metricNumber(stats?.keyspace_hits)
  const misses = metricNumber(stats?.keyspace_misses)
  const total = hits !== null && misses !== null ? hits + misses : null
  const hitRate = total !== null && total > 0 && hits !== null ? (hits / total) * 100 : null
  const connectedClients = metricNumber(clients?.connected)
  const operationsPerSecond = metricNumber(stats?.ops)
  const keyCount = metricNumber(keyspace?.dbsize)
  const uptimeSeconds = metricNumber(server?.uptime_seconds)
  const hitRateColor = hitRate === null
    ? undefined
    : hitRate >= 90
      ? 'var(--c-success)'
      : hitRate >= 70
        ? 'var(--c-warning)'
        : 'var(--c-error)'

  const cards: MiniStat[] = [
    {
      label: t('连接客户端'),
      value: connectedClients === null ? '--' : <CountUpValue value={connectedClients} />,
      icon: <TeamOutlined />,
      gradient: 'linear-gradient(135deg, #818cf8, #4f46e5)',
      shadow: 'rgba(79, 70, 229, 0.35)',
      tint: 'rgba(99, 102, 241, 0.13)',
    },
    {
      label: t('已用内存'),
      value: metricText(memory?.used),
      icon: <DatabaseOutlined />,
      gradient: 'linear-gradient(135deg, #22d3ee, #0891b2)',
      shadow: 'rgba(8, 145, 178, 0.35)',
      tint: 'rgba(6, 182, 212, 0.12)',
    },
    {
      label: t('每秒操作数'),
      value: operationsPerSecond === null ? '--' : <CountUpValue value={operationsPerSecond} />,
      icon: <ThunderboltOutlined />,
      gradient: 'linear-gradient(135deg, #fbbf24, #f59e0b)',
      shadow: 'rgba(245, 158, 11, 0.35)',
      tint: 'rgba(245, 158, 11, 0.11)',
    },
    {
      label: t('Key 数量'),
      value: keyCount === null ? '--' : <CountUpValue value={keyCount} />,
      icon: <KeyOutlined />,
      gradient: 'linear-gradient(135deg, #34d399, #059669)',
      shadow: 'rgba(5, 150, 105, 0.35)',
      tint: 'rgba(16, 185, 129, 0.12)',
    },
  ]

  const statusText = requestFailed
    ? lastSuccessAt
      ? t('数据中断 · 上次成功 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('数据中断 · 尚无成功数据')
    : data && lastSuccessAt
      ? t('每 10 秒自动刷新 · 更新于 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('正在获取监控数据')
  const statusAnnouncement = requestFailed ? t('Redis 监控数据中断') : data ? t('Redis 监控连接正常') : t('正在连接 Redis')

  return (
    <div className="monitor-status-page redis-monitor-page">
      <span className="monitor-status-announcement" aria-live="polite" aria-atomic="true">
        {statusAnnouncement}
      </span>
      <div className="monitor-page-header">
        <span className={`monitor-page-status ${requestFailed ? 'is-error' : data ? 'is-live' : 'is-loading'}`}>
          <span className="monitor-page-status-dot" />
          {statusText}
        </span>
        <Button icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>
          {t('刷新')}
        </Button>
      </div>

      {loading && data === null ? (
        <Card className="monitor-state-card glass-rise" bordered={false}>
          <Spin size="large" />
          <div className="monitor-state-title">{t('正在连接 Redis')}</div>
          <div className="monitor-state-copy">{t('首次监控数据返回后将展示内存、连接和缓存指标。')}</div>
        </Card>
      ) : requestFailed && data === null ? (
        <Card className="monitor-state-card glass-rise" bordered={false} role="alert">
          <WarningOutlined className="monitor-state-icon" />
          <div className="monitor-state-title">{t('数据中断')}</div>
          <div className="monitor-state-copy">{t('暂未获取到 Redis 监控数据，请检查服务状态后重新连接。')}</div>
          <Button type="primary" icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>{t('重新连接')}</Button>
        </Card>
      ) : (
      <>
      <Row gutter={[20, 20]}>
        {cards.map((s, i) => (
          <Col xs={24} sm={12} lg={6} key={s.label}>
            <Card
              className="stat-card glass-rise"
              style={{ '--tint': s.tint, '--i': i } as React.CSSProperties}
            >
              <div className="stat-card-row">
                <div>
                  <div className="stat-card-title">{s.label}</div>
                  <div className="stat-card-value" style={{ fontSize: 22 }}>{s.value}</div>
                </div>
                <div
                  className="stat-card-icon"
                  style={{ background: s.gradient, '--icon-shadow': s.shadow } as React.CSSProperties}
                >
                  {s.icon}
                </div>
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      <Row gutter={[20, 20]}>
        <Col xs={24} lg={8}>
          <Card
            title={t('缓存命中率')}
            className="glass-rise"
            style={{ height: '100%', '--i': 4 } as React.CSSProperties}
          >
            <div style={{ display: 'flex', justifyContent: 'center', padding: '12px 0' }}>
              {hitRate === null ? (
                <div className="monitor-unknown-gauge is-large">--</div>
              ) : (
              <div className="monitor-gauge-halo" style={{ '--halo': hitRateColor } as React.CSSProperties}>
                <Progress
                  type="circle"
                  percent={Math.round(hitRate * 100) / 100}
                  strokeColor={hitRateColor}
                  size={160}
                  format={(p) => (
                    <span className="monitor-gauge-value" style={{ fontSize: 24 }}>{p}%</span>
                  )}
                />
              </div>
              )}
            </div>
            <div className="monitor-gauge-foot" style={{ textAlign: 'center' }}>
              {t('命中 {{n}} · 未命中 {{m}}', { n: metricCount(hits), m: metricCount(misses) })}
            </div>
          </Card>
        </Col>

        <Col xs={24} lg={16}>
          <Card
            className="glass-rise"
            title={
              <Space>
                {t('Redis 详情')}
                <Tag color={server?.mode ? 'processing' : 'default'}>{metricText(server?.mode)}</Tag>
              </Space>
            }
            style={{ height: '100%', '--i': 5 } as React.CSSProperties}
          >
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label={t('版本')}>{metricText(server?.version)}</Descriptions.Item>
              <Descriptions.Item label={t('运行时间')}>
                {metricDuration(uptimeSeconds, t)}
              </Descriptions.Item>
              <Descriptions.Item label={t('系统')}>{metricText(server?.os)}</Descriptions.Item>
              <Descriptions.Item label={t('端口')}>{metricText(server?.tcp_port)}</Descriptions.Item>
              <Descriptions.Item label={t('内存峰值')}>{metricText(memory?.peak)}</Descriptions.Item>
              <Descriptions.Item label={t('内存上限')}>{metricText(memory?.maxmemory)}</Descriptions.Item>
              <Descriptions.Item label={t('碎片率')}>{metricText(memory?.fragmentation)}</Descriptions.Item>
              <Descriptions.Item label={t('阻塞客户端')}>{metricText(clients?.blocked)}</Descriptions.Item>
              <Descriptions.Item label={t('累计连接')}>
                {metricCount(metricNumber(stats?.total_connections_received))}
              </Descriptions.Item>
              <Descriptions.Item label={t('累计命令')}>
                {metricCount(metricNumber(stats?.total_commands_processed))}
              </Descriptions.Item>
              <Descriptions.Item label={t('过期 Key')}>
                {metricCount(metricNumber(stats?.expired_keys))}
              </Descriptions.Item>
              <Descriptions.Item label={t('淘汰 Key')}>
                {metricCount(metricNumber(stats?.evicted_keys))}
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>

      <Row gutter={[20, 20]}>
        <Col xs={24} lg={12}>
          <MetricTrendCard title={t('内存使用趋势')} metric="redis.memory.used_bytes" />
        </Col>
        <Col xs={24} lg={12}>
          <MetricTrendCard title={t('客户端连接趋势')} metric="redis.clients.connected" />
        </Col>
      </Row>
      </>
      )}
    </div>
  )
}
