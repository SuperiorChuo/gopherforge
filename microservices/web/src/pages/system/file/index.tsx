import { useEffect, useRef, useState } from 'react'
import {
  Table, Button, Space, Popconfirm, Card, Input, Form,
  Upload, Tag, Image,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  UploadOutlined, SearchOutlined, ReloadOutlined, DownloadOutlined, EyeOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { FileRecord } from '@/types'
import * as FileAPI from '@/api/system/file'
import TableToolbar from '@/components/TableToolbar'
import CountUpValue from '@/components/CountUpValue'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

interface SearchParams {
  keyword?: string
  file_type?: string
  page: number
  page_size: number
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

export default function FilePage() {
  const [list, setList] = useState<FileRecord[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [uploading, setUploading] = useState(false)
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [stats, setStats] = useState<FileAPI.FileStats | null>(null)
  const [dragging, setDragging] = useState(false)
  // dragenter/leave 在子元素间反复触发,用深度计数判断是否真的离开页面
  const dragDepth = useRef(0)
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  useEffect(() => {
    FileAPI.getFileStats().then(setStats).catch(() => setStats(null))
  }, [])

  // 上传/删除后统计卡与列表一起刷新
  const refreshStats = () => {
    FileAPI.getFileStats().then(setStats).catch(() => {})
  }

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await FileAPI.getFileList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取文件列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string; file_type?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const handleDelete = async (id: number) => {
    try {
      await FileAPI.deleteFile(id)
      message.success('删除成功')
      refreshStats()
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error('删除失败')
    }
  }

  const uploadBatch = async (files: File[]) => {
    if (!files.length) return
    setUploading(true)
    try {
      if (files.length > 1) {
        await FileAPI.uploadFiles(files)
        message.success(`已上传 ${files.length} 个文件`)
      } else {
        await FileAPI.uploadFile(files[0])
        message.success('上传成功')
      }
      fetchList(params)
      refreshStats()
    } catch {
      message.error('上传失败')
    } finally {
      setUploading(false)
    }
  }

  // antd 对多选的每个文件各调一次 beforeUpload，以首个文件为代表整批上传一次
  const beforeUpload = (file: File, fileList: File[]) => {
    if (fileList[0] === file) uploadBatch(fileList)
    return false
  }

  // 整页拖放上传:文件拖入页面任意位置即出现玻璃投放区
  const canUpload = hasPerm('system:file:upload')

  const onDragEnter = (e: React.DragEvent) => {
    if (!canUpload || !e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    dragDepth.current += 1
    setDragging(true)
  }

  const onDragLeave = (e: React.DragEvent) => {
    if (!canUpload) return
    e.preventDefault()
    dragDepth.current = Math.max(0, dragDepth.current - 1)
    if (dragDepth.current === 0) setDragging(false)
  }

  const onDrop = (e: React.DragEvent) => {
    if (!canUpload) return
    e.preventDefault()
    dragDepth.current = 0
    setDragging(false)
    uploadBatch(Array.from(e.dataTransfer.files))
  }

  const handleDownload = async (record: FileRecord) => {
    try {
      await FileAPI.downloadFile(record.id, record.file_name)
    } catch {
      message.error('下载失败')
    }
  }

  const handlePreview = async (record: FileRecord) => {
    try {
      const url = await FileAPI.previewFile(record.id)
      setPreviewUrl(url)
    } catch {
      message.error('预览失败')
    }
  }

  const closePreview = () => {
    if (previewUrl) URL.revokeObjectURL(previewUrl)
    setPreviewUrl(null)
  }

  const handleBatchDelete = async () => {
    try {
      await FileAPI.batchDeleteFiles(selectedIds)
      message.success(`已删除 ${selectedIds.length} 个文件`)
      setSelectedIds([])
      refreshStats()
      if (selectedIds.length >= list.length && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error('批量删除失败')
    }
  }

  const columns: ColumnsType<FileRecord> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '文件名', dataIndex: 'file_name', ellipsis: true },
    {
      title: '文件类型',
      dataIndex: 'file_type',
      width: 120,
      render: (v: string) => v && <Tag variant="filled" className="cell-mono">{v}</Tag>,
    },
    {
      title: '文件大小',
      dataIndex: 'file_size',
      width: 100,
      render: (v: number) => <span className="cell-mono">{formatSize(v)}</span>,
    },
    {
      title: '存储类型',
      dataIndex: 'storage_type',
      width: 100,
      render: (v: string) => v && <Tag color="geekblue" variant="filled">{v}</Tag>,
    },
    { title: '上传时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 200,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {record.file_type === 'image' && (
            <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => handlePreview(record)}>
              预览
            </Button>
          )}
          <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => handleDownload(record)}>
            下载
          </Button>
          {hasPerm('system:file:delete') && (
            <Popconfirm title="确认删除该文件?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div
      className="page-list file-page"
      onDragEnter={onDragEnter}
      onDragOver={(e) => { if (canUpload) e.preventDefault() }}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      {dragging && (
        <div className="file-drop-veil">
          <div className="file-drop-panel">
            <UploadOutlined className="file-drop-icon" />
            <div className="file-drop-title">松手上传</div>
            <div className="file-drop-sub">文件将上传到文件管理</div>
          </div>
        </div>
      )}
      {stats && stats.total > 0 && (
        <Card className="list-filter-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
          <div className="log-stats-row">
            <div className="log-stat">
              <span className="log-stat-label">文件总数</span>
              <span className="log-stat-value"><CountUpValue value={stats.total} /></span>
            </div>
            <div className="log-stat">
              <span className="log-stat-label">占用空间</span>
              <span className="log-stat-value log-stat-accent">{formatSize(stats.total_size)}</span>
            </div>
            {Object.keys(stats.by_type ?? {}).length > 0 && (
              <>
                <div className="log-stat-divider" />
                <div className="log-stat">
                  <span className="log-stat-label">类型分布</span>
                  <span>
                    {Object.entries(stats.by_type ?? {})
                      .sort((a, b) => b[1].count - a[1].count)
                      .map(([t, s]) => (
                        <Tag key={t} variant="filled">
                          {t} {s.count} · {formatSize(s.size)}
                        </Tag>
                      ))}
                  </span>
                </div>
              </>
            )}
          </div>
        </Card>
      )}

      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索文件名" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="file_type">
            <Input placeholder="文件类型" allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="文件列表"
          total={total}
          extra={
            <Space wrap>
              {selectedIds.length > 0 && hasPerm('system:file:delete') && (
                <Popconfirm
                  title={`确认删除选中的 ${selectedIds.length} 个文件?`}
                  onConfirm={handleBatchDelete}
                >
                  <Button danger>批量删除 ({selectedIds.length})</Button>
                </Popconfirm>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:file:upload') && (
                <Upload beforeUpload={beforeUpload} showUploadList={false} multiple>
                  <Button type="primary" icon={<UploadOutlined />} loading={uploading}>
                    上传文件
                  </Button>
                </Upload>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无文件，拖入文件即可上传" compact /> }}
          rowSelection={{
            selectedRowKeys: selectedIds,
            onChange: (keys) => setSelectedIds(keys as number[]),
          }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      {previewUrl && (
        <Image
          style={{ display: 'none' }}
          src={previewUrl}
          preview={{
            visible: true,
            src: previewUrl,
            onVisibleChange: (visible) => {
              if (!visible) closePreview()
            },
          }}
        />
      )}
    </div>
  )
}
