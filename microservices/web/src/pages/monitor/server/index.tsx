import { useEffect, useState, useCallback } from 'react'
import { Card, Descriptions, Spin, Row, Col, Button, Space } from 'antd'
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
import MonitorGaugeCard from '@/components/MonitorGaugeCard'

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
          <MonitorGaugeCard
            title="CPU 使用率"
            icon={<DesktopOutlined />}
            percent={cpuUsage}
            index={0}
            footer={<>{String(cpu?.cores ?? '-')} 核 · {String(cpu?.model_name || '未知型号')}</>}
          />
        </Col>
        <Col xs={24} sm={8}>
          <MonitorGaugeCard
            title="内存使用率"
            icon={<DatabaseOutlined />}
            percent={memUsage}
            index={1}
            footer={<>{formatBytes(Number(mem?.used ?? 0))} / {formatBytes(Number(mem?.total ?? 0))}</>}
          />
        </Col>
        <Col xs={24} sm={8}>
          <MonitorGaugeCard
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
