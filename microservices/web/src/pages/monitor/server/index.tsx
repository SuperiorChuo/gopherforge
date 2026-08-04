import { useState, useCallback } from 'react'
import { Card, Descriptions, Spin, Row, Col, Button, Space, Skeleton, Tag, Tooltip } from 'antd'
import {
  ReloadOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  HddOutlined,
  CloudServerOutlined,
  CodeOutlined,
  BellOutlined,
} from '@ant-design/icons'
import {
  getAlertEvents,
  getAlertSummary,
  getServerInfo,
  getServicesHealth,
  type MonitorAlertEvent,
  type MonitorAlertSummary,
  type ServiceHealthRow,
} from '@/api/monitor'
import { formatBytes, formatDateTime } from '@/utils/format'
import MonitorGaugeCard from '@/components/MonitorGaugeCard'
import MetricTrendCard from '@/components/MetricTrendCard'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'

export default function ServerMonitorPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const fetchData = useCallback(async () => {
    setRefreshing(true)
    try {
      const res = await getServerInfo()
      setData(res)
    } catch {
      // ignore
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  useVisibilityInterval(fetchData, 10000)

  const cpu = data.cpu as Record<string, unknown> | undefined
  const mem = data.memory as Record<string, unknown> | undefined
  const disk = data.disk as Record<string, unknown> | undefined
  const os = data.os as Record<string, unknown> | undefined

  const cpuUsage = Number(cpu?.used_percent ?? 0)
  const memUsage = Number(mem?.used_percent ?? 0)
  const diskUsage = Number(disk?.used_percent ?? 0)

  return (
    <Spin spinning={loading}>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
        <Space>
          <span className="auto-refresh-hint">
            <span className="live-dot" />
            每 10 秒自动刷新
          </span>
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>
            刷新
          </Button>
        </Space>
      </div>

      <ServicesHealthCard />

      <AlertOverviewCard />

      <Row gutter={[20, 20]}>
        <Col xs={24} md={12} lg={8}>
          <MonitorGaugeCard
            title="CPU 使用率"
            icon={<DesktopOutlined />}
            percent={cpuUsage}
            index={0}
            footer={<>{String(cpu?.cores ?? '-')} 核 · {String(cpu?.model_name || '未知型号')}</>}
          />
        </Col>
        <Col xs={24} md={12} lg={8}>
          <MonitorGaugeCard
            title="内存使用率"
            icon={<DatabaseOutlined />}
            percent={memUsage}
            index={1}
            footer={<>{formatBytes(Number(mem?.used ?? 0))} / {formatBytes(Number(mem?.total ?? 0))}</>}
          />
        </Col>
        <Col xs={24} md={12} lg={8}>
          <MonitorGaugeCard
            title="磁盘使用率"
            icon={<HddOutlined />}
            percent={diskUsage}
            index={2}
            footer={<>{formatBytes(Number(disk?.used ?? 0))} / {formatBytes(Number(disk?.total ?? 0))}</>}
          />
        </Col>

        <Col xs={24}>
          <MetricTrendCard title="CPU 使用率趋势" metric="system.cpu.used_percent" />
        </Col>
        <Col xs={24} lg={12}>
          <MetricTrendCard title="内存使用率趋势" metric="system.memory.used_percent" />
        </Col>
        <Col xs={24} lg={12}>
          <MetricTrendCard title="磁盘使用率趋势" metric="system.disk.used_percent" />
        </Col>

        <Col xs={24} lg={12}>
          <Card
            className="glass-rise"
            style={{ '--i': 3 } as React.CSSProperties}
            title={
              <Space>
                <CloudServerOutlined className="card-title-icon" /> 操作系统
              </Space>
            }
          >
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="主机名">{String(os?.hostname ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="平台">{String(os?.platform ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="系统 / 架构">
                {String(os?.go_os ?? '-')} / {String(os?.arch ?? '-')}
              </Descriptions.Item>
              <Descriptions.Item label="启动时间">{String(os?.boot_time ?? '-')}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card
            className="glass-rise"
            style={{ '--i': 4 } as React.CSSProperties}
            title={
              <Space>
                <CodeOutlined className="card-title-icon" /> Go 运行时
              </Space>
            }
          >
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="Go 版本">{String(os?.go_version ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="Goroutines">{String(os?.num_goroutine ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="编译器">{String(os?.compiler ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="内存空闲">{formatBytes(Number(mem?.free ?? 0))}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>
    </Spin>
  )
}

// 微服务健康总览：并发探测 15 个服务 ready，不健康的排最前。10 秒随页自刷。
function ServicesHealthCard() {
  const [rows, setRows] = useState<ServiceHealthRow[]>([])
  const [healthy, setHealthy] = useState(0)
  const [loading, setLoading] = useState(true)
  // 探测接口本身失败时保留上一轮结果，但要给出可见提示——
  // 监控页上「探测挂了」和「一切正常」不可混淆
  const [probeFailed, setProbeFailed] = useState(false)

  const load = useCallback(async () => {
    try {
      const res = await getServicesHealth()
      setRows(res.list ?? [])
      setHealthy(res.healthy ?? 0)
      setProbeFailed(false)
    } catch {
      setProbeFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useVisibilityInterval(load, 10000)

  const allUp = rows.length > 0 && healthy === rows.length
  return (
    <Card
      className="glass-rise"
      style={{ marginBottom: 20 }}
      title={
        <Space>
          <CloudServerOutlined className="card-title-icon" /> 微服务健康
          {rows.length > 0 && (
            <Tag color={allUp ? 'green' : 'red'} variant="filled">{healthy}/{rows.length}</Tag>
          )}
          {probeFailed && (
            <Tag color="orange" variant="filled">探测接口异常，显示为上一轮结果</Tag>
          )}
        </Space>
      }
    >
      {loading ? (
        <Skeleton active title={false} paragraph={{ rows: 2 }} />
      ) : rows.length === 0 ? (
        <span className="cell-muted">{probeFailed ? '健康探测接口不可用' : '暂无受监控的服务'}</span>
      ) : (
        <Space size={[8, 8]} wrap>
          {rows.map((r) => (
            <Tooltip
              key={r.name}
              title={r.ok ? `ready · ${r.latency_ms}ms` : r.error || `HTTP ${r.http_code}`}
            >
              <Tag
                color={r.ok ? 'green' : 'red'}
                variant={r.ok ? 'outlined' : 'filled'}
                style={{ marginInlineEnd: 0, cursor: 'default' }}
              >
                {r.name}{r.ok ? '' : ' ✕'}
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
  const [summary, setSummary] = useState<MonitorAlertSummary | null>(null)
  const [events, setEvents] = useState<MonitorAlertEvent[]>([])
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      const [sum, ev] = await Promise.all([
        getAlertSummary(),
        getAlertEvents({ page: 1, page_size: 5 }),
      ])
      setSummary(sum)
      setEvents(ev.list ?? [])
    } catch {
      // 告警引擎可能未就绪，保留上一轮结果
    } finally {
      setLoading(false)
    }
  }, [])

  useVisibilityInterval(load, 10000)

  return (
    <Card
      className="glass-rise"
      style={{ marginBottom: 20 }}
      title={
        <Space>
          <BellOutlined className="card-title-icon" /> 告警概览
          {summary && summary.firing > 0 && (
            <Tag color="red" variant="filled">告警中 {summary.firing}</Tag>
          )}
        </Space>
      }
      extra={summary?.checked_at ? <span className="cell-muted">{formatDateTime(summary.checked_at)}</span> : null}
    >
      {loading ? (
        <Skeleton active title={false} paragraph={{ rows: 2 }} />
      ) : (
        <Row gutter={[16, 16]} align="middle">
          <Col xs={24} lg={10}>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              {ALERT_STATE_STATS.map((stat) => {
                const value = summary ? Number(summary[stat.key] ?? 0) : 0
                return (
                  <div key={stat.key} className="alert-stat-pill">
                    <span className="alert-stat-value" style={{ color: stat.tone }}>{value}</span>
                    <span className="alert-stat-label">{stat.label}</span>
                  </div>
                )
              })}
            </div>
          </Col>
          <Col xs={24} lg={14}>
            {events.length === 0 ? (
              <span className="cell-muted">暂无告警事件</span>
            ) : (
              <div className="alert-event-mini">
                {events.map((e) => (
                  <div key={e.id} className="alert-event-row">
                    <Tag color={e.status === 'firing' ? 'red' : 'green'} style={{ marginInlineEnd: 8 }}>
                      {e.status === 'firing' ? '告警' : '恢复'}
                    </Tag>
                    <span className="cell-ellipsis">{e.rule_name}</span>
                    <span className="cell-muted" style={{ marginLeft: 'auto', whiteSpace: 'nowrap' }}>
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
