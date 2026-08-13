import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Popconfirm, Card, Input, Form,
  Upload, Tag, Image, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  UploadOutlined, SearchOutlined, ReloadOutlined, DownloadOutlined, EyeOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { FileRecord } from '@/types'
import * as FileAPI from '@/api/system/file'
import ListFilterForm from '@/components/ListFilterForm'
import TableToolbar from '@/components/TableToolbar'
import CountUpValue from '@/components/CountUpValue'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import './styles.css'

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
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [uploading, setUploading] = useState(false)
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)
  const [stats, setStats] = useState<FileAPI.FileStats | null>(null)
  const [dragging, setDragging] = useState(false)
  // dragenter/leave 在子元素间反复触发,用深度计数判断是否真的离开页面
  const dragDepth = useRef(0)
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

  useEffect(() => {
    FileAPI.getFileStats().then(setStats).catch(() => setStats(null))
  }, [])

  // 上传/删除后统计卡与列表一起刷新
  const refreshStats = () => {
    FileAPI.getFileStats().then(setStats).catch(() => {})
  }

  const fetchList = useCallback(async (p: SearchParams) => {
    const res = await FileAPI.getFileList(p)
    return { list: res.list, total: res.total }
  }, [])
  const onLoadError = useCallback(() => message.error(t('获取文件列表失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onLoadError,
  })

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
      message.success(t('删除成功'))
      refreshStats()
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        void reload()
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const uploadBatch = async (files: File[]) => {
    if (!files.length) return
    setUploading(true)
    try {
      if (files.length > 1) {
        await FileAPI.uploadFiles(files)
        message.success(t('已上传 {{n}} 个文件', { n: files.length }))
      } else {
        await FileAPI.uploadFile(files[0])
        message.success(t('上传成功'))
      }
      void reload()
      refreshStats()
    } catch {
      message.error(t('上传失败'))
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
      message.error(t('下载失败'))
    }
  }

  const handlePreview = async (record: FileRecord) => {
    try {
      const url = await FileAPI.previewFile(record.id)
      setPreviewUrl(url)
    } catch {
      message.error(t('预览失败'))
    }
  }

  const closePreview = () => {
    if (previewUrl) URL.revokeObjectURL(previewUrl)
    setPreviewUrl(null)
  }

  const handleBatchDelete = async () => {
    try {
      await FileAPI.batchDeleteFiles(selectedIds)
      message.success(t('已删除 {{n}} 个文件', { n: selectedIds.length }))
      setSelectedIds([])
      refreshStats()
      if (selectedIds.length >= list.length && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        void reload()
      }
    } catch {
      message.error(t('批量删除失败'))
    }
  }

  const columns: ColumnsType<FileRecord> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['md'] },
    {
      title: t('文件名'),
      dataIndex: 'file_name',
      width: 300,
      ellipsis: { showTitle: false },
      render: (value?: string) => (
        <Tooltip title={value || undefined}>
          <span className="file-name-cell">{value || '—'}</span>
        </Tooltip>
      ),
    },
    {
      title: t('文件类型'),
      dataIndex: 'file_type',
      width: 150,
      render: (value?: string) => value ? (
        <Tooltip title={value}>
          <Tag variant="filled" className="cell-mono file-type-tag">{value}</Tag>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('文件大小'),
      dataIndex: 'file_size',
      width: 100,
      responsive: ['md'],
      render: (v: number) => <span className="cell-mono">{formatSize(v)}</span>,
    },
    {
      title: t('存储类型'),
      dataIndex: 'storage_type',
      width: 100,
      responsive: ['md'],
      render: (v: string) => v && <Tag color="geekblue" variant="filled">{v}</Tag>,
    },
    { title: t('上传时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: 136,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4} className="table-actions file-row-actions">
          {record.file_type === 'image' && (
            <Tooltip title={t('预览')}>
              <Button
                type="text"
                size="small"
                icon={<EyeOutlined />}
                aria-label={`${t('预览文件')} ${record.file_name}`}
                onClick={() => handlePreview(record)}
              />
            </Tooltip>
          )}
          <Tooltip title={t('下载')}>
            <Button
              type="text"
              size="small"
              icon={<DownloadOutlined />}
              aria-label={`${t('下载文件')} ${record.file_name}`}
              onClick={() => handleDownload(record)}
            />
          </Tooltip>
          {hasPerm('system:file:delete') && (
            <Popconfirm title={t('确认删除该文件?')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  aria-label={`${t('删除文件')} ${record.file_name}`}
                />
              </Tooltip>
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
            <div className="file-drop-title">{t('松手上传')}</div>
            <div className="file-drop-sub">{t('文件将上传到文件管理')}</div>
          </div>
        </div>
      )}
      {stats && stats.total > 0 && (
        <Card className="list-filter-card file-stats-card" bordered={false} styles={{ body: { padding: '14px 24px' } }}>
          <div className="log-stats-row">
            <div className="log-stat">
              <span className="log-stat-label">{t('文件总数')}</span>
              <span className="log-stat-value"><CountUpValue value={stats.total} /></span>
            </div>
            <div className="log-stat">
              <span className="log-stat-label">{t('占用空间')}</span>
              <span className="log-stat-value log-stat-accent">{formatSize(stats.total_size)}</span>
            </div>
            {stats.storage_quota_mb == null ? (
              <div className="log-stat">
                <span className="log-stat-label">{t('存储配额')}</span>
                <span className="log-stat-value cell-muted">{t('查询失败')}</span>
              </div>
            ) : stats.storage_quota_mb > 0 ? (
              <div className="log-stat">
                <span className="log-stat-label">{t('存储配额')}</span>
                <span className="log-stat-value">{formatSize(stats.storage_quota_mb * 1024 * 1024)}</span>
              </div>
            ) : null}
            {Object.keys(stats.by_type ?? {}).length > 0 && (
              <>
                <div className="log-stat-divider" />
                <div className="log-stat file-type-stat">
                  <span className="log-stat-label">{t('类型分布')}</span>
                  <span className="file-type-breakdown">
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
        <ListFilterForm
          form={searchForm}
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索文件名')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="file_type">
            <Input placeholder={t('文件类型')} allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="文件列表"
          total={total}
          extra={
            <Space wrap className="file-toolbar-actions">
              {selectedIds.length > 0 && hasPerm('system:file:delete') && (
                <Popconfirm
                  title={t('确认删除选中的 {{n}} 个文件?', { n: selectedIds.length })}
                  onConfirm={handleBatchDelete}
                >
                  <Button danger icon={<DeleteOutlined />}>{t('批量删除 ({{n}})', { n: selectedIds.length })}</Button>
                </Popconfirm>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
              {hasPerm('system:file:upload') && (
                <Upload beforeUpload={beforeUpload} showUploadList={false} multiple>
                  <Button type="primary" icon={<UploadOutlined />} loading={uploading}>
                    {t('上传文件')}
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
          scroll={{ x: 'max-content' }}
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
            showTotal: (n) => t('共 {{n}} 条', { n }),
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
