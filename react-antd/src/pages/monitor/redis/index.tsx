import { useCallback, useEffect, useState } from 'react'
import { Card, Descriptions, Button } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { getRedisInfo } from '@/api/monitor'

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
  }, [fetchData])

  const hits = Number(data.keyspace_hits ?? 0)
  const misses = Number(data.keyspace_misses ?? 0)
  const total = hits + misses
  const hitRate = total > 0 ? ((hits / total) * 100).toFixed(2) : '0.00'

  return (
    <Card
      title="Redis 信息"
      extra={
        <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
          刷新
        </Button>
      }
    >
      <Descriptions column={2} bordered size="small">
        <Descriptions.Item label="版本">{String(data.version ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="模式">{String(data.mode ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="连接客户端数">{String(data.connected_clients ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="已用内存">{String(data.used_memory ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="已用内存(可读)">{String(data.used_memory_human ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="运行时间(秒)">{String(data.uptime_in_seconds ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="总命令数">{String(data.total_commands_processed ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="命中次数">{String(data.keyspace_hits ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="未命中次数">{String(data.keyspace_misses ?? '-')}</Descriptions.Item>
        <Descriptions.Item label="命中率">{hitRate}%</Descriptions.Item>
      </Descriptions>
    </Card>
  )
}
