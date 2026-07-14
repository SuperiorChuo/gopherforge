import { useEffect, useState, useCallback } from 'react'
import { Card, Descriptions, Progress, Spin, Row, Col, Button, Space } from 'antd'
import {
  ReloadOutlined,
  DesktopOutlined,
  DatabaseOutlined,
  HddOutlined,
  CloudServerOutlined,
  CodeOutlined,
} from '@ant-design/icons'
import { getServerInfo } from '@/api/monitor'
import { formatBytes } from '@/utils/format'
import { useThemeMode } from '@/theme/ThemeContext'

// 用量三档色,亮色主题用更深一号保证对比度
const USAGE_COLORS = {
  dark: { high: '#f87171', mid: '#fbbf24', low: '#818cf8' },
  light: { high: '#dc2626', mid: '#d97706', low: '#2563eb' },
}

// 玻璃透光色:同色但极低透明度,由 .stat-card 的 --tint 消费
const USAGE_TINTS = {
  high: 'rgba(248, 113, 113, 0.14)',
  mid: 'rgba(251, 191, 36, 0.12)',
  low: 'rgba(99, 102, 241, 0.12)',
}

function usageLevel(pct: number): 'high' | 'mid' | 'low' {
  return pct >= 90 ? 'high' : pct >= 70 ? 'mid' : 'low'
}

interface GaugeProps {
  title: string
  icon: React.ReactNode
  percent: number
  footer: React.ReactNode
  index: number
}

function GaugeCard({ title, icon, percent, footer, index }: GaugeProps) {
  const { mode } = useThemeMode()
  const pct = Math.round(percent)
  const level = usageLevel(pct)
  const color = USAGE_COLORS[mode][level]
  return (
    <Card
      className="monitor-gauge-card stat-card glass-rise"
      style={{ '--tint': USAGE_TINTS[level], '--i': index } as React.CSSProperties}
    >
      <div className="monitor-gauge-head">
        <span className="monitor-gauge-icon" style={{ color }}>
          {icon}
        </span>
        <span className="monitor-gauge-title">{title}</span>
      </div>
      <div className="monitor-gauge-body">
        {/* 表盘背后同色辉光,玻璃内侧被仪表照亮的感觉 */}
        <div className="monitor-gauge-halo" style={{ '--halo': color } as React.CSSProperties}>
          <Progress
            type="dashboard"
            percent={pct}
            strokeColor={color}
            size={140}
            format={(p) => (
              <span className="monitor-gauge-value" style={{ color }}>{p}%</span>
            )}
          />
        </div>
      </div>
      <div className="monitor-gauge-foot">{footer}</div>
    </Card>
  )
}

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

  useEffect(() => {
    fetchData()
    const timer = setInterval(fetchData, 10000)
    return () => clearInterval(timer)
  }, [fetchData])

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

      <Row gutter={[20, 20]}>
        <Col xs={24} sm={8}>
          <GaugeCard
            title="CPU 使用率"
            icon={<DesktopOutlined />}
            percent={cpuUsage}
            index={0}
            footer={<>{String(cpu?.cores ?? '-')} 核 · {String(cpu?.model_name || '未知型号')}</>}
          />
        </Col>
        <Col xs={24} sm={8}>
          <GaugeCard
            title="内存使用率"
            icon={<DatabaseOutlined />}
            percent={memUsage}
            index={1}
            footer={<>{formatBytes(Number(mem?.used ?? 0))} / {formatBytes(Number(mem?.total ?? 0))}</>}
          />
        </Col>
        <Col xs={24} sm={8}>
          <GaugeCard
            title="磁盘使用率"
            icon={<HddOutlined />}
            percent={diskUsage}
            index={2}
            footer={<>{formatBytes(Number(disk?.used ?? 0))} / {formatBytes(Number(disk?.total ?? 0))}</>}
          />
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
