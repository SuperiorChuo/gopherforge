import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Card, Input, Select, Form, Drawer, Descriptions, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import { SearchOutlined, ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getAuditLogList, type AuditLog, type AuditLogListResult } from '@/api/system/audit-log'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import './styles.css'

interface SearchParams {
  keyword?: string
  action?: string
  target_type?: string
  page: number
  page_size: number
}

function JsonBlock({ title, data }: { title: string; data?: Record<string, unknown> }) {
  if (!data || Object.keys(data).length === 0) return null
  return (
    <div className="log-detail-block">
      <div className="log-detail-block-title">{title}</div>
      <pre>{JSON.stringify(data, null, 2)}</pre>
    </div>
  )
}

export default function AuditLogPage() {
  const [list, setList] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [facets, setFacets] = useState<AuditLogListResult['facets'] | null>(null)
  const [detail, setDetail] = useState<AuditLog | null>(null)
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getAuditLogList(p)
      setList(res.items ?? [])
      setTotal(res.pagination?.total ?? 0)
      if (res.facets) setFacets(res.facets)
    } catch {
      message.error('获取审计日志失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string; action?: string; target_type?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const columns: ColumnsType<AuditLog> = [
    { title: 'ID', dataIndex: 'id', width: 70, responsive: ['lg'] },
    {
      title: '操作者',
      dataIndex: 'actor_id',
      width: 160,
      responsive: ['sm'],
      render: (v: string, record) => (
        <div className="audit-actor-cell">
          <Tag variant="filled" className="audit-actor-type">{record.actor_type}</Tag>
          <Tooltip title={v}>
            <span className="cell-mono audit-actor-id">{v || '—'}</span>
          </Tooltip>
        </div>
      ),
    },
    {
      title: '动作',
      dataIndex: 'action',
      width: 200,
      render: (v: string) => (
        <Tooltip title={v}>
          <Tag color="geekblue" variant="filled" className="cell-mono audit-action-tag">{v}</Tag>
        </Tooltip>
      ),
    },
    {
      title: '目标',
      dataIndex: 'target_type',
      width: 220,
      responsive: ['md'],
      render: (v: string, record) => {
        const target = `${v}#${record.target_id}`
        return (
          <Tooltip title={target}>
            <span className="cell-mono cell-dim audit-target-cell">{target}</span>
          </Tooltip>
        )
      },
    },
    { title: '摘要', dataIndex: 'summary', width: 300, ellipsis: true, responsive: ['lg'] },
    { title: '时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['md'] },
    {
      title: '操作',
      width: 64,
      fixed: 'right',
      render: (_, record) => (
        <Tooltip title="查看详情">
          <Button
            type="text"
            size="small"
            className="audit-row-action"
            aria-label="查看审计日志详情"
            icon={<EyeOutlined />}
            onClick={() => setDetail(record)}
          />
        </Tooltip>
      ),
    },
  ]

  return (
    <div className="page-list audit-log-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索关键字" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="action">
            <Select placeholder="动作" style={{ width: 180 }} allowClear showSearch>
              {(facets?.actions ?? []).map((a) => (
                <Select.Option key={a} value={a}>{a}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="target_type">
            <Select placeholder="目标类型" style={{ width: 150 }} allowClear showSearch>
              {(facets?.target_types ?? []).map((t) => (
                <Select.Option key={t} value={t}>{t}</Select.Option>
              ))}
            </Select>
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
          title="审计日志"
          total={total}
          extra={<Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>}
        />
        <Table
          rowKey="id"
          className="list-table audit-log-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无审计记录" compact /> }}
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

      <Drawer
        title="审计详情"
        open={!!detail}
        onClose={() => setDetail(null)}
        width="min(720px, 100vw)"
        destroyOnHidden
      >
        {detail && (
          <div className="audit-detail-content">
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label="ID">{detail.id}</Descriptions.Item>
              <Descriptions.Item label="时间">{formatDateTime(detail.created_at)}</Descriptions.Item>
              <Descriptions.Item label="操作者">
                <Tag variant="filled">{detail.actor_type}</Tag>
                <span className="cell-mono audit-detail-long">{detail.actor_id}</span>
              </Descriptions.Item>
              <Descriptions.Item label="动作">
                <Tag color="geekblue" variant="filled" className="cell-mono audit-detail-tag">{detail.action}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="目标" span={2}>
                <span className="cell-mono audit-detail-long">{detail.target_type}#{detail.target_id}</span>
              </Descriptions.Item>
              {detail.summary && (
                <Descriptions.Item label="摘要" span={2}>{detail.summary}</Descriptions.Item>
              )}
            </Descriptions>
            <JsonBlock title="变更前 (before)" data={detail.before} />
            <JsonBlock title="变更后 (after)" data={detail.after} />
          </div>
        )}
      </Drawer>
    </div>
  )
}
