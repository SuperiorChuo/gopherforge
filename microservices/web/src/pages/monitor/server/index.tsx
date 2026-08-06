import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Descriptions, Spin, Row, Col, Button, Space, Skeleton, Tag, Tooltip } from 'antd'
import {
  ReloadOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  HddOutlined,
  CloudServerOutlined,
  CodeOutlined,
  BellOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import {
  getAlertEvents,
  getAlertSummary,
  getServerInfo,
  getServicesHealth,
  type MonitorAlertEvent,
  type MonitorAlertSummary,
  type ServerInfo,
  type ServiceHealthRow,
} from '@/api/monitor'
import { formatBytes, formatDateTime } from '@/utils/format'
import MonitorGaugeCard from '@/components/MonitorGaugeCard'
import MetricTrendCard from '@/components/MetricTrendCard'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import '../monitor-page.css'
import './styles.css'

function metricNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function metricPercent(value: unknown): number | null {
  const parsed = metricNumber(value)
  return parsed !== null && parsed >= 0 && parsed <= 100 ? parsed : null
}

function metricText(value: unknown): string {
  if (typeof value === 'string') return value.trim() || '--'
  return value === null || value === undefined ? '--' : String(value)
}

function metricBytes(value: number | null): string {
  return value === null ? '--' : formatBytes(value)
}

function updatedTime(value: Date): string {
  return value.toLocaleTimeString('zh-CN', { hour12: false })
}

interface ServerGaugeProps {
  title: string
  icon: React.ReactNode
  percent: number | null
  footer: React.ReactNode
  index: number
}

function ServerGaugeCard({ title, icon, percent, footer, index }: ServerGaugeProps) {
  if (percent !== null) {
    return <MonitorGaugeCard title={title} icon={icon} percent={percent} footer={footer} index={index} />
  }

  return (
    <Card
      className="monitor-gauge-card stat-card glass-rise"
      style={{ '--tint': 'rgba(148, 163, 184, 0.08)', '--i': index } as React.CSSProperties}
    >
      <div className="monitor-gauge-head">
        <span className="monitor-gauge-icon is-unknown">{icon}</span>
        <span className="monitor-gauge-title">{title}</span>
      </div>
      <div className="monitor-gauge-body"><div className="monitor-unknown-gauge">--</div></div>
      <div className="monitor-gauge-foot">{footer}</div>
    </Card>
  )
}

export default function ServerMonitorPage() {
  const { t } = useTranslation()
  const [data, setData] = useState<ServerInfo | null>(null)
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
      const res = await getServerInfo()
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

  useVisibilityInterval(fetchData, 10000)

  const cpuCores = metricNumber(data?.cpu?.cores)
  const cpuUsage = cpuCores !== null && cpuCores > 0 ? metricPercent(data?.cpu?.used_percent) : null
  const memTotal = metricNumber(data?.memory?.total)
  const memAvailable = memTotal !== null && memTotal > 0
  const memUsed = memAvailable ? metricNumber(data?.memory?.used) : null
  const memFree = memAvailable ? metricNumber(data?.memory?.free) : null
  const memUsage = memAvailable ? metricPercent(data?.memory?.used_percent) : null
  const diskTotal = metricNumber(data?.disk?.total)
  const diskAvailable = diskTotal !== null && diskTotal > 0
  const diskUsed = diskAvailable ? metricNumber(data?.disk?.used) : null
  const diskUsage = diskAvailable ? metricPercent(data?.disk?.used_percent) : null
  const osAvailable = Boolean(
    data?.os?.hostname?.trim()
      && data.os.platform?.trim()
      && data.os.boot_time?.trim()
      && !data.os.boot_time.startsWith('1970-01-01'),
  )
  const hasPartialData = data !== null && (
    cpuUsage === null || memUsage === null || diskUsage === null || !osAvailable
  )
  const statusText = requestFailed
    ? lastSuccessAt
      ? t('数据中断 · 上次成功 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('数据中断 · 尚无成功数据')
    : data && lastSuccessAt
      ? hasPartialData
        ? t('连接正常 · 部分指标不可用 · 更新于 {{n}}', { n: updatedTime(lastSuccessAt) })
        : t('每 10 秒自动刷新 · 更新于 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('正在获取监控数据')
  const statusTone = requestFailed ? 'is-error' : data ? hasPartialData ? 'is-warning' : 'is-live' : 'is-loading'
  const statusAnnouncement = requestFailed
    ? t('服务器监控数据中断')
    : hasPartialData
      ? t('服务器监控连接正常，部分指标不可用')
      : data
        ? t('服务器监控连接正常')
        : t('正在连接服务器监控')

  return (
    <div className="monitor-status-page server-monitor-page">
      <span className="monitor-status-announcement" aria-live="polite" aria-atomic="true">
        {statusAnnouncement}
      </span>
      <div className="monitor-page-header">
        <span className={`monitor-page-status ${statusTone}`}>
          <span className="monitor-page-status-dot" />
          {statusText}
        </span>
        <Button icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>
          {t('刷新主机指标')}
        </Button>
      </div>

      <ServicesHealthCard />

      <AlertOverviewCard />

      {loading && data === null ? (
        <Card className="monitor-state-card glass-rise" bordered={false}>
          <Spin size="large" />
          <div className="monitor-state-title">{t('正在连接服务器监控')}</div>
          <div className="monitor-state-copy">{t('首次监控数据返回后将展示资源用量和运行环境。')}</div>
        </Card>
      ) : requestFailed && data === null ? (
        <Card className="monitor-state-card glass-rise" bordered={false} role="alert">
          <WarningOutlined className="monitor-state-icon" />
          <div className="monitor-state-title">{t('主机数据中断')}</div>
          <div className="monitor-state-copy">{t('暂未获取到服务器监控数据，请检查监控服务后重新连接。')}</div>
          <Button type="primary" icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>{t('重新连接')}</Button>
        </Card>
      ) : (
        <Row gutter={[20, 20]}>
          <Col xs={24} md={12} lg={8}>
            <ServerGaugeCard
              title={t('CPU 使用率')}
              icon={<DesktopOutlined />}
              percent={cpuUsage}
              index={0}
              footer={<>{cpuCores !== null && cpuCores > 0 ? t('{{n}} 核', { n: cpuCores }) : t('核心数 --')} · {metricText(data?.cpu?.model_name)}</>}
            />
          </Col>
          <Col xs={24} md={12} lg={8}>
            <ServerGaugeCard
              title={t('内存使用率')}
              icon={<DatabaseOutlined />}
              percent={memUsage}
              index={1}
              footer={<>{metricBytes(memUsed)} / {metricBytes(memAvailable ? memTotal : null)}</>}
            />
          </Col>
          <Col xs={24} md={12} lg={8}>
            <ServerGaugeCard
              title={t('磁盘使用率')}
              icon={<HddOutlined />}
              percent={diskUsage}
              index={2}
              footer={<>{metricBytes(diskUsed)} / {metricBytes(diskAvailable ? diskTotal : null)}</>}
            />
          </Col>

          <Col xs={24}>
            <MetricTrendCard title={t('CPU 使用率趋势')} metric="system.cpu.used_percent" />
          </Col>
          <Col xs={24} lg={12}>
            <MetricTrendCard title={t('内存使用率趋势')} metric="system.memory.used_percent" />
          </Col>
          <Col xs={24} lg={12}>
            <MetricTrendCard title={t('磁盘使用率趋势')} metric="system.disk.used_percent" />
          </Col>

          <Col xs={24} lg={12}>
            <Card
              className="glass-rise"
              style={{ '--i': 3 } as React.CSSProperties}
              title={
                <Space>
                  <CloudServerOutlined className="card-title-icon" /> {t('操作系统')}
                </Space>
              }
            >
              <Descriptions column={1} bordered size="small">
                <Descriptions.Item label={t('主机名')}>{metricText(data?.os?.hostname)}</Descriptions.Item>
                <Descriptions.Item label={t('平台')}>{metricText(data?.os?.platform)}</Descriptions.Item>
                <Descriptions.Item label={t('系统 / 架构')}>
                  {metricText(data?.os?.go_os)} / {metricText(data?.os?.arch)}
                </Descriptions.Item>
                <Descriptions.Item label={t('启动时间')}>
                  {osAvailable ? metricText(data?.os?.boot_time) : '--'}
                </Descriptions.Item>
              </Descriptions>
            </Card>
          </Col>

          <Col xs={24} lg={12}>
            <Card
              className="glass-rise"
              style={{ '--i': 4 } as React.CSSProperties}
              title={
                <Space>
                  <CodeOutlined className="card-title-icon" /> {t('Go 运行时')}
                </Space>
              }
            >
              <Descriptions column={1} bordered size="small">
                <Descriptions.Item label={t('Go 版本')}>{metricText(data?.os?.go_version)}</Descriptions.Item>
                <Descriptions.Item label="Goroutines">{metricText(data?.os?.num_goroutine)}</Descriptions.Item>
                <Descriptions.Item label={t('编译器')}>{metricText(data?.os?.compiler)}</Descriptions.Item>
                <Descriptions.Item label={t('内存空闲')}>{metricBytes(memFree)}</Descriptions.Item>
              </Descriptions>
            </Card>
          </Col>
        </Row>
      )}
    </div>
  )
}

// 微服务健康总览：并发探测 15 个服务 ready，不健康的排最前。10 秒随页自刷。
function ServicesHealthCard() {
  const { t } = useTranslation()
  const [rows, setRows] = useState<ServiceHealthRow[]>([])
  const [healthy, setHealthy] = useState(0)
  const [loading, setLoading] = useState(true)
  // 探测接口本身失败时保留上一轮结果，但要给出可见提示——
  // 监控页上「探测挂了」和「一切正常」不可混淆
  const [probeFailed, setProbeFailed] = useState(false)
  const requestInFlightRef = useRef(false)

  const load = useCallback(async () => {
    if (requestInFlightRef.current) return
    requestInFlightRef.current = true
    try {
      const res = await getServicesHealth()
      setRows(res.list ?? [])
      setHealthy(res.healthy ?? 0)
      setProbeFailed(false)
    } catch {
      setProbeFailed(true)
    } finally {
      requestInFlightRef.current = false
      setLoading(false)
    }
  }, [])

  useVisibilityInterval(load, 10000)

  const allUp = rows.length > 0 && healthy === rows.length
  return (
    <Card
      className="glass-rise"
      title={
        <div className="server-card-title">
          <CloudServerOutlined className="card-title-icon" /> {t('微服务健康')}
          {rows.length > 0 && (
            <Tag color={allUp ? 'green' : 'red'} variant="filled">{healthy}/{rows.length}</Tag>
          )}
          {probeFailed && (
            <Tag className="server-card-warning" color="orange" variant="filled">
              {rows.length > 0 ? t('探测中断，显示上次结果') : t('探测接口不可用')}
            </Tag>
          )}
        </div>
      }
    >
      {loading ? (
        <Skeleton active title={false} paragraph={{ rows: 2 }} />
      ) : rows.length === 0 ? (
        <span className="cell-muted">{probeFailed ? t('健康探测接口不可用') : t('暂无受监控的服务')}</span>
      ) : (
        <Space size={[8, 8]} wrap>
          {rows.map((r) => (
            <Tooltip
              key={r.name}
              title={r.ok ? `ready · ${r.latency_ms}ms` : r.error || `HTTP ${r.http_code}`}
            >
              <Tag
                className="server-health-tag"
                color={r.ok ? 'green' : 'red'}
                variant={r.ok ? 'outlined' : 'filled'}
                tabIndex={0}
                style={{ marginInlineEnd: 0, cursor: 'default' }}
              >
                <span>{r.name}{r.ok ? '' : ' ✕'}</span>
              </Tag>
            </Tooltip>
          ))}
        </Space>
      )}
    </Card>
  )
}

// 告警概览：内置告警引擎的摘要卡片 + 最近事件。随页 10 秒自刷。
const ALERT_STATE_STATS: { key: keyof MonitorAlertSummary; label: string; tone: string }[] = [
  { key: 'firing', label: '告警中', tone: 'var(--c-error-strong)' },
  { key: 'pending', label: '等待确认', tone: 'var(--c-warning)' },
  { key: 'error', label: '采集异常', tone: 'var(--c-orange)' },
  { key: 'total', label: '规则总数', tone: 'var(--text-secondary)' },
]

function AlertOverviewCard() {
  const { t } = useTranslation()
  const [summary, setSummary] = useState<MonitorAlertSummary | null>(null)
  const [events, setEvents] = useState<MonitorAlertEvent[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [summaryFailed, setSummaryFailed] = useState(false)
  const [eventsFailed, setEventsFailed] = useState(false)
  const requestInFlightRef = useRef(false)

  const load = useCallback(async () => {
    if (requestInFlightRef.current) return
    requestInFlightRef.current = true
    const [summaryResult, eventsResult] = await Promise.allSettled([
      getAlertSummary(),
      getAlertEvents({ page: 1, page_size: 5 }),
    ])

    if (summaryResult.status === 'fulfilled') {
      setSummary(summaryResult.value)
      setSummaryFailed(false)
    } else {
      setSummaryFailed(true)
    }
    if (eventsResult.status === 'fulfilled') {
      setEvents(eventsResult.value.list ?? [])
      setEventsFailed(false)
    } else {
      setEventsFailed(true)
    }
    requestInFlightRef.current = false
    setLoading(false)
  }, [])

  useVisibilityInterval(load, 10000)

  return (
    <Card
      className="glass-rise server-alert-card"
      title={
        <div className="server-card-title">
          <BellOutlined className="card-title-icon" /> {t('告警概览')}
          {summary && summary.firing > 0 && (
            <Tag color="red" variant="filled">{t('告警中 {{n}}', { n: summary.firing })}</Tag>
          )}
          {(summaryFailed || eventsFailed) && (
            <Tag className="server-card-warning" color="orange" variant="filled">
              {summary || events ? t('数据中断，显示可用结果') : t('告警数据不可用')}
            </Tag>
          )}
        </div>
      }
      extra={summary?.checked_at ? <span className="cell-muted">{formatDateTime(summary.checked_at)}</span> : null}
    >
      {loading && summary === null && events === null ? (
        <Skeleton active title={false} paragraph={{ rows: 2 }} />
      ) : summary === null && events === null ? (
        <div className="server-alert-unavailable" role="alert">
          <WarningOutlined />
          <span>{t('暂未获取到告警摘要和最近事件。')}</span>
        </div>
      ) : (
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} lg={10}>
            {summary === null ? (
              <div className="server-alert-unavailable"><WarningOutlined /> {t('告警摘要暂不可用')}</div>
            ) : (
              <div className="server-alert-stats">
                {ALERT_STATE_STATS.map((stat) => {
                  const value = Number(summary[stat.key] ?? 0)
                  return (
                    <div key={stat.key} className="alert-stat-pill">
                      <span className="alert-stat-value" style={{ color: stat.tone }}>{value}</span>
                      <span className="alert-stat-label">{t(stat.label)}</span>
                    </div>
                  )
                })}
              </div>
            )}
          </Col>
          <Col xs={24} lg={14}>
            {events === null ? (
              <div className="server-alert-unavailable"><WarningOutlined /> {t('最近事件暂不可用')}</div>
            ) : events.length === 0 ? (
              <span className="cell-muted">{t('暂无告警事件')}</span>
            ) : (
              <div className="alert-event-mini">
                {events.map((e) => (
                  <div key={e.id} className="alert-event-row server-alert-event-row">
                    <Tag color={e.status === 'firing' ? 'red' : 'green'} style={{ marginInlineEnd: 8 }}>
                      {e.status === 'firing' ? t('告警') : t('恢复')}
                    </Tag>
                    <span className="cell-ellipsis">{e.rule_name}</span>
                    <span className="cell-muted server-alert-event-time">
                      {formatDateTime(e.created_at)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Col>
        </Row>
      )}
    </Card>
  )
}
