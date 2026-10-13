import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Card, Col, Form, Input, Modal, Row, Select, Space, Statistic, Table, Tag, Typography,
  Grid,
} from 'antd'
import { BookOutlined, ReloadOutlined, PlusOutlined, CheckOutlined, StopOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import request from '@/utils/request'
import * as AiAPI from '@/api/ai'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { useUrlParams } from '@/hooks/useUrlParams'

type KBMiss = {
  id: number
  site_id: number
  conversation_public_id?: string
  visitor_id?: number
  question: string
  status: string
  note?: string
  created_at: string
}

type KBStats = {
  days: number
  hits: number
  misses: number
  vector_hits: number
  keyword_hits: number
  vector_errors: number
  hit_rate: number
  today?: {
    hits: number
    misses: number
    vector_hits: number
    keyword_hits: number
  }
}

interface Params {
  status?: string
  keyword?: string
  page: number
  page_size: number
}

const STATUS_META: Record<string, { label: string; color: string }> = {
  open: { label: '待处理', color: 'orange' },
  resolved: { label: '已补充', color: 'green' },
  ignored: { label: '已忽略', color: 'default' },
}

export default function ImKbMissesPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [list, setList] = useState<KBMiss[]>([])
  const [total, setTotal] = useState(0)
  const [openCount, setOpenCount] = useState(0)
  const [loading, setLoading] = useState(false)
  const [stats, setStats] = useState<KBStats | null>(null)
  const [vectorEnabled, setVectorEnabled] = useState(false)
  const [params, setParams] = useUrlParams<Params>({ page: 1, page_size: 20, status: 'open' })
  const [searchForm] = Form.useForm()
  const [draftOpen, setDraftOpen] = useState(false)
  const [draftRow, setDraftRow] = useState<KBMiss | null>(null)
  const [draftForm] = Form.useForm()
  const [savingDraft, setSavingDraft] = useState(false)

  async function load(p: Params) {
    setLoading(true)
    try {
      const data = (await request.get('/api/v1/im/agent/kb-misses', {
        params: {
          page: p.page,
          page_size: p.page_size,
          status: p.status || undefined,
          keyword: p.keyword || undefined,
        },
      })) as { list: KBMiss[]; total: number; open_count: number }
      setList(data.list || [])
      setTotal(data.total || 0)
      setOpenCount(data.open_count || 0)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加载失败'))
    } finally {
      setLoading(false)
    }
  }

  async function loadStats() {
    try {
      const data = (await request.get('/api/v1/im/agent/kb-stats', { params: { days: 7 } })) as {
        stats: KBStats
        vector_enabled: boolean
      }
      setStats(data.stats)
      setVectorEnabled(!!data.vector_enabled)
    } catch {
      /* silent */
    }
  }

  useEffect(() => {
    void load(params)
    searchForm.setFieldsValue({ status: params.status || 'open', keyword: params.keyword })
  }, [params.page, params.page_size, params.status, params.keyword]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    void loadStats()
  }, [])

  async function setStatus(row: KBMiss, status: string) {
    try {
      await request.put(`/api/v1/im/agent/kb-misses/${row.id}`, { status })
      message.success(status === 'resolved' ? t('已标记补充') : status === 'open' ? t('已重开') : t('已忽略'))
      void load(params)
      void loadStats()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('操作失败'))
    }
  }

  function openDraft(row: KBMiss) {
    setDraftRow(row)
    draftForm.setFieldsValue({
      title: row.question.slice(0, 80) || t('访客常见问题'),
      content: t('【来自 IM 未命中回流】\n问题：{{q}}\n\n【标准答复】\n（请在此填写标准答案，保存后写入 AI 知识库并参与机器人检索）\n', { q: row.question }),
    })
    setDraftOpen(true)
  }

  async function onSaveDraft() {
    const values = await draftForm.validateFields()
    if (!draftRow) return
    setSavingDraft(true)
    try {
      const res = await AiAPI.createKbDocument({
        title: values.title,
        content: values.content,
      })
      await request.put(`/api/v1/im/agent/kb-misses/${draftRow.id}`, {
        status: 'resolved',
        note: t('已入库知识库文档 #{{m}}（{{n}} 分块）', { m: res.id, n: res.chunk_count }),
      })
      message.success(t('已写入知识库（{{n}} 个分块），并标记已补充', { n: res.chunk_count }))
      setDraftOpen(false)
      setDraftRow(null)
      void load(params)
      void loadStats()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('入库失败（请确认 AI 服务与 Embedding 已配置）'))
    } finally {
      setSavingDraft(false)
    }
  }

  const hitRatePct = stats ? Math.round((stats.hit_rate || 0) * 1000) / 10 : 0

  return (
    <div className="page-list">
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('近7日命中')} value={stats?.hits ?? 0} valueStyle={{ color: 'var(--c-success)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('近7日未命中')} value={stats?.misses ?? 0} valueStyle={{ color: 'var(--c-orange)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('命中率')} value={hitRatePct} suffix="%" valueStyle={{ color: 'var(--c-primary)' }} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('向量命中')} value={stats?.vector_hits ?? 0} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic title={t('关键词命中')} value={stats?.keyword_hits ?? 0} />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={4}>
          <Card size="small" className="glass-card">
            <Statistic
              title={t('向量通道')}
              value={vectorEnabled ? t('已启用') : t('降级关键词')}
              valueStyle={{ fontSize: 16, color: vectorEnabled ? 'var(--c-success)' : 'var(--text-tertiary)' }}
            />
          </Card>
        </Col>
      </Row>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="待补充知识"
          total={total}
          icon={<BookOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.35)"
          description="机器人知识库未命中的访客问题；可一键写入 AI 知识库"
          extra={
            <Space wrap>
              <Tag color="orange">{t('待处理')} {openCount}</Tag>
              <Button href="/ai/knowledge">{t('打开 AI 知识库')}</Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => {
                  void load(params)
                  void loadStats()
                }}
              >
                {t('刷新')}
              </Button>
            </Space>
          }
        />
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          {t('检索顺序：向量（ai-service）→ 关键词降级。未命中会记入本表；点「写入知识库」生成文档并自动标记已补充。')}
        </Typography.Paragraph>

        <Form
          form={searchForm}
          layout="inline"
          style={{ marginBottom: 16, rowGap: 8 }}
          onFinish={(v) =>
            setParams({
              ...params,
              page: 1,
              status: v.status || undefined,
              keyword: v.keyword || undefined,
            })
          }
        >
          <Form.Item name="status">
            <Select
              allowClear
              placeholder={t('状态')}
              style={{ width: 120 }}
              options={[
                { value: 'open', label: t('待处理') },
                { value: 'resolved', label: t('已补充') },
                { value: 'ignored', label: t('已忽略') },
              ]}
            />
          </Form.Item>
          <Form.Item name="keyword">
            <Input allowClear placeholder={t('问题关键词')} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit">
              {t('查询')}
            </Button>
          </Form.Item>
        </Form>

        <Table
          rowKey="id"
          loading={loading}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="暂无未命中记录" compact /> }}
          pagination={{
            current: params.page,
            pageSize: params.page_size,
            total,
            onChange: (page, pageSize) => setParams({ ...params, page, page_size: pageSize }),
          }}
          columns={[
            {
              title: t('时间'),
              dataIndex: 'created_at',
              width: 170,
              render: (v: string) => formatDateTime(v),
            },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 100,
              render: (s: string) => {
                const m = STATUS_META[s] || { label: s, color: 'default' }
                return <Tag color={m.color}>{t(m.label)}</Tag>
              },
            },
            {
              title: t('访客问题'),
              dataIndex: 'question',
              render: (q: string, row: KBMiss) => (
                <Space direction="vertical" size={0}>
                  <Typography.Text>{q}</Typography.Text>
                  {row.note ? (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {row.note}
                    </Typography.Text>
                  ) : null}
                </Space>
              ),
            },
            {
              title: t('会话'),
              dataIndex: 'conversation_public_id',
              width: 120,
              render: (id?: string) =>
                id ? (
                  <a href="/im/desk" className="cell-mono" style={{ fontSize: 12 }}>
                    {id.slice(0, 8)}
                  </a>
                ) : (
                  '—'
                ),
            },
            {
              title: t('操作'),
              width: compactActions ? 48 : 132,
              align: 'center' as const,
              fixed: 'right' as const,
              render: (_, row) => (
                <TableRowActions
                  menuOnly={compactActions}
                  maxInline={2}
                  actions={
                    row.status === 'open'
                      ? [
                          { key: 'draft', label: t('写入知识库'), icon: <PlusOutlined />, onClick: () => openDraft(row) },
                          { key: 'resolved', label: t('已补充'), icon: <CheckOutlined />, onClick: () => { void setStatus(row, 'resolved') } },
                          { key: 'ignored', label: t('忽略'), icon: <StopOutlined />, onClick: () => { void setStatus(row, 'ignored') } },
                        ]
                      : [
                          { key: 'reopen', label: t('重开'), icon: <ReloadOutlined />, onClick: () => { void setStatus(row, 'open') } },
                        ]
                  }
                />
              ),
            },
          ]}
         scroll={{ x: 960 }} />
      </Card>

      <Modal
        title={t('写入 AI 知识库')}
        open={draftOpen}
        onCancel={() => setDraftOpen(false)}
        onOk={() => void onSaveDraft()}
        confirmLoading={savingDraft}
        okText={t('入库并标记已补充')}
        width={640}
        destroyOnHidden
      >
        <Typography.Paragraph type="secondary">
          {t('将生成知识库文档（自动切片 + Embedding）。若 Embedding 未配置会失败，请先在系统设置配置 AI。')}
        </Typography.Paragraph>
        <Form form={draftForm} layout="vertical">
          <Form.Item name="title" label={t('标题')} rules={[{ required: true }]}>
            <Input maxLength={200} />
          </Form.Item>
          <Form.Item name="content" label={t('正文')} rules={[{ required: true }]}>
            <Input.TextArea rows={10} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
