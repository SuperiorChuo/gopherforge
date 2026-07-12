import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Popconfirm, message, Card, Input, Form,
  Upload,
} from 'antd'
import { UploadOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { FileRecord } from '@/types'
import * as FileAPI from '@/api/system/file'

interface SearchParams {
  keyword?: string
  file_type?: string
  page: number
  page_size: number
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default function FilePage() {
  const [list, setList] = useState<FileRecord[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<SearchParams>({ page: 1, page_size: 10 })
  const [uploading, setUploading] = useState(false)
  const [searchForm] = Form.useForm()

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
      fetchList(params)
    } catch {
      message.error('删除失败')
    }
  }

  const beforeUpload = async (file: File) => {
    setUploading(true)
    try {
      await FileAPI.uploadFile(file)
      message.success('上传成功')
      fetchList(params)
    } catch {
      message.error('上传失败')
    } finally {
      setUploading(false)
    }
    return false
  }

  const columns: ColumnsType<FileRecord> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '文件名', dataIndex: 'file_name', ellipsis: true },
    { title: '文件类型', dataIndex: 'file_type', width: 120 },
    {
      title: '文件大小',
      dataIndex: 'file_size',
      width: 100,
      render: (v: number) => formatSize(v),
    },
    { title: '存储类型', dataIndex: 'storage_type', width: 100 },
    { title: '上传时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Popconfirm title="确认删除该文件?" onConfirm={() => handleDelete(record.id)}>
          <Button type="link" size="small" danger>删除</Button>
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch}>
          <Form.Item name="keyword">
            <Input placeholder="文件名" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="file_type">
            <Input placeholder="文件类型" allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card>
        <div style={{ marginBottom: 16 }}>
          <Upload beforeUpload={beforeUpload} showUploadList={false} multiple={false}>
            <Button type="primary" icon={<UploadOutlined />} loading={uploading}>
              上传文件
            </Button>
          </Upload>
        </div>
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
    </div>
  )
}
