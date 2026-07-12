import { useEffect, useState } from 'react'
import { Table, Button, Popconfirm, message, Card } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OnlineUser } from '@/types'
import { getOnlineUserList, kickUser } from '@/api/system/online-user'

export default function OnlineUserPage() {
  const [list, setList] = useState<OnlineUser[]>([])
  const [loading, setLoading] = useState(false)

  const fetchList = async () => {
    setLoading(true)
    try {
      const res = await getOnlineUserList()
      setList(res)
    } catch {
      message.error('获取在线用户列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList()
  }, [])

  const handleKick = async (session_id: string) => {
    try {
      await kickUser(session_id)
      message.success('已踢出该用户')
      fetchList()
    } catch {
      message.error('踢出失败')
    }
  }

  const columns: ColumnsType<OnlineUser> = [
    { title: 'Session ID', dataIndex: 'session_id', ellipsis: true },
    { title: '用户名', dataIndex: 'username', width: 140 },
    { title: 'IP', dataIndex: 'ip', width: 140 },
    { title: '最后活跃时间', dataIndex: 'last_seen_at', width: 170 },
    { title: '登录时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 100,
      render: (_, record) => (
        <Popconfirm
          title="确认踢出该用户?"
          onConfirm={() => handleKick(record.session_id)}
        >
          <Button type="link" size="small" danger>踢出</Button>
        </Popconfirm>
      ),
    },
  ]

  return (
    <Card
      title="在线用户"
      extra={
        <Button icon={<ReloadOutlined />} onClick={fetchList} loading={loading}>
          刷新
        </Button>
      }
    >
      <Table
        rowKey="session_id"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ showTotal: (t) => `共 ${t} 条`, showSizeChanger: true }}
      />
    </Card>
  )
}
