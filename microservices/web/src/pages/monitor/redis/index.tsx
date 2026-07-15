import { useCallback, useEffect, useState } from 'react'
import { Card, Descriptions, Button, Row, Col, Progress, Space, Tag } from 'antd'
import {
  ReloadOutlined,
  ThunderboltOutlined,
  TeamOutlined,
  DatabaseOutlined,
  KeyOutlined,
} from '@ant-design/icons'
import { getRedisInfo } from '@/api/monitor'
import { formatDuration } from '@/utils/format'

interface MiniStat {
  label: string
  value: React.ReactNode
  icon: React.ReactNode
  gradient: string
  shadow: string
  tint: string
}

export default function RedisMonitorPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(false)

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getRedisInfo()
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

  const server = data.server as Record<string, unknown> | undefined
  const memory = data.memory as Record<string, unknown> | undefined
  const stats = data.stats as Record<string, unknown> | undefined
  const clients = data.clients as Record<string, unknown> | undefined
  const keyspace = data.keyspace as Record<string, unknown> | undefined

  const hits = Number(stats?.keyspace_hits ?? 0)
  const misses = Number(stats?.keyspace_misses ?? 0)
  const total = hits + misses
  const hitRate = total > 0 ? (hits / total) * 100 : 0

  const cards: MiniStat[] = [
    {
      label: '连接客户端',
      value: String(clients?.connected ?? '-'),
      icon: <TeamOutlined />,
      gradient: 'linear-gradient(135deg, #818cf8, #4f46e5)',
      shadow: 'rgba(79, 70, 229, 0.35)',
      tint: 'rgba(99, 102, 241, 0.13)',
    },
    {
      label: '已用内存',
      value: String(memory?.used ?? '-'),
      icon: <DatabaseOutlined />,
      gradient: 'linear-gradient(135deg, #22d3ee, #0891b2)',
      shadow: 'rgba(8, 145, 178, 0.35)',
      tint: 'rgba(6, 182, 212, 0.12)',
    },
    {
      label: '每秒操作数',
      value: String(stats?.ops ?? '0'),
      icon: <ThunderboltOutlined />,
      gradient: 'linear-gradient(135deg, #fbbf24, #f59e0b)',
      shadow: 'rgba(245, 158, 11, 0.35)',
      tint: 'rgba(245, 158, 11, 0.11)',
    },
    {
      label: 'Key 数量',
      value: Number(keyspace?.dbsize ?? 0).toLocaleString(),
      icon: <KeyOutlined />,
      gradient: 'linear-gradient(135deg, #34d399, #059669)',
      shadow: 'rgba(5, 150, 105, 0.35)',
      tint: 'rgba(16, 185, 129, 0.12)',
    },
  ]

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
            title="缓存命中率"
            className="glass-rise"
            style={{ height: '100%', '--i': 4 } as React.CSSProperties}
          >
            <div style={{ display: 'flex', justifyContent: 'center', padding: '12px 0' }}>
              <div className="monitor-gauge-halo" style={{ '--halo': '#34d399' } as React.CSSProperties}>
                <Progress
                  type="circle"
                  percent={Math.round(hitRate * 100) / 100}
                  strokeColor={{ '0%': '#6366f1', '100%': '#34d399' }}
                  size={160}
                  format={(p) => (
                    <span className="monitor-gauge-value" style={{ fontSize: 24 }}>{p}%</span>
                  )}
                />
              </div>
            </div>
            <div className="monitor-gauge-foot" style={{ textAlign: 'center' }}>
              命中 {hits.toLocaleString()} · 未命中 {misses.toLocaleString()}
            </div>
          </Card>
        </Col>

        <Col xs={24} lg={16}>
          <Card
            className="glass-rise"
            title={
              <Space>
                Redis 详情
                <Tag color={server?.mode ? 'processing' : 'default'}>{String(server?.mode ?? '-')}</Tag>
              </Space>
            }
            style={{ height: '100%', '--i': 5 } as React.CSSProperties}
          >
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="版本">{String(server?.version ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="运行时间">
                {formatDuration(Number(server?.uptime_seconds ?? 0))}
              </Descriptions.Item>
              <Descriptions.Item label="系统">{String(server?.os ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="端口">{String(server?.tcp_port ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="内存峰值">{String(memory?.peak ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="内存上限">{String(memory?.maxmemory ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="碎片率">{String(memory?.fragmentation ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="阻塞客户端">{String(clients?.blocked ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="累计连接">
                {Number(stats?.total_connections_received ?? 0).toLocaleString()}
              </Descriptions.Item>
              <Descriptions.Item label="累计命令">
                {Number(stats?.total_commands_processed ?? 0).toLocaleString()}
              </Descriptions.Item>
              <Descriptions.Item label="过期 Key">
                {Number(stats?.expired_keys ?? 0).toLocaleString()}
              </Descriptions.Item>
              <Descriptions.Item label="淘汰 Key">
                {Number(stats?.evicted_keys ?? 0).toLocaleString()}
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
