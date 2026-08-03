import { useEffect, useState } from 'react'
import { Table, Button, Popconfirm, Card, Space, Tag, Tooltip } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, DisconnectOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OnlineUser } from '@/types'
import { getOnlineUserList, kickUser } from '@/api/system/online-user'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'

function tokenFingerprint(tokenId: string): string {
  if (tokenId.length <= 16) return tokenId
  return `${tokenId.slice(0, 8)}...${tokenId.slice(-6)}`
}

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 在线会话有实时语义，静默轮询避免表格 loading 闪烁；
  // 后台标签页暂停，回前台立即补一次
  useVisibilityInterval(() => {
    getOnlineUserList().then(setList).catch(() => {})
  }, 30000, false)

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
      width: 220,
      ellipsis: true,
      render: (v: string, record) => {
        const text = record.nickname ? `${v}（${record.nickname}）` : v
        return (
          <span className="online-user-cell">
            <span className="live-dot" />
            <span className="list-primary-cell">{text}</span>
          </span>
        )
      },
    },
    {
      title: 'Token',
      dataIndex: 'token_id',
      width: 190,
      responsive: ['lg'],
      render: (v: string) => (
        <Tooltip title={v}>
          <Tag variant="filled" className="cell-mono list-code-tag">{tokenFingerprint(v)}</Tag>
        </Tooltip>
      ),
    },
    {
      title: 'IP / 位置',
      dataIndex: 'ip',
      width: 220,
      ellipsis: true,
      responsive: ['sm'],
      render: (v: string, record) => {
        const text = [v, record.location].filter(Boolean).join(' · ')
        return text ? <span className="cell-mono">{text}</span> : <span className="cell-muted">—</span>
      },
    },
    {
      title: '浏览器 / 系统',
      dataIndex: 'browser',
      width: 200,
      ellipsis: true,
      responsive: ['md'],
      render: (v: string, record) => {
        const text = [v, record.os].filter(Boolean).join(' / ')
        return text || <span className="cell-muted">—</span>
      },
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
      responsive: ['lg'],
      render: formatDateTime,
    },
    {
      title: '操作',
      width: 80,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <Space size={4} className="table-actions compact-table-actions">
          {hasPerm('system:online-user:kick') && (
            <Popconfirm
              title="确认踢出该用户?"
              onConfirm={() => handleKick(record.token_id)}
            >
              <Tooltip title="踢出">
                <Button
                  type="text"
                  size="small"
                  danger
                  aria-label="踢出在线用户"
                  icon={<DisconnectOutlined />}
                />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
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
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="当前没有在线会话" compact /> }}
          pagination={{ showTotal: (t) => `共 ${t} 条`, showSizeChanger: true }}
        />
      </Card>
    </div>
  )
}
