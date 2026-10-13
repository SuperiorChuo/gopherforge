import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Card, Input, Form, Modal, Tabs, Tag, Radio,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, SafetyCertificateOutlined, FileTextOutlined,
  ProfileOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listQcResults, listQcKeywords, createQcKeyword, deleteQcKeyword, getTranscript,
  type QcResult, type QcKeyword, type QcHit,
} from '@/api/cc'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'

function parseHits(raw: string): QcHit[] {
  try {
    const arr = JSON.parse(raw || '[]')
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

// ---- Tab 1：质检结果 ----
function QcResultsTab() {
  const { t } = useTranslation()
  const [list, setList] = useState<QcResult[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  // 查看全文弹窗
  const [viewing, setViewing] = useState<{ callId: string; text: string; hits: QcHit[] } | null>(null)
  const [loadingText, setLoadingText] = useState(false)

  const fetchList = useCallback(async (p: number, ps: number) => {
    setLoading(true)
    try {
      const res = await listQcResults({ page: p, page_size: ps })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('获取质检结果失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { fetchList(page, pageSize) }, [fetchList, page, pageSize])

  const handleViewText = async (row: QcResult) => {
    setLoadingText(true)
    try {
      const tr = await getTranscript(row.call_id)
      setViewing({ callId: row.call_id, text: tr.text, hits: parseHits(row.hits) })
    } catch {
      message.error(t('尚无转写全文（可在话单页转写）'))
    } finally {
      setLoadingText(false)
    }
  }

  const columns: ColumnsType<QcResult> = [
    { title: t('时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('主叫'),
      dataIndex: 'caller',
      width: 140,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('被叫'),
      dataIndex: 'callee',
      width: 140,
      render: (v: string) => <span className="cell-mono">{v || '—'}</span>,
    },
    {
      title: t('坐席'),
      dataIndex: 'agent_ext',
      width: 100,
      render: (v: string) => (v ? <span className="cell-mono">{v}</span> : <span className="cell-dim">—</span>),
    },
    {
      title: t('评分'),
      dataIndex: 'score',
      width: 100,
      render: (v: number) => (
        <Tag color={v >= 80 ? 'green' : v >= 60 ? 'orange' : 'red'} variant="filled">{t('{{n}} 分', { n: v })}</Tag>
      ),
    },
    {
      title: t('命中词'),
      key: 'hits',
      render: (_, row) => {
        const hits = parseHits(row.hits)
        if (!hits.length) return <span className="cell-dim">{t('无')}</span>
        return (
          <Space size={4} wrap>
            {hits.map((h) => (
              <Tag key={h.word} color={h.type === 'forbidden' ? 'red' : h.count > 0 ? 'green' : 'default'}>
                {h.type === 'forbidden' ? t('禁') : t('必')} {h.word}
                {h.type === 'forbidden' ? ` ×${h.count}` : h.count > 0 ? ' ✓' : t('缺失')}
              </Tag>
            ))}
          </Space>
        )
      },
    },
    {
      title: t('操作'),
      key: 'action',
      width: 48,
      align: 'center' as const,
      render: (_, row) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'view',
              label: t('查看全文'),
              icon: <FileTextOutlined />,
              loading: loadingText,
              onClick: () => handleViewText(row),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <Card className="list-main-card" bordered={false}>
      <TableToolbar
        title="质检结果"
        total={total}
        extra={<Button icon={<ReloadOutlined />} onClick={() => fetchList(page, pageSize)}>{t('刷新')}</Button>}
      />
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        scroll={{ x: 1000 }}
        locale={{ emptyText: <GlassEmpty text="暂无质检结果（在话单页对有录音的通话点转写质检）" compact /> }}
        pagination={{
          total,
          current: page,
          pageSize,
          showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
      />

      <Modal
        title={<Space><FileTextOutlined />{t('转写全文')}</Space>}
        open={!!viewing}
        onCancel={() => setViewing(null)}
        footer={null}
        destroyOnHidden
        width={720}
      >
        {viewing && (
          <div
            style={{
              maxHeight: 420,
              overflow: 'auto',
              padding: 12,
              borderRadius: 8,
              background: 'rgba(128,128,160,0.08)',
              lineHeight: 1.9,
              whiteSpace: 'pre-wrap',
            }}
          >
            {viewing.text || <span className="cell-dim">{t('（空）')}</span>}
          </div>
        )}
      </Modal>
    </Card>
  )
}

// ---- Tab 2：关键词库 ----
function QcKeywordsTab() {
  const { t } = useTranslation()
  const [list, setList] = useState<QcKeyword[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<{ word: string; type: 'forbidden' | 'required'; remark?: string }>()

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listQcKeywords()
      setList(res.list ?? [])
    } catch {
      message.error(t('获取关键词库失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { fetchList() }, [fetchList])

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ type: 'forbidden' })
    setModalOpen(true)
  }

  const handleSave = async (values: { word: string; type: 'forbidden' | 'required'; remark?: string }) => {
    setSaving(true)
    try {
      await createQcKeyword({ word: values.word.trim(), type: values.type, remark: values.remark?.trim() })
      message.success(t('关键词已添加'))
      setModalOpen(false)
      fetchList()
    } catch (e) {
      message.error((e as Error)?.message || t('添加失败（可能已存在）'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteQcKeyword(id)
      message.success(t('已删除'))
      fetchList()
    } catch (e) {
      message.error((e as Error)?.message || t('删除失败'))
    }
  }

  const columns: ColumnsType<QcKeyword> = [
    {
      title: t('关键词'),
      dataIndex: 'word',
      width: 220,
      render: (v: string) => <span className="cell-mono">{v}</span>,
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 140,
      render: (v: string) =>
        v === 'forbidden'
          ? <Tag color="red" variant="filled">{t('违禁词')}</Tag>
          : <Tag color="green" variant="filled">{t('必说话术')}</Tag>,
    },
    {
      title: t('备注'),
      dataIndex: 'remark',
      render: (v: string) => v || <span className="cell-dim">—</span>,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      key: 'action',
      width: 48,
      align: 'center' as const,
      render: (_, r) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('删除关键词「{{word}}」？', { word: r.word }),
              onClick: () => { void handleDelete(r.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <Card className="list-main-card" bordered={false}>
      <TableToolbar
        title="关键词库"
        total={list.length}
        extra={
          <Space size={16}>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建关键词')}</Button>
            <Button icon={<ReloadOutlined />} onClick={fetchList}>{t('刷新')}</Button>
          </Space>
        }
      />
      <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
        {t('评分规则：满分 100；转写文本命中违禁词每次 -20，必说话术缺失每条 -10，最低 0 分。')}
      </div>
      <Table
        rowKey="id"
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
        locale={{ emptyText: <GlassEmpty text="暂无关键词" compact /> }}
       scroll={{ x: 960 }} />

      <Modal
        title={t('新建关键词')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saving}
        okText={t('添加')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleSave} style={{ marginTop: 8 }}>
          <Form.Item
            name="word"
            label={t('关键词')}
            rules={[{ required: true, message: t('请输入关键词') }, { max: 128, message: t('≤128 字') }]}
          >
            <Input placeholder={t('如：保证 / 一定 / 您好，很高兴为您服务')} />
          </Form.Item>
          <Form.Item name="type" label={t('类型')} rules={[{ required: true }]}>
            <Radio.Group>
              <Radio.Button value="forbidden">{t('违禁词')}</Radio.Button>
              <Radio.Button value="required">{t('必说话术')}</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item name="remark" label={t('备注（可选）')}>
            <Input placeholder={t('如：合规红线 / 开场白规范')} maxLength={256} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

export default function CcQcPage() {
  const { t } = useTranslation()
  return (
    <div className="page-list cc-qc-page">
      <Tabs
        defaultActiveKey="results"
        items={[
          {
            key: 'results',
            label: <Space size={6}><SafetyCertificateOutlined />{t('质检结果')}</Space>,
            children: <QcResultsTab />,
          },
          {
            key: 'keywords',
            label: <Space size={6}><ProfileOutlined />{t('关键词库')}</Space>,
            children: <QcKeywordsTab />,
          },
        ]}
      />
    </div>
  )
}
