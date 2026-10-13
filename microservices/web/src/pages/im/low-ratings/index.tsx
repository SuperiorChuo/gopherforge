import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Form, Rate, Select, Space, Table, Tag, Typography, Modal, Input,
  Grid,
} from 'antd'
import {
  StarOutlined, ReloadOutlined, FileTextOutlined, MessageOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { ColumnsType } from 'antd/es/table'
import { listLowRatings, type ImLowRatedRow } from '@/api/im'
import { createTicket } from '@/api/ticket'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { useTableQuery } from '@/hooks/useTableQuery'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'

interface Params {
  max_score?: string
  days?: string
  page: number
  page_size: number
}

export default function ImLowRatingsPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [params, setParams] = useUrlParams<Params>({ page: 1, page_size: 20, max_score: '3', days: '30' })
  const [searchForm] = Form.useForm()
  const [ticketOpen, setTicketOpen] = useState(false)
  const [ticketRow, setTicketRow] = useState<ImLowRatedRow | null>(null)
  const [ticketNote, setTicketNote] = useState('')
  const [ticketSaving, setTicketSaving] = useState(false)

  const fetcher = useCallback(async (p: Params) => {
    const res = await listLowRatings({
      page: p.page,
      page_size: p.page_size,
      max_score: Number(p.max_score || 3),
      days: Number(p.days || 30),
    })
    return { list: res?.list ?? [], total: res?.total ?? 0 }
  }, [])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher,
    onError: useCallback(() => {
      message.error(t('加载失败'))
    }, [t]),
  })

  const openTicket = (row: ImLowRatedRow) => {
    setTicketRow(row)
    const score = row.rating_score ?? 0
    const comment = row.rating_comment || t('无文字评价')
    setTicketNote(
      t('访客评分 {{n}}/5\n评价：{{a}}\n会话摘要：{{b}}\n会话 ID：{{c}}', {
        n: score,
        a: comment,
        b: row.summary || row.last_message_preview || '—',
        c: row.public_id,
      }),
    )
    setTicketOpen(true)
  }

  const submitTicket = async () => {
    if (!ticketRow) return
    setTicketSaving(true)
    try {
      const score = ticketRow.rating_score ?? 0
      const tk = await createTicket({
        title: t('IM 低分复盘 ★{{n}} · {{m}}', { n: score, m: ticketRow.public_id.slice(0, 8) }),
        description: ticketNote,
        source: 'im',
        priority: score <= 2 ? 'high' : 'normal',
        conversation_public_id: ticketRow.public_id,
        im_visitor_id: ticketRow.visitor_id,
      })
      message.success(t('已建工单 {{n}}', { n: tk.ticket_no || tk.id }))
      setTicketOpen(false)
      navigate(`/ticket?id=${tk.id}`)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('建单失败'))
    } finally {
      setTicketSaving(false)
    }
  }

  const columns: ColumnsType<ImLowRatedRow> = [
    {
      title: t('评分'),
      dataIndex: 'rating_score',
      width: 140,
      render: (s?: number) =>
        s ? (
          <Space size={4}>
            <Rate disabled value={s} style={{ fontSize: 14 }} />
            <Tag color={s <= 2 ? 'red' : 'orange'}>{s}</Tag>
          </Space>
        ) : (
          '—'
        ),
    },
    {
      title: t('评价'),
      dataIndex: 'rating_comment',
      ellipsis: true,
      render: (c?: string) => c || <Typography.Text type="secondary">{t('无文字')}</Typography.Text>,
    },
    {
      title: t('坐席'),
      dataIndex: 'agent_display_name',
      width: 120,
      render: (n: string | undefined, row) => n || (row.agent_user_id ? `#${row.agent_user_id}` : '—'),
    },
    {
      title: t('预览 / 小结'),
      key: 'preview',
      ellipsis: true,
      render: (_, row) => row.summary || row.last_message_preview || '—',
    },
    {
      title: t('关闭时间'),
      dataIndex: 'closed_at',
      width: 170,
      render: (tt?: string) => (tt ? formatDateTime(tt) : '—'),
    },
    {
      title: t('会话'),
      dataIndex: 'public_id',
      width: 100,
      render: (id: string) => (
        <Typography.Text code style={{ fontSize: 12 }}>
          {id.slice(0, 8)}
        </Typography.Text>
      ),
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
          actions={[
            { key: 'open', label: t('打开'), icon: <MessageOutlined />, onClick: () => navigate(`/im/desk?cid=${row.public_id}`) },
            { key: 'ticket', label: t('转工单'), icon: <FileTextOutlined />, onClick: () => openTicket(row) },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="im-low-ratings-page"
      toolbar={
        <TableToolbar
          title="低分会话"
          total={total}
          icon={<StarOutlined />}
          gradient="linear-gradient(135deg, #fbbf24, #d97706)"
          glow="rgba(217, 119, 6, 0.35)"
          description="访客 CSAT 低分回流，便于质检复盘与一键转工单"
          extra={
            <Space>
              <Button href="/im/stats">{t('客服统计')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>
                {t('刷新')}
              </Button>
            </Space>
          }
        />}
      >
        <Form
          form={searchForm}
          layout="inline"
          style={{ marginBottom: 16, rowGap: 8 }}
          initialValues={{ max_score: params.max_score || '3', days: params.days || '30' }}
          onFinish={(v) =>
            setParams({
              ...params,
              page: 1,
              max_score: v.max_score || '3',
              days: v.days || '30',
            })
          }
        >
          <Form.Item name="max_score" label={t('最高分')}>
            <Select
              style={{ width: 120 }}
              options={[
                { value: '2', label: t('≤ 2 星') },
                { value: '3', label: t('≤ 3 星') },
                { value: '4', label: t('≤ 4 星') },
              ]}
            />
          </Form.Item>
          <Form.Item name="days" label={t('范围')}>
            <Select
              style={{ width: 120 }}
              options={[
                { value: '7', label: t('近 7 天') },
                { value: '30', label: t('近 30 天') },
                { value: '90', label: t('近 90 天') },
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
                  searchForm.setFieldsValue({ max_score: '3', days: '30' })
                  setParams({ page: 1, page_size: 20, max_score: '3', days: '30' })
                }}
              >
                {t('重置')}
              </Button>
            </Space>
          </Form.Item>
        </Form>

        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="暂无低分会话" /> }}
          pagination={{
            current: params.page,
            pageSize: params.page_size,
            total,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
         scroll={{ x: 960 }} />

      <Modal
        title={t('低分会话转工单')}
        open={ticketOpen}
        onCancel={() => setTicketOpen(false)}
        onOk={() => void submitTicket()}
        confirmLoading={ticketSaving}
        okText={t('创建工单')}
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
          {t('将评价与会话上下文写入工单描述，便于跟踪改进。')}
        </Typography.Paragraph>
        <Input.TextArea
          rows={8}
          value={ticketNote}
          onChange={(e) => setTicketNote(e.target.value)}
        />
      </Modal>
    </ListPageShell>
  )
}
