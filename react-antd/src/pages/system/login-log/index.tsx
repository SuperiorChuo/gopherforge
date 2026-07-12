import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, message, Card, Input, Select, Form, DatePicker,
} from 'antd'
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { LoginLog } from '@/types'
import { getLoginLogList } from '@/api/system/log'
import dayjs from 'dayjs'

const { RangePicker } = DatePicker

interface SearchParams {
  username?: string
  ip?: string
  status?: number
  start_time?: string
  end_time?: string
  page: number
  page_size: number
}

const loginTypeLabels: Record<number, string> = { 1: '密码登录', 2: 'OAuth', 3: 'TOTP' }

export default function LoginLogPage() {
  const [list, setList] = useState<LoginLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<SearchParams>({ page: 1, page_size: 10 })
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getLoginLogList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取登录日志失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: {
    username?: string
    ip?: string
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

  const columns: ColumnsType<LoginLog> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: 'IP', dataIndex: 'ip', width: 140 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <Tag color={v === 1 ? 'success' : 'error'}>{v === 1 ? '成功' : '失败'}</Tag>,
    },
    {
      title: '登录类型',
      dataIndex: 'login_type',
      width: 100,
      render: (v: number) => loginTypeLabels[v] ?? v,
    },
    { title: '浏览器', dataIndex: 'browser', ellipsis: true },
    { title: 'OS', dataIndex: 'os', width: 120 },
    { title: '时间', dataIndex: 'created_at', width: 170 },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch}>
          <Form.Item name="username">
            <Input placeholder="用户名" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="ip">
            <Input placeholder="IP" allowClear style={{ width: 140 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>成功</Select.Option>
              <Select.Option value={0}>失败</Select.Option>
            </Select>
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
    </div>
  )
}
