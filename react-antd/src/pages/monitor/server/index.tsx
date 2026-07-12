import { useEffect, useState, useCallback } from 'react'
import { Card, Descriptions, Progress, Spin, Row, Col } from 'antd'
import { getServerInfo } from '@/api/monitor'

export default function ServerMonitorPage() {
  const [data, setData] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(true)

  const fetchData = useCallback(async () => {
    try {
      const res = await getServerInfo()
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

  const cpu = data.cpu as Record<string, unknown> | undefined
  const mem = data.memory as Record<string, unknown> | undefined
  const disk = data.disk as Record<string, unknown> | undefined
  const os = data.os as Record<string, unknown> | undefined
  const goRuntime = data.go as Record<string, unknown> | undefined

  const cpuUsage = Number(cpu?.usage ?? 0)
  const memUsage = Number(mem?.usage ?? 0)
  const diskUsage = Number(disk?.usage ?? 0)

  return (
    <Spin spinning={loading}>
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title="CPU 信息">
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="核心数">{String(cpu?.count ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="型号">{String(cpu?.model ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="使用率" span={2}>
                <Progress percent={Math.round(cpuUsage)} status={cpuUsage > 90 ? 'exception' : 'normal'} />
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col span={24}>
          <Card title="内存信息">
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="总内存">{String(mem?.total ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="已使用">{String(mem?.used ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="空闲">{String(mem?.free ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="使用率">
                <Progress percent={Math.round(memUsage)} status={memUsage > 90 ? 'exception' : 'normal'} />
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col span={24}>
          <Card title="磁盘信息">
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="总容量">{String(disk?.total ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="已使用">{String(disk?.used ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="空闲">{String(disk?.free ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="使用率">
                <Progress percent={Math.round(diskUsage)} status={diskUsage > 90 ? 'exception' : 'normal'} />
              </Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card title="操作系统">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="系统名称">{String(os?.name ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="平台">{String(os?.platform ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="架构">{String(os?.arch ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="主机名">{String(os?.hostname ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="运行时间">{String(os?.uptime ?? '-')}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>

        <Col xs={24} lg={12}>
          <Card title="Go 运行时">
            <Descriptions column={1} bordered size="small">
              <Descriptions.Item label="版本">{String(goRuntime?.version ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="Goroutines">{String(goRuntime?.goroutines ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="GOPATH">{String(goRuntime?.gopath ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="GOROOT">{String(goRuntime?.goroot ?? '-')}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>
    </Spin>
  )
}
