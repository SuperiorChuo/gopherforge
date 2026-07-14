import { useCallback, useEffect, useState } from 'react'
import { Card, Descriptions, Button, Row, Col, Progress, Space } from 'antd'
import {
  ReloadOutlined,
  DatabaseOutlined,
  SwapOutlined,
  ThunderboltOutlined,
  ApiOutlined,
} from '@ant-design/icons'
import { getMySQLInfo } from '@/api/monitor'
import { formatBytes, formatDuration } from '@/utils/format'

export default function MySQLMonitorPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(false)

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getMySQLInfo()
      setData(res)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 10000)
    return () => clearInterval(timer)
  }, [fetchData])

  const db = data.database as Record<string, unknown> | undefined
  const conn = data.connections as Record<string, unknown> | undefined
  const query = data.queries as Record<string, unknown> | undefined
  const traffic = data.traffic as Record<string, unknown> | undefined

  // 用数据库服务端连接数/上限计算使用率，连接池数据作为补充展示
  const maxConns = Number(conn?.max_connections ?? 0)
  const threads = Number(conn?.threads_connected ?? 0)
  const threadsRunning = Number(conn?.threads_running ?? 0)
  const connUsage = maxConns > 0 ? (threads / maxConns) * 100 : 0

  const bytesReceived = Number(traffic?.bytes_received ?? 0)
  const bytesSent = Number(traffic?.bytes_sent ?? 0)

  const queryStats = [
    { label: 'SELECT', value: Number(query?.selects ?? 0).toLocaleString(), color: '#818cf8', lightColor: '#4f46e5' },
    { label: 'INSERT', value: Number(query?.inserts ?? 0).toLocaleString(), color: '#34d399', lightColor: '#059669' },
    { label: 'UPDATE', value: Number(query?.updates ?? 0).toLocaleString(), color: '#fbbf24', lightColor: '#d97706' },
    { label: 'DELETE', value: Number(query?.deletes ?? 0).toLocaleString(), color: '#f87171', lightColor: '#dc2626' },
  ]

  const connLevel = connUsage > 80 ? '#f87171' : connUsage > 60 ? '#fbbf24' : '#818cf8'
  const connTint =
    connUsage > 80 ? 'rgba(248, 113, 113, 0.14)' : connUsage > 60 ? 'rgba(251, 191, 36, 0.12)' : 'rgba(99, 102, 241, 0.12)'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Space>
          <span className="auto-refresh-hint">
            <span className="live-dot" />
            每 10 秒自动刷新
          </span>
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
            刷新
          </Button>
        </Space>
      </div>

      <Row gutter={[20, 20]}>
        <Col xs={24} lg={8}>
          <Card
            className="monitor-gauge-card stat-card glass-rise"
            style={{ '--tint': connTint, '--i': 0 } as React.CSSProperties}
          >
            <div className="monitor-gauge-head">
              <span className="monitor-gauge-icon" style={{ color: connLevel }}>
                <ApiOutlined />
              </span>
              <span className="monitor-gauge-title">连接使用率</span>
            </div>
            <div className="monitor-gauge-body">
              <div className="monitor-gauge-halo" style={{ '--halo': connLevel } as React.CSSProperties}>
                <Progress
                  type="dashboard"
                  percent={Math.round(connUsage)}
                  strokeColor={connLevel}
                  size={140}
                  format={(p) => <span className="monitor-gauge-value">{p}%</span>}
                />
              </div>
            </div>
            <div className="monitor-gauge-foot">
              连接 {threads} / {maxConns || '-'} · 运行中 {threadsRunning}
            </div>
          </Card>
        </Col>

        <Col xs={24} lg={8}>
          <Card
            className="stat-card glass-rise"
            style={{ height: '100%', '--tint': 'rgba(99, 102, 241, 0.1)', '--i': 1 } as React.CSSProperties}
          >
            <div className="stat-card-row" style={{ marginBottom: 16 }}>
              <div>
                <div className="stat-card-title">接收流量</div>
                <div className="stat-card-value" style={{ fontSize: 22 }}>
                  {formatBytes(bytesReceived)}
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
                <div className="stat-card-title">发送流量</div>
                <div className="stat-card-value" style={{ fontSize: 22 }}>
                  {formatBytes(bytesSent)}
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
                <span className="kv-pill kv-pill-info">{String(query?.qps ?? '0')}</span>
              </div>
              <div className="kv-row">
                <span className="kv-label">慢查询</span>
                <span className={`kv-pill ${Number(query?.slow_queries ?? 0) > 0 ? 'kv-pill-danger' : 'kv-pill-success'}`}>
                  {String(query?.slow_queries ?? '0')}
                </span>
              </div>
              <div className="kv-row">
                <span className="kv-label">表数量</span>
                <span className="kv-pill">{String(db?.table_count ?? '0')}</span>
              </div>
              <div className="kv-row">
                <span className="kv-label">库大小</span>
                <span className="kv-pill">{String(db?.size ?? '0 B')}</span>
              </div>
            </div>
          </Card>
        </Col>
      </Row>

      <Row gutter={[20, 20]}>
        <Col xs={24} lg={12}>
          <Card
            className="glass-rise"
            style={{ '--i': 3 } as React.CSSProperties}
            title={
              <Space>
                <DatabaseOutlined className="card-title-icon" /> 基本信息
              </Space>
            }
          >
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="版本">{String(data.version ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="数据库">{String(db?.name ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="地址">
                <span className="cell-mono">{db?.host ? `${db.host}:${db.port ?? ''}` : '-'}</span>
              </Descriptions.Item>
              <Descriptions.Item label="字符集 / 排序规则">
                {String(db?.charset ?? '-')} / {String(db?.collation ?? '-')}
              </Descriptions.Item>
              <Descriptions.Item label="运行时间">
                {formatDuration(Number(data.uptime_seconds ?? 0))}
              </Descriptions.Item>
              <Descriptions.Item label="历史累计连接">
                {Number(conn?.total_connections ?? 0).toLocaleString()}
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
                <ThunderboltOutlined className="card-title-icon" /> 查询统计
              </Space>
            }
          >
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              {queryStats.map((q) => (
                <div
                  key={q.label}
                  className="query-stat-tile"
                  style={{ '--qs': q.color, '--qs-light': q.lightColor } as React.CSSProperties}
                >
                  <div className="query-stat-label">{q.label}</div>
                  <div className="query-stat-value">{q.value}</div>
                </div>
              ))}
            </div>
            <Descriptions column={2} size="small" style={{ marginTop: 16 }}>
              <Descriptions.Item label="连接池 打开">{String(conn?.open_conns ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="连接池 使用中">{String(conn?.in_use ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="连接池 空闲">{String(conn?.idle ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="峰值连接">{String(conn?.max_used_connections ?? '-')}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
