import { useCallback, useEffect, useState } from 'react'
import {
  Button, Card, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Progress, Space, Table, Tag,
} from 'antd'
import {
  DeleteOutlined, FileSearchOutlined, PlusOutlined, ReloadOutlined, SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import * as AiAPI from '@/api/ai'
import type { AiKbDocument, AiKbSearchResult } from '@/types'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  page: number
  page_size: number
}

export default function AiKnowledgePage() {
  const [list, setList] = useState<AiKbDocument[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })

  const [uploadOpen, setUploadOpen] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadForm] = Form.useForm()

  const [searchOpen, setSearchOpen] = useState(false)
  const [searching, setSearching] = useState(false)
  const [searchResults, setSearchResults] = useState<AiKbSearchResult[] | null>(null)
  const [searchForm] = Form.useForm()

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await AiAPI.getKbDocuments(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      // 拦截器已提示
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  const handleUpload = async () => {
    const values = await uploadForm.validateFields().catch(() => null)
    if (!values) return
    setUploading(true)
    try {
      const res = await AiAPI.createKbDocument(values)
      message.success(`文档已入库，切分为 ${res.chunk_count} 个分块`)
      setUploadOpen(false)
      uploadForm.resetFields()
      fetchList({ ...params, page: 1 })
    } catch {
      // 拦截器已提示
    } finally {
      setUploading(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await AiAPI.deleteKbDocument(id)
      message.success('删除成功')
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
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

  const columns: ColumnsType<AiKbDocument> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '标题', dataIndex: 'title', ellipsis: true },
    {
      title: '分块数',
      dataIndex: 'chunk_count',
      width: 100,
      render: (v: number) => <Tag color="geekblue" variant="filled">{v}</Tag>,
    },
    { title: '入库时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 90,
      render: (_, record) => (
        <Popconfirm title="删除该文档及其全部分块?" onConfirm={() => handleDelete(record.id)}>
          <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <Card>
        <TableToolbar
          title="知识库文档"
          total={total}
          icon={<FileSearchOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.4)"
          description="AI 助手的检索增强语料，文档入库后自动向量化"
          extra={
            <>
              <Button
                icon={<SearchOutlined />}
                onClick={() => {
                  setSearchResults(null)
                  searchForm.resetFields()
                  setSearchOpen(true)
                }}
              >
                检索测试
              </Button>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => { uploadForm.resetFields(); setUploadOpen(true) }}
              >
                上传文档
              </Button>
            </>
          }
        />
        <Table
          rowKey="id"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Modal
        title="上传文档"
        open={uploadOpen}
        onOk={handleUpload}
        onCancel={() => setUploadOpen(false)}
        confirmLoading={uploading}
        okText="入库"
        destroyOnHidden
        width={640}
      >
        <Form form={uploadForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入文档标题' }]}>
            <Input placeholder="如：运维手册 · 数据库备份流程" maxLength={200} />
          </Form.Item>
          <Form.Item
            name="content"
            label="内容（纯文本粘贴，入库后自动切分向量化）"
            rules={[{ required: true, message: '请粘贴文档内容' }]}
          >
            <Input.TextArea rows={12} placeholder="粘贴文档正文…" showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title="知识库检索测试"
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        width={560}
        destroyOnHidden
      >
        <Form form={searchForm} layout="vertical" onFinish={handleSearch}>
          <Form.Item name="query" label="查询语句" rules={[{ required: true, message: '请输入查询内容' }]}>
            <Input.TextArea rows={2} placeholder="输入一句话，检索最相关的文档分块" />
          </Form.Item>
          <Form.Item name="top_k" label="返回条数 (top_k)" initialValue={5}>
            <InputNumber min={1} max={20} style={{ width: 140 }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />} loading={searching}>
                检索
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
                      <Tag style={{ marginLeft: 8 }}>分块 #{r.chunk_index}</Tag>
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
    </div>
  )
}
