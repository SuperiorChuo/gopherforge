import { useCallback, useEffect, useState } from 'react'
import { Card, Descriptions, Button } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { getMySQLInfo } from '@/api/monitor'

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`
}

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
  }, [fetchData])

  const info = data.info as Record<string, unknown> | undefined
  const conn = data.connections as Record<string, unknown> | undefined
  const query = data.queries as Record<string, unknown> | undefined
  const traffic = data.traffic as Record<string, unknown> | undefined
  const tables = data.tables as Record<string, unknown> | undefined

  const bytesReceived = Number(traffic?.bytes_received ?? 0)
  const bytesSent = Number(traffic?.bytes_sent ?? 0)
  const tableSize = Number(tables?.size ?? 0)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card
        title="基本信息"
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
            刷新
          </Button>
        }
      >
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="版本">{String(info?.version ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="数据库">{String(info?.database ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="字符集">{String(info?.charset ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="运行时间">{String(info?.uptime ?? '-')}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="连接信息">
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="最大连接数">{String(conn?.max_open_conns ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="当前连接数">{String(conn?.open_conns ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="使用中">{String(conn?.in_use ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="空闲">{String(conn?.idle ?? '-')}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="查询统计">
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="QPS">{String(query?.qps ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="慢查询">{String(query?.slow_queries ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="SELECT">{String(query?.selects ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="INSERT">{String(query?.inserts ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="UPDATE">{String(query?.updates ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="DELETE">{String(query?.deletes ?? '-')}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="流量统计">
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="接收字节">{formatBytes(bytesReceived)}</Descriptions.Item>
          <Descriptions.Item label="发送字节">{formatBytes(bytesSent)}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="表统计">
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="表数量">{String(tables?.table_count ?? '-')}</Descriptions.Item>
          <Descriptions.Item label="总大小">{formatBytes(tableSize)}</Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  )
}
