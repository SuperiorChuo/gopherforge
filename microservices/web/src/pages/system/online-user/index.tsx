import { useEffect, useState } from 'react'
import { Table, Button, Popconfirm, Card } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OnlineUser } from '@/types'
import { getOnlineUserList, kickUser } from '@/api/system/online-user'
import TableToolbar from '@/components/TableToolbar'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

export default function OnlineUserPage() {
  const [list, setList] = useState<OnlineUser[]>([])
  const [loading, setLoading] = useState(false)
  const { hasPerm } = usePermission()

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

  const handleKick = async (tokenId: string) => {
    try {
      await kickUser(tokenId)
      message.success('已踢出该用户')
      fetchList()
    } catch {
      message.error('踢出失败')
    }
  }

  const columns: ColumnsType<OnlineUser> = [
    {
      title: '用户',
      dataIndex: 'username',
      width: 180,
      render: (v: string, record) => (
        <span className="online-user-cell">
          <span className="live-dot" />
          {record.nickname ? `${v}（${record.nickname}）` : v}
        </span>
      ),
    },
    {
      title: 'Token',
      dataIndex: 'token_id',
      ellipsis: true,
      render: (v: string) => <span className="cell-mono">{v}</span>,
    },
    {
      title: 'IP / 位置',
      dataIndex: 'ip',
      width: 180,
      render: (v: string, record) => (
        <span className="cell-mono">{[v, record.location].filter(Boolean).join(' · ') || '-'}</span>
      ),
    },
    {
      title: '浏览器 / 系统',
      dataIndex: 'browser',
      width: 180,
      render: (v: string, record) => [v, record.os].filter(Boolean).join(' / ') || '-',
    },
    {
      title: '登录时间',
      dataIndex: 'login_time',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: '过期时间',
      dataIndex: 'access_token_expires_at',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: '操作',
      width: 100,
      render: (_, record) =>
        hasPerm('system:online-user:kick') && (
          <Popconfirm
            title="确认踢出该用户?"
            onConfirm={() => handleKick(record.token_id)}
          >
            <Button type="link" size="small" danger>踢出</Button>
          </Popconfirm>
        ),
    },
  ]

  return (
    <Card>
      <TableToolbar
        title="在线用户"
        total={list.length}
        extra={
          <Button icon={<ReloadOutlined />} onClick={fetchList} loading={loading}>
            刷新
          </Button>
        }
      />
      <Table
        rowKey="token_id"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ showTotal: (t) => `共 ${t} 条`, showSizeChanger: true }}
      />
    </Card>
  )
}
