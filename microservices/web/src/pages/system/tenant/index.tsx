import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip } from 'antd'
import { PlusOutlined, ReloadOutlined, SwapOutlined, SearchOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { message } from '@/utils/feedback'
import type { ColumnsType } from 'antd/es/table'
import type { TenantInfo, TenantPackageInfo } from '@/types'
import * as TenantAPI from '@/api/system/tenant'
import { getAllTenantPackages } from '@/api/system/tenantPackages'
import EntityFormModal from '@/components/EntityFormModal'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'
import { useAppSelector } from '@/hooks/store'
import { clearActTenantId, getActTenantId, setActTenantId } from '@/utils/request'
import { useConfirmAction } from '@/hooks/useConfirmAction'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import './styles.css'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

const planColors: Record<string, string> = { enterprise: 'gold', pro: 'blue' }

export default function TenantPage() {
  const { t } = useTranslation()
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const isPlatform = !!userInfo?.is_platform_admin
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 20 })
  const [createOpen, setCreateOpen] = useState(false)
  const [createdAdmin, setCreatedAdmin] = useState<TenantAPI.TenantCreateResult['admin']>(null)
  const editModal = useCrudModal<TenantInfo>()
  const [actTenant, setActTenant] = useState<string | null>(getActTenantId())
  const [packages, setPackages] = useState<TenantPackageInfo[]>([])
  const [form] = Form.useForm()
  const [editForm] = Form.useForm()
  const [searchForm] = Form.useForm()

  const fetchList = useCallback(async (p: SearchParams) => {
    const data = (await TenantAPI.getTenantList({
      page: p.page,
      page_size: p.page_size,
      keyword: p.keyword || undefined,
    })) as { list?: TenantInfo[]; total?: number }
    const rows = data.list || []
    return { list: rows, total: data.total ?? rows.length }
  }, [])
  const onLoadError = useCallback((error?: unknown) => {
    message.error(error instanceof Error ? error.message : t('加载失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  useEffect(() => {
    // 权限套餐下拉与列名映射（无权限或接口失败时静默降级为仅显示 ID）
    getAllTenantPackages()
      .then((res) => setPackages(res || []))
      .catch(() => {})
  }, [])

  const handleSearch = (values: { keyword?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 20 })
  }

  async function onCreate() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      const res = await TenantAPI.createTenant({
        code: values.code,
        name: values.name,
        plan: values.plan || 'free',
        // 0 = use plan default on server (free→10, pro→50, enterprise→unlimited)
        max_users: values.max_users ?? 0,
        status: 1,
        // 权限套餐：缺省 = 不限
        package_id: values.package_id,
      })
      setCreateOpen(false)
      form.resetFields()
      setParams({ ...params, page: 1 })
      if (res.admin) {
        // 开通自动创建了初始管理员：一次性展示凭据，转交租户管理员后关闭
        setCreatedAdmin(res.admin)
      } else {
        message.success(t('已创建租户'))
      }
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建失败'))
    }
  }

  const confirmAction = useConfirmAction()
  function onDelete(row: TenantInfo) {
    void confirmAction.run({
      action: () => TenantAPI.deleteTenant(row.id),
      onSuccess: () => {
        message.success(t('已删除租户「{{name}}」及其账号体系数据', { name: row.name }))
        setParams({ ...params, page: 1 })
      },
      onError: () => message.error(t('删除失败')),
    })
  }

  function openEdit(row: TenantInfo) {
    editModal.openEdit(row)
    editForm.setFieldsValue({
      name: row.name,
      plan: row.plan,
      max_users: row.max_users,
      status: row.status,
      package_id: row.package_id ?? undefined,
    })
  }

  async function onSaveEdit() {
    if (!editModal.record) return
    const values = await editForm.validateFields().catch(() => null)
    if (!values) return
    try {
      await TenantAPI.updateTenant(editModal.record.id, {
        name: values.name,
        plan: values.plan,
        max_users: values.max_users,
        status: values.status,
        // 清空选择 → 0（解绑）
        package_id: values.package_id ?? 0,
      })
      message.success(t('已保存'))
      editModal.close()
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  function actAs(row: TenantInfo) {
    setActTenantId(row.id)
    setActTenant(String(row.id))
    message.success(t('已切换操作租户为 {{code}}（后续请求带 X-Act-Tenant-ID）', { code: row.code }))
  }

  function clearAct() {
    clearActTenantId()
    setActTenant(null)
    message.success(t('已取消租户切换，回到本账号所属租户'))
  }

  const activeTenant = list.find((row) => String(row.id) === actTenant)
  const activeTenantLabel = activeTenant
    ? `${activeTenant.name} (${activeTenant.code})`
    : `ID=${actTenant}`

  const columns: ColumnsType<TenantInfo> = [
    { title: 'ID', dataIndex: 'id', width: 70, responsive: ['lg'] },
    {
      title: 'Code',
      dataIndex: 'code',
      width: 180,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono list-code-tag">{v}</Tag>,
    },
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (value: string, row) => (
        <span className="tenant-name-cell">
          <span className="list-primary-cell">{value}</span>
          {actTenant === String(row.id) && (
            <Tag color="blue" variant="filled" className="tenant-current-tag">{t('当前')}</Tag>
          )}
        </span>
      ),
    },
    {
      title: t('计费方案'),
      dataIndex: 'plan',
      width: 110,
      responsive: ['md'],
      render: (v: string) => (
        <Tag variant="filled" color={planColors[v] ?? 'default'}>{v || 'free'}</Tag>
      ),
    },
    {
      title: t('权限套餐'),
      dataIndex: 'package_id',
      width: 130,
      responsive: ['lg'],
      render: (v: number | null | undefined) => {
        if (!v) return <Tag variant="filled">{t('不限')}</Tag>
        const pkg = packages.find((p) => p.id === v)
        return <Tag variant="filled" color="purple">{pkg ? pkg.name : `#${v}`}</Tag>
      },
    },
    {
      title: t('用户上限'),
      dataIndex: 'max_users',
      width: 100,
      responsive: ['lg'],
      render: (v: number) => (v > 0 ? <span className="cell-mono">{v}</span> : t('不限')),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (v: number) =>
        v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
    },
    {
      title: t('操作'),
      width: 144,
      fixed: 'right',
      render: (_, row) => (
        <Space size={4} className="table-actions tenant-row-actions">
          <Tooltip title={t('编辑')}>
            <Button type="text" size="small" aria-label={t('编辑租户 {{name}}', { name: row.name })} icon={<EditOutlined />} onClick={() => openEdit(row)} />
          </Tooltip>
          {isPlatform && (
            <Tooltip title={row.status === 1 ? t('以该租户身份操作业务数据') : t('停用租户不可进入')}>
              <span>
                <Button
                  size="small"
                  type={actTenant === String(row.id) ? 'primary' : 'default'}
                  className="tenant-enter-button"
                  aria-label={t('进入租户 {{name}}', { name: row.name })}
                  icon={<SwapOutlined />}
                  disabled={row.status !== 1}
                  onClick={() => actAs(row)}
                >
                  {actTenant === String(row.id) ? t('当前') : t('进入')}
                </Button>
              </span>
            </Tooltip>
          )}
          {isPlatform && row.id !== 1 && (
            <Popconfirm
              title={t('删除租户「{{name}}」？', { name: row.name })}
              description={t('将级联删除该租户的用户/角色/部门/岗位及配置，且不可恢复。')}
              okText={t('删除')}
              okButtonProps={{ danger: true }}
              onConfirm={() => void onDelete(row)}
            >
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除租户 {{name}}', { name: row.name })} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
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
          message={t('平台运营账号（platform_admin）')}
          description={
            actTenant
              ? t('当前以租户 {{name}} 操作数据。用户/角色/部门等列表将只显示该租户。', { name: activeTenantLabel })
              : t('可点击「进入」切换操作租户；仅影响本机后续 API 的 X-Act-Tenant-ID。')
          }
          action={
            actTenant ? (
              <Button size="small" onClick={clearAct}>
                {t('取消切换')}
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
            <Input placeholder={t('搜索 Code / 名称')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
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
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                {t('新建租户')}
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
          rowClassName={(row) => {
            if (row.status !== 1) return 'list-row-disabled'
            return actTenant === String(row.id) ? 'tenant-row-active' : ''
          }}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无租户" compact /> }}
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

      <Modal title={t('新建租户')} open={createOpen} onCancel={() => setCreateOpen(false)} onOk={() => void onCreate()}>
        <Form form={form} layout="vertical" initialValues={{ plan: 'free', max_users: 0 }}>
          <Form.Item
            name="code"
            label="Code"
            rules={[{ required: true, message: t('必填') }]}
            extra={t('登录可用 tenant_code 或子域名 acme.example.com')}
          >
            <Input placeholder="acme" />
          </Form.Item>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true }]}>
            <Input placeholder={t('Acme 公司')} />
          </Form.Item>
          <Form.Item name="plan" label={t('计费方案')} extra={t('max_users=0 时：free→10、pro→50、enterprise→不限')}>
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label={t('最大用户数（0=按计费方案默认）')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="package_id" label={t('权限套餐')} extra={t('不选 = 不限；绑定后租户内角色只能分配套餐内权限')}>
            <Select
              allowClear
              placeholder={t('不限')}
              options={packages.map((p) => ({
                label: p.status === 1 ? p.name : t('{{name}}（已停用）', { name: p.name }),
                value: p.id,
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      {createdAdmin ? (
        <Modal
          title={t('租户已创建，请记录初始管理员凭据')}
          open
          onCancel={() => setCreatedAdmin(null)}
          footer={<Button type="primary" onClick={() => setCreatedAdmin(null)}>{t('我已保存，关闭')}</Button>}
          maskClosable={false}
        >
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message={t('此凭据只显示这一次，请立即转交租户管理员并提醒首次登录后修改密码。')}
          />
          <Card size="small">
            <div className="cell-mono" style={{ fontSize: 14, marginBottom: 8 }}>
              {t('账号')}：{createdAdmin.username}
            </div>
            <div className="cell-mono" style={{ fontSize: 14 }}>
              {t('初始密码')}：{createdAdmin.initial_password}
            </div>
          </Card>
        </Modal>
      ) : null}

      <EntityFormModal
        title={editModal.record ? t('编辑租户 #{{id}}', { id: editModal.record.id }) : t('编辑')}
        open={editModal.open}
        form={editForm}
        onClose={editModal.close}
        onSubmit={onSaveEdit}
      >
          <Form.Item name="name" label={t('名称')} rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="plan" label={t('计费方案')}>
            <Select
              options={[
                { label: 'free', value: 'free' },
                { label: 'pro', value: 'pro' },
                { label: 'enterprise', value: 'enterprise' },
              ]}
            />
          </Form.Item>
          <Form.Item name="max_users" label={t('最大用户数（0=不限）')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="package_id" label={t('权限套餐')} extra={t('清空 = 解绑（不限）；改小套餐不回收存量角色权限，仅拦截新分配')}>
            <Select
              allowClear
              placeholder={t('不限')}
              options={packages.map((p) => ({
                label: p.status === 1 ? p.name : t('{{name}}（已停用）', { name: p.name }),
                value: p.id,
              }))}
            />
          </Form.Item>
          <Form.Item name="status" label={t('状态')}>
            <Select
              options={[
                { label: t('启用'), value: 1 },
                { label: t('停用'), value: 0 },
              ]}
            />
          </Form.Item>
      </EntityFormModal>
    </div>
  )
}
