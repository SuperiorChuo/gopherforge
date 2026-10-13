import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert,
  Button,
  Form,
  Input,
  Modal,
  Radio,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Grid,
} from 'antd'
import {
  DeleteOutlined, PauseCircleOutlined, PlayCircleOutlined,
  EditOutlined,
  ExperimentOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyOutlined,
} from '@ant-design/icons'
import { message } from '@/utils/feedback'
import {
  createSensitiveWords,
  deleteSensitiveWord,
  listSensitiveWords,
  testSensitiveText,
  updateSensitiveWord,
  type ImSensitiveHit,
  type ImSensitiveWord,
} from '@/api/im'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { useTableQuery } from '@/hooks/useTableQuery'

const actionOptions = [
  { label: '打码（替换为 *）', value: 'mask' },
  { label: '拦截（拒绝发送）', value: 'block' },
]

export default function ImSensitivePage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [keyword, setKeyword] = useState('')
  const [appliedKeyword, setAppliedKeyword] = useState('')
  const [actionFilter, setActionFilter] = useState<string | undefined>()
  const [statusFilter, setStatusFilter] = useState<number | undefined>()
  const [importOpen, setImportOpen] = useState(false)
  const [editRow, setEditRow] = useState<ImSensitiveWord | null>(null)
  const [testOpen, setTestOpen] = useState(false)
  const [testResult, setTestResult] = useState<{ masked_text: string; blocked: boolean; hits: ImSensitiveHit[] } | null>(null)
  const [importForm] = Form.useForm()
  const [editForm] = Form.useForm()
  const [testForm] = Form.useForm()

  const params = { page, pageSize, appliedKeyword, actionFilter, statusFilter }
  const fetcher = useCallback(async (p: typeof params) => {
    const data = await listSensitiveWords({
      keyword: p.appliedKeyword.trim() || undefined,
      action: p.actionFilter,
      status: p.statusFilter,
      page: p.page,
      page_size: p.pageSize,
    })
    return { list: data.list || [], total: data.total || 0 }
  }, [])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher,
    onError: useCallback(() => message.error(t('加载失败')), [t]),
  })

  async function onImport() {
    const values = await importForm.validateFields()
    const words = String(values.words || '')
      .split(/[\n,，、]/)
      .map((w: string) => w.trim())
      .filter(Boolean)
    if (!words.length) {
      message.warning(t('请输入敏感词'))
      return
    }
    try {
      const res = await createSensitiveWords({ words, action: values.action, tag: values.tag })
      message.success(`${t('已添加 {{n}} 个', { n: res.created })}${res.skipped ? t('，跳过重复 {{n}} 个', { n: res.skipped }) : ''}`)
      setImportOpen(false)
      importForm.resetFields()
      setPage(1)
      await reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('添加失败'))
    }
  }

  function openEdit(row: ImSensitiveWord) {
    setEditRow(row)
    editForm.setFieldsValue({ word: row.word, action: row.action, tag: row.tag, status: row.status })
  }

  async function onSaveEdit() {
    if (!editRow) return
    const values = await editForm.validateFields()
    try {
      await updateSensitiveWord(editRow.id, {
        word: values.word,
        action: values.action,
        tag: values.tag,
        status: values.status,
      })
      message.success(t('已保存'))
      setEditRow(null)
      await reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  async function onToggleStatus(row: ImSensitiveWord) {
    try {
      await updateSensitiveWord(row.id, { status: row.status === 1 ? 0 : 1 })
      message.success(row.status === 1 ? t('已停用') : t('已启用'))
      await reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('操作失败'))
    }
  }

  async function onDelete(id: number) {
    try {
      await deleteSensitiveWord(id)
      message.success(t('已删除'))
      await reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('删除失败'))
    }
  }

  async function onTest() {
    const values = await testForm.validateFields()
    try {
      const res = await testSensitiveText(values.text)
      setTestResult({ masked_text: res.masked_text, blocked: res.blocked, hits: res.hits || [] })
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('检测失败'))
    }
  }

  return (
    <ListPageShell
      className="im-sensitive-page"
      toolbar={
        <TableToolbar
          title="敏感词"
          total={total}
          icon={<SafetyOutlined />}
          gradient="linear-gradient(135deg, #f87171, #dc2626)"
          glow="rgba(220, 38, 38, 0.4)"
          description="客服会话文本消息的词库过滤，保存后热生效"
          extra={
            <Space wrap>
              <Input.Search
                placeholder={t('搜索词')}
                allowClear
                style={{ width: 180 }}
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                onSearch={() => {
                  setPage(1)
                  setAppliedKeyword(keyword)
                }}
              />
              <Select
                placeholder={t('策略')}
                allowClear
                style={{ width: 130 }}
                options={[
                  { label: t('打码'), value: 'mask' },
                  { label: t('拦截'), value: 'block' },
                ]}
                value={actionFilter}
                onChange={(v) => {
                  setPage(1)
                  setActionFilter(v)
                }}
              />
              <Select
                placeholder={t('状态')}
                allowClear
                style={{ width: 110 }}
                options={[
                  { label: t('启用'), value: 1 },
                  { label: t('停用'), value: 0 },
                ]}
                value={statusFilter}
                onChange={(v) => {
                  setPage(1)
                  setStatusFilter(v)
                }}
              />
              <Button icon={<ExperimentOutlined />} onClick={() => { setTestResult(null); setTestOpen(true) }}>
                {t('试敏测试')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setImportOpen(true)}>
                {t('添加敏感词')}
              </Button>
            </Space>
          }
        />}
      >
        <Typography.Paragraph type="secondary">
          {t('命中「打码」词的消息以')}<code>*</code>{t('替换后正常发送；命中「拦截」词的消息直接拒发。对访客与坐席双向生效（仅文本消息），词库修改后约 30 秒内全实例生效。')}
        </Typography.Paragraph>
        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="暂无敏感词，点击右上角添加" compact /> }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (p, ps) => {
              setPage(p)
              setPageSize(ps)
            },
          }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 70 },
            { title: t('敏感词'), dataIndex: 'word', render: (v: string) => <Tag variant="filled" className="cell-mono">{v}</Tag> },
            {
              title: t('策略'),
              dataIndex: 'action',
              width: 110,
              render: (v: string) =>
                v === 'block' ? <Tag color="red">{t('拦截')}</Tag> : <Tag color="orange">{t('打码')}</Tag>,
            },
            { title: t('标签'), dataIndex: 'tag', width: 120, render: (v?: string) => (v ? <Tag>{v}</Tag> : '-') },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 100,
              render: (v: number) =>
                v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
            },
            {
              title: t('创建时间'),
              dataIndex: 'created_at',
              width: 170,
              render: (v?: string) => (v ? new Date(v).toLocaleString() : '-'),
            },
            {
              title: t('操作'),
              width: compactActions ? 48 : 120,
              align: 'center' as const,
              fixed: 'right' as const,
              render: (_, row: ImSensitiveWord) => (
                <TableRowActions
                  menuOnly={compactActions}
                  maxInline={2}
                  ariaLabel={t('更多操作：{{name}}', { name: row.word })}
                  actions={[
                    { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(row) },
                    {
                      key: 'toggle',
                      label: row.status === 1 ? t('停用') : t('启用'),
                      icon: row.status === 1 ? <PauseCircleOutlined /> : <PlayCircleOutlined />,
                      onClick: () => { void onToggleStatus(row) },
                    },
                    {
                      key: 'delete',
                      label: t('删除'),
                      icon: <DeleteOutlined />,
                      danger: true,
                      confirm: t('删除该敏感词？'),
                      onClick: () => { void onDelete(row.id) },
                    },
                  ]}
                />
              ),
            },
          ]}
         scroll={{ x: 960 }} />

      <Modal
        title={t('添加敏感词')}
        open={importOpen}
        onCancel={() => setImportOpen(false)}
        onOk={() => void onImport()}
        okText={t('添加')}
      >
        <Form form={importForm} layout="vertical" initialValues={{ action: 'mask' }}>
          <Form.Item
            name="words"
            label={t('敏感词（每行一个，支持逗号分隔批量粘贴）')}
            rules={[{ required: true, message: t('请输入敏感词') }]}
          >
            <Input.TextArea rows={6} placeholder={t('骗子\n加微信\n刷单')} />
          </Form.Item>
          <Form.Item name="action" label={t('命中策略')}>
            <Radio.Group options={actionOptions.map((o) => ({ ...o, label: t(o.label) }))} />
          </Form.Item>
          <Form.Item name="tag" label={t('标签（可选，用于筛选）')}>
            <Input placeholder={t('如：辱骂 / 广告 / 引流')} maxLength={64} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editRow ? t('编辑 #{{id}}', { id: editRow.id }) : t('编辑')}
        open={!!editRow}
        onCancel={() => setEditRow(null)}
        onOk={() => void onSaveEdit()}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="word" label={t('敏感词')} rules={[{ required: true, message: t('请输入敏感词') }]}>
            <Input maxLength={128} />
          </Form.Item>
          <Form.Item name="action" label={t('命中策略')}>
            <Radio.Group options={actionOptions.map((o) => ({ ...o, label: t(o.label) }))} />
          </Form.Item>
          <Form.Item name="tag" label={t('标签')}>
            <Input maxLength={64} />
          </Form.Item>
          <Form.Item name="status" label={t('状态')}>
            <Select
              options={[
                { label: t('启用'), value: 1 },
                { label: t('停用'), value: 0 },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('试敏测试')}
        open={testOpen}
        onCancel={() => setTestOpen(false)}
        onOk={() => void onTest()}
        okText={t('检测')}
      >
        <Form form={testForm} layout="vertical">
          <Form.Item name="text" label={t('待检测文本')} rules={[{ required: true, message: t('请输入文本') }]}>
            <Input.TextArea rows={4} placeholder={t('粘贴一段消息文本，查看词库处理结果')} />
          </Form.Item>
        </Form>
        {testResult && (
          <Space direction="vertical" style={{ width: '100%' }}>
            {testResult.blocked ? (
              <Alert type="error" showIcon message={t('该文本将被拦截（命中拦截词）')} />
            ) : testResult.hits.length ? (
              <Alert type="warning" showIcon message={t('命中打码词，发送时将替换为 *')} />
            ) : (
              <Alert type="success" showIcon message={t('未命中任何敏感词')} />
            )}
            {testResult.hits.length > 0 && (
              <div>
                {t('命中：')}
                {testResult.hits.map((h, i) => (
                  <Tag key={i} color={h.action === 'block' ? 'red' : 'orange'}>
                    {h.word}
                  </Tag>
                ))}
              </div>
            )}
            {!testResult.blocked && (
              <Typography.Paragraph style={{ marginBottom: 0 }}>
                {t('处理后：')}<code>{testResult.masked_text}</code>
              </Typography.Paragraph>
            )}
          </Space>
        )}
      </Modal>
    </ListPageShell>
  )
}
