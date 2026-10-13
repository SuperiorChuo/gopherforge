import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Form, Select, Space, Table, Tag, Typography, Grid } from 'antd'
import { CheckOutlined, EyeOutlined, MailOutlined, ReloadOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { ColumnsType } from 'antd/es/table'
import {
  getNotifyChannels,
  getNotifyUnreadCount,
  listNotifyDeliveries,
  listNotifyMessages,
  markAllNotifyRead,
  markNotifyRead,
  type NotifyChannelStatus,
  type NotifyDelivery,
  type NotifyMessage,
} from '@/api/notify'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'

interface Params {
  unread?: string
  type?: string
  page: number
  page_size: number
}

const typeLabels: Record<string, { text: string; color: string }> = {
  notice: { text: '通知', color: 'blue' },
  ticket: { text: '工单', color: 'red' },
  im: { text: 'IM', color: 'green' },
  system: { text: '系统', color: 'default' },
}

export default function InboxPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [unread, setUnread] = useState(0)
  const [channels, setChannels] = useState<NotifyChannelStatus | null>(null)
  const [deliveries, setDeliveries] = useState<NotifyDelivery[]>([])
  const [params, setParams] = useUrlParams<Params>({ page: 1, page_size: 20 })
  const [searchForm] = Form.useForm()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const loadMeta = useCallback(async () => {
    try {
      const [ch, del] = await Promise.all([getNotifyChannels(), listNotifyDeliveries(10)])
      setChannels(ch ?? null)
      setDeliveries(del?.list ?? [])
    } catch {
      /* silent */
    }
  }, [])

  const fetchInbox = useCallback(async (p: Params) => {
    const [res, countRes] = await Promise.all([
      listNotifyMessages({
        page: p.page,
        page_size: p.page_size,
        unread: p.unread === '1' ? '1' : undefined,
        type: p.type || undefined,
      }),
      getNotifyUnreadCount(),
    ])
    setUnread(Number(countRes?.count ?? 0))
    return { list: res?.list ?? [], total: res?.total ?? 0 }
  }, [])
  const onLoadError = useCallback(() => {
    message.error(t('加载站内信失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchInbox, onError: onLoadError })

  useEffect(() => {
    void loadMeta()
  }, [loadMeta])

  const openItem = async (row: NotifyMessage) => {
    if (!row.read_at) {
      try {
        await markNotifyRead(row.id)
      } catch {
        /* silent */
      }
    }
    if (row.link) {
      if (row.link.startsWith('http://') || row.link.startsWith('https://')) {
        window.open(row.link, '_blank', 'noopener,noreferrer')
      } else {
        navigate(row.link)
      }
    }
    void reload()
  }

  const handleMarkAll = async () => {
    try {
      const res = await markAllNotifyRead()
      message.success(t('已读 {{n}} 条', { n: res?.updated ?? 0 }))
      void reload()
    } catch {
      message.error(t('操作失败'))
    }
  }

  const columns: ColumnsType<NotifyMessage> = [
    {
      title: t('状态'),
      dataIndex: 'read_at',
      width: 88,
      render: (v) =>
        v ? <Tag>{t('已读')}</Tag> : <Tag color="blue">{t('未读')}</Tag>,
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 96,
      render: (v: string) => {
        const meta = typeLabels[v] ?? typeLabels.notice
        return <Tag color={meta.color}>{t(meta.text)}</Tag>
      },
    },
    {
      title: t('标题'),
      dataIndex: 'title',
      ellipsis: true,
      render: (title: string, row) => (
        <Typography.Link onClick={() => void openItem(row)} style={{ fontWeight: row.read_at ? 400 : 600 }}>
          {title}
        </Typography.Link>
      ),
    },
    {
      title: t('内容'),
      dataIndex: 'content',
      ellipsis: true,
      render: (c: string) => (
        <Typography.Text type="secondary">{c}</Typography.Text>
      ),
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      width: 170,
      render: (v: string) => formatDateTime(v),
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{title}}', { title: row.title || row.id })}
          actions={[
            {
              key: 'open',
              label: t('打开'),
              icon: <EyeOutlined />,
              onClick: () => { void openItem(row) },
            },
            {
              key: 'read',
              label: t('已读'),
              icon: <CheckOutlined />,
              show: !row.read_at,
              onClick: () => {
                void (async () => {
                  try {
                    await markNotifyRead(row.id)
                    void reload()
                  } catch {
                    message.error(t('标记失败'))
                  }
                })()
              },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      toolbar={(
        <TableToolbar
          title="站内信"
          total={total}
          icon={<MailOutlined />}
          gradient="linear-gradient(135deg, #60a5fa, #2563eb)"
          glow="rgba(37, 99, 235, 0.35)"
          description={t('工单超时 / 指派 / IM 转人工；未读 {{n}} 条。外发通道可在模板 channels 与 SMTP/短信环境变量中开启。', { n: unread })}
          extra={
            <Space wrap>
              <Tag color="green">{t('站内信 开')}</Tag>
              <Tag color={channels?.email?.enabled ? 'blue' : 'default'}>
                {t('邮件')} {channels?.email?.enabled ? t('开({{p}})', { p: channels.email.provider || 'smtp' }) : t('关')}
              </Tag>
              {/* stub 只写日志、不真发短信 */}
              <Tag color={channels?.sms?.enabled ? (channels.sms.provider === 'stub' ? 'default' : 'orange') : 'default'}>
                {t('短信')} {channels?.sms?.enabled
                  ? (channels.sms.provider === 'stub' ? t('演示 stub（不真发）') : t('开({{p}})', { p: channels.sms.provider }))
                  : t('关')}
              </Tag>
              <Tag color={unread ? 'blue' : 'default'}>{t('未读')} {unread}</Tag>
              <Button type="link" onClick={() => navigate('/system/notify-templates')}>
                {t('通知模板')}
              </Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => {
                  void reload()
                  void loadMeta()
                }}
              >
                {t('刷新')}
              </Button>
              <Button type="primary" icon={<CheckOutlined />} onClick={() => void handleMarkAll()}>
                {t('全部已读')}
              </Button>
            </Space>
          }
        />
      )}
      filter={(
        <ListFilterForm
          form={searchForm}
          style={{ marginBottom: 0, rowGap: 8 }}
          initialValues={{ unread: params.unread, type: params.type }}
          onFinish={(v) =>
            setParams({
              ...params,
              page: 1,
              unread: v.unread || undefined,
              type: v.type || undefined,
            })
          }
        >
          <Form.Item name="unread" label={t('状态')}>
            <Select
              allowClear
              placeholder={t('全部')}
              style={{ width: 120 }}
              options={[{ value: '1', label: t('仅未读') }]}
            />
          </Form.Item>
          <Form.Item name="type" label={t('类型')}>
            <Select
              allowClear
              placeholder={t('全部')}
              style={{ width: 120 }}
              options={[
                { value: 'ticket', label: t('工单') },
                { value: 'im', label: 'IM' },
                { value: 'notice', label: t('通知') },
                { value: 'system', label: t('系统') },
              ]}
            />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">
                {t('查询')}
              </Button>
              <Button
                onClick={() => {
                  searchForm.resetFields()
                  setParams({ page: 1, page_size: 20 })
                }}
              >
                {t('重置')}
              </Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      )}
      >

      <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="暂无站内信" /> }}
          pagination={{
            current: params.page,
            pageSize: params.page_size,
            total,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
          onRow={(row) => ({
            style: {
              cursor: row.link ? 'pointer' : undefined,
              background: row.read_at ? undefined : 'rgba(99, 102, 241, 0.06)',
            },
          })}
         scroll={{ x: 960 }} />

        {deliveries.length > 0 && (
          <div style={{ marginTop: 24 }}>
            <Typography.Title level={5} style={{ marginTop: 0 }}>
              {t('外发记录（邮件 / 短信）')}
            </Typography.Title>
            <Table
              size="small"
              rowKey="id"
              pagination={false}
              dataSource={deliveries}
              columns={[
                {
                  title: t('通道'),
                  dataIndex: 'channel',
                  width: 80,
                  render: (c: string) => <Tag>{c}</Tag>,
                },
                {
                  title: t('状态'),
                  dataIndex: 'status',
                  width: 90,
                  render: (st: string) => {
                    const color =
                      st === 'sent' ? 'green' : st === 'stub' ? 'orange' : st === 'failed' ? 'red' : 'default'
                    return <Tag color={color}>{st}</Tag>
                  },
                },
                { title: t('目标'), dataIndex: 'target', ellipsis: true },
                { title: t('模板'), dataIndex: 'template_code', width: 140 },
                {
                  title: t('说明'),
                  dataIndex: 'detail',
                  ellipsis: true,
                  render: (d: string) => <Typography.Text type="secondary">{d}</Typography.Text>,
                },
                {
                  title: t('时间'),
                  dataIndex: 'created_at',
                  width: 170,
                  render: (v: string) => formatDateTime(v),
                },
              ]}
             scroll={{ x: 960 }} />
          </div>
        )}
    </ListPageShell>
  )
}
