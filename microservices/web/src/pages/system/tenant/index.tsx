import { useEffect, useState } from 'react'
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Select, Space, Table, Tag, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined, SwapOutlined, SearchOutlined, EditOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import type { ColumnsType } from 'antd/es/table'
import type { TenantInfo } from '@/types'
import * as TenantAPI from '@/api/system/tenant'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'
import { useAppSelector } from '@/hooks/store'
import { clearActTenantId, getActTenantId, setActTenantId } from '@/utils/request'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

const planColors: Record<string, string> = { enterprise: 'gold', pro: 'blue' }

export default function TenantPage() {
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const isPlatform = !!userInfo?.is_platform_admin
  const [list, setList] = useState<TenantInfo[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 20 })
  const [createOpen, setCreateOpen] = useState(false)
  const [editRow, setEditRow] = useState<TenantInfo | null>(null)
  const [actTenant, setActTenant] = useState<string | null>(getActTenantId())
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const data = (await TenantAPI.getTenantList({
        page: p.page,
        page_size: p.page_size,
        keyword: p.keyword || undefined,
      })) as { list?: TenantInfo[]; total?: number }
      const rows = data.list || []
      setList(rows)
      setTotal(data.total ?? rows.length)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params])

  const handleSearch = (values: { keyword?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 20 })
  }

  async function onCreate() {
    const values = await form.validateFields()
    try {
      await TenantAPI.createTenant({
        code: values.code,
        name: values.name,
        plan: values.plan || 'free',
        // 0 = use plan default on server (free→10, pro→50, enterprise→unlimited)
        max_users: values.max_users ?? 0,
        status: 1,
      })
      message.success('已创建租户')
      setCreateOpen(false)
      form.resetFields()
      setParams({ ...params, page: 1 })
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  function openEdit(row: TenantInfo) {
    setEditRow(row)
    editForm.setFieldsValue({
      name: row.name,
      plan: row.plan,
      max_users: row.max_users,
      status: row.status,
    })
  }

  async function onSaveEdit() {
    if (!editRow) return
    const values = await editForm.validateFields()
    try {
      await TenantAPI.updateTenant(editRow.id, {
        name: values.name,
        plan: values.plan,
        max_users: values.max_users,
        status: values.status,
      })
      message.success('已保存')
      setEditRow(null)
      fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    }
  }

  function actAs(row: TenantInfo) {
    setActTenantId(row.id)
    setActTenant(String(row.id))
    message.success(`已切换操作租户为 ${row.code}（后续请求带 X-Act-Tenant-ID）`)
  }

  function clearAct() {
    clearActTenantId()
    setActTenant(null)
    message.success('已取消租户切换，回到本账号所属租户')
  }

  const columns: ColumnsType<TenantInfo> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: 'Code',
      dataIndex: 'code',
      width: 160,
      render: (v: string) => <Tag variant="filled" className="cell-mono">{v}</Tag>,
    },
    { title: '名称', dataIndex: 'name' },
    {
      title: '套餐',
      dataIndex: 'plan',
      width: 110,
      render: (v: string) => (
        <Tag variant="filled" color={planColors[v] ?? 'default'}>{v || 'free'}</Tag>
      ),
    },
    {
      title: '用户上限',
      dataIndex: 'max_users',
      width: 100,
      render: (v: number) => (v > 0 ? <span className="cell-mono">{v}</span> : '不限'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: number) =>
        v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
    },
    {
      title: '操作',
      width: 200,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(row)}>
            编辑
          </Button>
          {isPlatform && (
            <Tooltip title="以该租户身份操作业务数据">
              <Button
                size="small"
                type={actTenant === String(row.id) ? 'primary' : 'default'}
                icon={<SwapOutlined />}
                onClick={() => actAs(row)}
              >
                进入
              </Button>
            </Tooltip>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list tenant-page">
      {isPlatform && (
        <Alert
          type="info"
          showIcon
          message="平台运营账号（platform_admin）"
          description={
            actTenant
              ? `当前以租户 ID=${actTenant} 操作数据。用户/角色/部门等列表将只显示该租户。`
              : '可点击「进入」切换操作租户；仅影响本机后续 API 的 X-Act-Tenant-ID。'
          }
          action={
            actTenant ? (
              <Button size="small" onClick={clearAct}>
                取消切换
              </Button>
            ) : null
          }
        />
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
            <Input placeholder="搜索 Code / 名称" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
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
          title="租户管理"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                新建租户
              </Button>
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          dataSource={list}
          columns={columns}
          locale={{ emptyText: <GlassEmpty text="暂无租户" compact /> }}
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

      <Modal title="新建租户" open={createOpen} onCancel={() => setCreateOpen(false)} onOk={() => void onCreate()}>
        <Form form={form} layout="vertical" initialValues={{ plan: 'free', max_users: 0 }}>
          <Form.Item
            name="code"
            label="Code"
            rules={[{ required: true, message: '必填' }]}
            extra="登录可用 tenant_code 或子域名 acme.example.com"
          >
            <Input placeholder="acme" />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="Acme 公司" />
          </Form.Item>
          <Form.Item name="plan" label="套餐" extra="max_users=0 时：free→10、pro→50、enterprise→不限">
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label="最大用户数（0=按套餐默认）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={editRow ? `编辑租户 #${editRow.id}` : '编辑'}
        open={!!editRow}
        onCancel={() => setEditRow(null)}
        onOk={() => void onSaveEdit()}
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="plan" label="套餐">
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label="最大用户数（0=不限）">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select
              options={[
                { label: '启用', value: 1 },
                { label: '停用', value: 0 },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
