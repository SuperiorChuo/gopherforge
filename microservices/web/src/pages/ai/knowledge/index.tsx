import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button, Drawer, Form, Input, InputNumber, Modal, Progress, Space, Table, Tag,
  Grid,
} from 'antd'
import {
  DeleteOutlined, EditOutlined, EyeOutlined, FileSearchOutlined, PlusOutlined, ReloadOutlined, SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import * as AiAPI from '@/api/ai'
import type { AiKbDocument, AiKbDocumentChunk, AiKbSearchResult } from '@/types'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  page: number
  page_size: number
}

export default function AiKnowledgePage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:kb:write')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })

  const [uploadOpen, setUploadOpen] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadForm] = Form.useForm()

  const [searchOpen, setSearchOpen] = useState(false)
  const [searching, setSearching] = useState(false)
  const [searchResults, setSearchResults] = useState<AiKbSearchResult[] | null>(null)
  const [searchForm] = Form.useForm()

  const [editOpen, setEditOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editLoading, setEditLoading] = useState(false)
  const [editForm] = Form.useForm()

  const [viewOpen, setViewOpen] = useState(false)
  const [viewLoading, setViewLoading] = useState(false)
  const [viewDoc, setViewDoc] = useState<AiKbDocument | null>(null)
  const [viewChunks, setViewChunks] = useState<AiKbDocumentChunk[]>([])

  const fetchList = useCallback((p: SearchParams) => AiAPI.getKbDocuments(p), [])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
  })

  const handleUpload = async () => {
    const values = await uploadForm.validateFields().catch(() => null)
    if (!values) return
    setUploading(true)
    try {
      const res = await AiAPI.createKbDocument(values)
      message.success(t('文档已入库，切分为 {{n}} 个分块', { n: res.chunk_count }))
      setUploadOpen(false)
      uploadForm.resetFields()
      setParams({ ...params, page: 1 })
      void reload()
    } catch {
      // 拦截器已提示
    } finally {
      setUploading(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await AiAPI.deleteKbDocument(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        void reload()
      }
    } catch {
      // 拦截器已提示
    }
  }

  const handleSearch = async () => {
    const values = await searchForm.validateFields().catch(() => null)
    if (!values) return
    setSearching(true)
    try {
      const res = await AiAPI.searchKb({ query: values.query, top_k: values.top_k || undefined })
      setSearchResults(res.list)
    } catch {
      setSearchResults(null)
    } finally {
      setSearching(false)
    }
  }

  const handleEditOpen = async (id: number) => {
    try {
      const doc = await AiAPI.getKbDocument(id)
      editForm.setFieldsValue({ title: doc.title, content: doc.content ?? '' })
      setEditingId(id)
      setEditOpen(true)
    } catch {
      // 拦截器已提示
    }
  }

  const handleEditSave = async () => {
    if (editingId === null) return
    const values = await editForm.validateFields().catch(() => null)
    if (!values) return
    setEditLoading(true)
    try {
      const res = await AiAPI.updateKbDocument(editingId, values)
      message.success(t('文档已更新，重新切分为 {{n}} 个分块', { n: res.chunk_count }))
      setEditOpen(false)
      editForm.resetFields()
      void reload()
    } catch {
      // 拦截器已提示
    } finally {
      setEditLoading(false)
    }
  }

  const handleViewOpen = async (id: number) => {
    setViewLoading(true)
    try {
      const [doc, chunks] = await Promise.all([
        AiAPI.getKbDocument(id),
        AiAPI.getKbDocumentChunks(id),
      ])
      setViewDoc(doc)
      setViewChunks(chunks)
      setViewOpen(true)
    } catch {
      // 拦截器已提示
    } finally {
      setViewLoading(false)
    }
  }

  const columns: ColumnsType<AiKbDocument> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: t('标题'), dataIndex: 'title', ellipsis: true },
    {
      title: t('分块数'),
      dataIndex: 'chunk_count',
      width: 100,
      render: (v: number) => <Tag color="geekblue" variant="filled">{v}</Tag>,
    },
    { title: t('入库时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: compactActions ? 48 : 120,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'view', label: t('查看'), icon: <EyeOutlined />, onClick: () => handleViewOpen(record.id) },
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, show: canWrite, onClick: () => handleEditOpen(record.id) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: canWrite,
              confirm: t('删除该文档及其全部分块?'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="ai-knowledge-page"
      toolbar={
        <TableToolbar
          title="知识库文档"
          total={total}
          icon={<FileSearchOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.4)"
          description="AI 助手的检索增强语料，文档入库后自动向量化"
          extra={
            <Space wrap>
              <Button
                icon={<SearchOutlined />}
                onClick={() => {
                  setSearchResults(null)
                  searchForm.resetFields()
                  setSearchOpen(true)
                }}
              >
                {t('检索测试')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
              {canWrite && (
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => { uploadForm.resetFields(); setUploadOpen(true) }}
                >
                  {t('上传文档')}
                </Button>
              )}
            </Space>
          }
        />}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="知识库为空，先上传一篇文档" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
         scroll={{ x: 960 }} />

      <Modal
        title={t('上传文档')}
        open={uploadOpen}
        onOk={handleUpload}
        onCancel={() => setUploadOpen(false)}
        confirmLoading={uploading}
        okText={t('入库')}
        destroyOnHidden
        width={640}
      >
        <Form form={uploadForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('请输入文档标题') }]}>
            <Input placeholder={t('如：运维手册 · 数据库备份流程')} maxLength={200} />
          </Form.Item>
          <Form.Item
            name="content"
            label={t('内容（纯文本粘贴，入库后自动切分向量化）')}
            rules={[{ required: true, message: t('请粘贴文档内容') }]}
          >
            <Input.TextArea rows={12} placeholder={t('粘贴文档正文…')} showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('编辑文档')}
        open={editOpen}
        onOk={handleEditSave}
        onCancel={() => setEditOpen(false)}
        confirmLoading={editLoading}
        okText={t('保存并重新入库')}
        destroyOnHidden
        width={640}
      >
        <Form form={editForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('请输入文档标题') }]}>
            <Input maxLength={200} />
          </Form.Item>
          <Form.Item
            name="content"
            label={t('内容（保存后删除旧分块并重新向量化）')}
            rules={[{ required: true, message: t('请粘贴文档内容') }]}
          >
            <Input.TextArea rows={12} showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={t('知识库检索测试')}
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        width={560}
        destroyOnHidden
      >
        <Form form={searchForm} layout="vertical" onFinish={handleSearch}>
          <Form.Item name="query" label={t('查询语句')} rules={[{ required: true, message: t('请输入查询内容') }]}>
            <Input.TextArea rows={2} placeholder={t('输入一句话，检索最相关的文档分块')} />
          </Form.Item>
          <Form.Item name="top_k" label={t('返回条数 (top_k)')} initialValue={5}>
            <InputNumber min={1} max={20} style={{ width: 140 }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />} loading={searching}>
                {t('检索')}
              </Button>
            </Space>
          </Form.Item>
        </Form>

        {searchResults !== null && (
          searchResults.length === 0 ? (
            <GlassEmpty text="没有命中任何分块" compact />
          ) : (
            <div className="ai-kb-results">
              {searchResults.map((r, idx) => (
                <div key={`${r.document_id}-${r.chunk_index}-${idx}`} className="ai-kb-result glass-well">
                  <div className="ai-kb-result-head">
                    <span className="ai-kb-result-title">
                      {r.title}
                      <Tag style={{ marginLeft: 8 }}>{t('分块 #{{n}}', { n: r.chunk_index })}</Tag>
                    </span>
                    <span className="ai-kb-result-score">
                      <Progress
                        type="circle"
                        size={36}
                        percent={Math.round(Math.max(0, Math.min(1, r.score)) * 100)}
                        format={(p) => `${p}`}
                      />
                    </span>
                  </div>
                  <div className="ai-kb-result-content">{r.content}</div>
                </div>
              ))}
            </div>
          )
        )}
      </Drawer>

      <Drawer
        title={viewDoc?.title || t('文档详情')}
        open={viewOpen}
        onClose={() => setViewOpen(false)}
        width={560}
        destroyOnHidden
        loading={viewLoading}
      >
        {viewDoc && (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <Tag color="geekblue" variant="filled">{t('{{n}} 个分块', { n: viewDoc.chunk_count })}</Tag>
              {viewDoc.created_at && <span style={{ color: 'var(--text-tertiary)' }}>{formatDateTime(viewDoc.created_at)}</span>}
            </div>
            {viewDoc.content && (
              <div style={{ marginTop: 16 }}>
                <div style={{ fontWeight: 600, marginBottom: 8, color: 'var(--text-secondary)' }}>{t('全文')}</div>
                <pre className="ai-kb-result-content">{viewDoc.content}</pre>
              </div>
            )}
            <div style={{ marginTop: 16 }}>
              <div style={{ fontWeight: 600, marginBottom: 8, color: 'var(--text-secondary)' }}>{t('分块预览')}</div>
              {viewChunks.length === 0 ? (
                <GlassEmpty text={t('该文档暂无分块（嵌入可能未完成）')} compact />
              ) : (
                viewChunks.map((c) => (
                  <div key={c.id} className="glass-well" style={{ marginBottom: 12, padding: 12 }}>
                    <div style={{ marginBottom: 8 }}>
                      <Tag>{t('分块 #{{n}}', { n: c.chunk_index })}</Tag>
                    </div>
                    <pre className="ai-kb-result-content">{c.content}</pre>
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </Drawer>
    </ListPageShell>
  )
}
