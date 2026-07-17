import { useEffect, useState } from 'react'
import { Table, Button, Popconfirm, Card } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, DisconnectOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OnlineUser } from '@/types'
import { getOnlineUserList, kickUser } from '@/api/system/online-user'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
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
    // 在线会话有实时语义，静默轮询避免表格 loading 闪烁
    const timer = setInterval(() => {
      getOnlineUserList().then(setList).catch(() => {})
    }, 30000)
    return () => clearInterval(timer)
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
      render: (v: string, record) => {
        const text = [v, record.location].filter(Boolean).join(' · ')
        return text ? <span className="cell-mono">{text}</span> : <span className="cell-muted">—</span>
      },
    },
    {
      title: '浏览器 / 系统',
      dataIndex: 'browser',
      width: 180,
      render: (v: string, record) =>
        [v, record.os].filter(Boolean).join(' / ') || <span className="cell-muted">—</span>,
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
            <Button type="link" size="small" danger icon={<DisconnectOutlined />}>踢出</Button>
          </Popconfirm>
        ),
    },
  ]

  return (
    <div className="page-list online-user-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="在线用户"
          total={list.length}
          extra={
            <>
              <span className="auto-refresh-hint">
                <span className="live-dot" />
                每 30 秒自动刷新
              </span>
              <Button icon={<ReloadOutlined />} onClick={fetchList} loading={loading}>
                刷新
              </Button>
            </>
          }
        />
        <Table
          rowKey="token_id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="当前没有在线会话" compact /> }}
          pagination={{ showTotal: (t) => `共 ${t} 条`, showSizeChanger: true }}
        />
      </Card>
    </div>
  )
}
