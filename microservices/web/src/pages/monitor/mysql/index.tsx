import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Descriptions, Button, Row, Col, Space, Spin } from 'antd'
import {
  ReloadOutlined,
  DatabaseOutlined,
  SwapOutlined,
  ThunderboltOutlined,
  ApiOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { getMySQLInfo } from '@/api/monitor'
import { formatBytes, formatDuration } from '@/utils/format'
import MonitorGaugeCard from '@/components/monitor/MonitorGaugeCard'
import MetricTrendCard from '@/components/monitor/MetricTrendCard'
import CountUpValue from '@/components/common/CountUpValue'
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

export default function MySQLMonitorPage() {
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
      const res = await getMySQLInfo()
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

  const db = data?.database as Record<string, unknown> | undefined
  const conn = data?.connections as Record<string, unknown> | undefined
  const query = data?.queries as Record<string, unknown> | undefined
  const traffic = data?.traffic as Record<string, unknown> | undefined

  // 用数据库服务端连接数/上限计算使用率，连接池数据作为补充展示
  const maxConns = metricNumber(conn?.max_connections)
  const threads = metricNumber(conn?.threads_connected)
  const threadsRunning = metricNumber(conn?.threads_running)
  const connUsage = maxConns !== null && maxConns > 0 && threads !== null ? (threads / maxConns) * 100 : null

  const bytesReceived = metricNumber(traffic?.bytes_received)
  const bytesSent = metricNumber(traffic?.bytes_sent)
  const qps = metricNumber(query?.qps)
  const slowQueries = metricNumber(query?.slow_queries)
  const uptimeSeconds = metricNumber(data?.uptime_seconds)

  // 颜色取主题变量，亮/暗深浅由 index.css 的 --c-* 负责，无需再分两档
  const queryStats = [
    { label: 'SELECT', value: metricNumber(query?.selects), color: 'var(--c-primary)' },
    { label: 'INSERT', value: metricNumber(query?.inserts), color: 'var(--c-success)' },
    { label: 'UPDATE', value: metricNumber(query?.updates), color: 'var(--c-warning)' },
    { label: 'DELETE', value: metricNumber(query?.deletes), color: 'var(--c-error)' },
  ]

  // 连接数比 CPU 类指标更敏感，用 80/60 阈值分档
  const connLevel = connUsage === null ? undefined : connUsage > 80 ? 'high' : connUsage > 60 ? 'mid' : 'low'
  const statusText = requestFailed
    ? lastSuccessAt
      ? t('数据中断 · 上次成功 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('数据中断 · 尚无成功数据')
    : data && lastSuccessAt
      ? t('每 10 秒自动刷新 · 更新于 {{n}}', { n: updatedTime(lastSuccessAt) })
      : t('正在获取监控数据')
  const statusAnnouncement = requestFailed ? t('MySQL 监控数据中断') : data ? t('MySQL 监控连接正常') : t('正在连接 MySQL')

  return (
    <div className="monitor-status-page mysql-monitor-page">
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
          <div className="monitor-state-title">{t('正在连接 MySQL')}</div>
          <div className="monitor-state-copy">{t('首次监控数据返回后将展示连接、流量和查询指标。')}</div>
        </Card>
      ) : requestFailed && data === null ? (
        <Card className="monitor-state-card glass-rise" bordered={false} role="alert">
          <WarningOutlined className="monitor-state-icon" />
          <div className="monitor-state-title">{t('数据中断')}</div>
          <div className="monitor-state-copy">{t('暂未获取到 MySQL 监控数据，请检查服务状态后重新连接。')}</div>
          <Button type="primary" icon={<ReloadOutlined />} onClick={fetchData} loading={refreshing}>{t('重新连接')}</Button>
        </Card>
      ) : (
      <>
      <Row gutter={[20, 20]}>
        <Col xs={24} lg={8}>
          {connUsage === null ? (
            <Card
              className="monitor-gauge-card stat-card glass-rise"
              style={{ '--tint': 'rgba(148, 163, 184, 0.08)', '--i': 0 } as React.CSSProperties}
            >
              <div className="monitor-gauge-head">
                <span className="monitor-gauge-icon is-unknown"><ApiOutlined /></span>
                <span className="monitor-gauge-title">{t('连接使用率')}</span>
              </div>
              <div className="monitor-gauge-body"><div className="monitor-unknown-gauge">--</div></div>
              <div className="monitor-gauge-foot">
                {t('连接 {{n}} / {{m}} · 运行中 {{a}}', { n: metricCount(threads), m: metricCount(maxConns), a: metricCount(threadsRunning) })}
              </div>
            </Card>
          ) : (
            <MonitorGaugeCard
              title={t('连接使用率')}
              icon={<ApiOutlined />}
              percent={connUsage}
              index={0}
              level={connLevel}
              footer={<>{t('连接 {{n}} / {{m}} · 运行中 {{a}}', { n: metricCount(threads), m: metricCount(maxConns), a: metricCount(threadsRunning) })}</>}
            />
          )}
        </Col>

        <Col xs={24} lg={8}>
          <Card
            className="stat-card glass-rise"
            style={{ height: '100%', '--tint': 'rgba(99, 102, 241, 0.1)', '--i': 1 } as React.CSSProperties}
          >
            <div className="stat-card-row" style={{ marginBottom: 16 }}>
              <div>
                <div className="stat-card-title">{t('接收流量')}</div>
                <div className="stat-card-value" style={{ fontSize: 22 }}>
                  {bytesReceived === null ? '--' : formatBytes(bytesReceived)}
                </div>
              </div>
              <div
                className="stat-card-icon"
                style={{ background: 'linear-gradient(135deg, #6366f1, #4f46e5)', '--icon-shadow': 'rgba(79,70,229,0.35)' } as React.CSSProperties}
              >
                <SwapOutlined />
              </div>
            </div>
            <div className="stat-card-row">
              <div>
                <div className="stat-card-title">{t('发送流量')}</div>
                <div className="stat-card-value" style={{ fontSize: 22 }}>
                  {bytesSent === null ? '--' : formatBytes(bytesSent)}
                </div>
              </div>
              <div
                className="stat-card-icon"
                style={{ background: 'linear-gradient(135deg, #34d399, #059669)', '--icon-shadow': 'rgba(5,150,105,0.35)' } as React.CSSProperties}
              >
                <SwapOutlined rotate={180} />
              </div>
            </div>
          </Card>
        </Col>

        <Col xs={24} lg={8}>
          <Card
            className="glass-rise"
            style={{ height: '100%', '--i': 2 } as React.CSSProperties}
          >
            <div className="kv-list">
              <div className="kv-row">
                <span className="kv-label">QPS</span>
                <span className={`kv-pill ${qps === null ? '' : 'kv-pill-info'}`}>{metricCount(qps)}</span>
              </div>
              <div className="kv-row">
                <span className="kv-label">{t('慢查询')}</span>
                <span className={`kv-pill ${slowQueries === null ? '' : slowQueries > 0 ? 'kv-pill-danger' : 'kv-pill-success'}`}>
                  {metricCount(slowQueries)}
                </span>
              </div>
              <div className="kv-row">
                <span className="kv-label">{t('表数量')}</span>
                <span className="kv-pill">{metricText(db?.table_count)}</span>
              </div>
              <div className="kv-row">
                <span className="kv-label">{t('库大小')}</span>
                <span className="kv-pill">{metricText(db?.size)}</span>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={[20, 20]} style={{ marginBottom: 20 }}>
        <Col xs={24}>
          <MetricTrendCard title={t('连接使用率趋势')} metric="postgres.connections.percent" />
        </Col>
      </Row>

      <Row gutter={[20, 20]}>
        <Col xs={24} lg={12}>
          <Card
            className="glass-rise"
            style={{ '--i': 3 } as React.CSSProperties}
            title={
              <Space>
                <DatabaseOutlined className="card-title-icon" /> {t('基本信息')}
              </Space>
            }
          >
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label={t('版本')}>{metricText(data?.version)}</Descriptions.Item>
              <Descriptions.Item label={t('数据库')}>{metricText(db?.name)}</Descriptions.Item>
              <Descriptions.Item label={t('地址')}>
                <span className="cell-mono">{db?.host ? `${db.host}:${db.port ?? ''}` : '--'}</span>
              </Descriptions.Item>
              <Descriptions.Item label={t('字符集 / 排序规则')}>
                {metricText(db?.charset)} / {metricText(db?.collation)}
              </Descriptions.Item>
              <Descriptions.Item label={t('运行时间')}>
                {metricDuration(uptimeSeconds, t)}
              </Descriptions.Item>
              <Descriptions.Item label={t('历史累计连接')}>
                {metricCount(metricNumber(conn?.total_connections))}
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
                <ThunderboltOutlined className="card-title-icon" /> {t('查询统计')}
              </Space>
            }
          >
            <div className="monitor-query-grid">
              {queryStats.map((q) => (
                <div
                  key={q.label}
                  className="query-stat-tile"
                  style={{ '--qs': q.color } as React.CSSProperties}
                >
                  <div className="query-stat-label">{q.label}</div>
                  <div className="query-stat-value">
                    {q.value === null ? '--' : <CountUpValue value={q.value} />}
                  </div>
                </div>
              ))}
            </div>
            <Descriptions column={{ xs: 1, sm: 2 }} size="small" style={{ marginTop: 16 }}>
              <Descriptions.Item label={t('连接池 打开')}>{metricText(conn?.open_conns)}</Descriptions.Item>
              <Descriptions.Item label={t('连接池 使用中')}>{metricText(conn?.in_use)}</Descriptions.Item>
              <Descriptions.Item label={t('连接池 空闲')}>{metricText(conn?.idle)}</Descriptions.Item>
              <Descriptions.Item label={t('峰值连接')}>{metricText(conn?.max_used_connections)}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>
      </>
      )}
    </div>
  )
}
