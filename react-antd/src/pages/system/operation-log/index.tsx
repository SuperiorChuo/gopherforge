import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, message, Card, Input, Select, Form, Modal, Descriptions, DatePicker,
} from 'antd'
import { SearchOutlined, ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { OperationLog } from '@/types'
import { getOperationLogList, getOperationLogDetail } from '@/api/system/log'
import dayjs from 'dayjs'

const { RangePicker } = DatePicker

interface SearchParams {
  username?: string
  method?: string
  path?: string
  module?: string
  status?: number
  start_time?: string
  end_time?: string
  page: number
  page_size: number
}

function statusColor(status: number): string {
  if (status >= 500) return 'error'
  if (status >= 400) return 'warning'
  if (status >= 200 && status < 300) return 'success'
  return 'default'
}

export default function OperationLogPage() {
  const [list, setList] = useState<OperationLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<SearchParams>({ page: 1, page_size: 10 })
  const [detailOpen, setDetailOpen] = useState(false)
  const [detail, setDetail] = useState<OperationLog | null>(null)
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getOperationLogList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取操作日志失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: {
    username?: string
    method?: string
    path?: string
    module?: string
    status?: number
    dateRange?: [dayjs.Dayjs, dayjs.Dayjs]
  }) => {
    const { dateRange, ...rest } = values
    setParams({
      ...params,
      page: 1,
      ...rest,
      start_time: dateRange?.[0]?.format('YYYY-MM-DD HH:mm:ss'),
      end_time: dateRange?.[1]?.format('YYYY-MM-DD HH:mm:ss'),
    })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openDetail = async (id: number) => {
    try {
      const res = await getOperationLogDetail(id)
      setDetail(res)
      setDetailOpen(true)
    } catch {
      message.error('获取详情失败')
    }
  }

  const columns: ColumnsType<OperationLog> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: '方法', dataIndex: 'method', width: 80 },
    { title: '路径', dataIndex: 'path', ellipsis: true },
    { title: '模块', dataIndex: 'module', width: 100 },
    { title: '动作', dataIndex: 'action', width: 100 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <Tag color={statusColor(v)}>{v}</Tag>,
    },
    { title: '时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => openDetail(record.id)}>
          详情
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch}>
          <Form.Item name="username">
            <Input placeholder="用户名" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="method">
            <Select placeholder="方法" style={{ width: 90 }} allowClear>
              {['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].map((m) => (
                <Select.Option key={m} value={m}>{m}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="path">
            <Input placeholder="路径" allowClear style={{ width: 160 }} />
          </Form.Item>
          <Form.Item name="module">
            <Input placeholder="模块" allowClear style={{ width: 120 }} />
          </Form.Item>
          <Form.Item name="dateRange">
            <RangePicker showTime format="YYYY-MM-DD HH:mm:ss" />
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
        title="操作日志详情"
        open={detailOpen}
        onCancel={() => setDetailOpen(false)}
        footer={null}
        width={640}
      >
        {detail && (
          <Descriptions column={2} bordered size="small" style={{ marginTop: 16 }}>
            <Descriptions.Item label="ID">{detail.id}</Descriptions.Item>
            <Descriptions.Item label="用户名">{detail.username}</Descriptions.Item>
            <Descriptions.Item label="方法">{detail.method}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={statusColor(detail.status)}>{detail.status}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="路径" span={2}>{detail.path}</Descriptions.Item>
            <Descriptions.Item label="模块">{detail.module}</Descriptions.Item>
            <Descriptions.Item label="动作">{detail.action}</Descriptions.Item>
            <Descriptions.Item label="请求ID" span={2}>{detail.request_id}</Descriptions.Item>
            <Descriptions.Item label="时间" span={2}>{detail.created_at}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </div>
  )
}
