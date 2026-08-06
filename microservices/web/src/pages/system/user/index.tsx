import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select, Alert,
  Card, Row, Col, Avatar, Tooltip, Switch,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, UserOutlined, EditOutlined, DeleteOutlined,
  DownloadOutlined, UploadOutlined, KeyOutlined, MailOutlined, TeamOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemUser, SystemRole, Department } from '@/types'
import * as UserAPI from '@/api/system/user'
import { createInvite } from '@/api/system/invite'
import { getRoleList } from '@/api/system/role'
import { getDepartmentList } from '@/api/system/department'
import { getAllPosts } from '@/api/system/posts'
import type { SystemPost } from '@/api/system/posts'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import ExcelImportModal from '@/components/ExcelImportModal'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
import './styles.css'

const avatarPalette = [
  'linear-gradient(135deg, #818cf8, #4f46e5)',
  'linear-gradient(135deg, #38bdf8, #0284c7)',
  'linear-gradient(135deg, #34d399, #059669)',
  'linear-gradient(135deg, #fbbf24, #d97706)',
  'linear-gradient(135deg, #f472b6, #db2777)',
  'linear-gradient(135deg, #a78bfa, #7c3aed)',
]
const roleTagPalette = ['geekblue', 'cyan', 'purple', 'magenta', 'gold']

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

// 用户行：后端用户列表 / 详情会附带岗位摘要（posts）与岗位 id 数组（post_ids）
type UserRow = SystemUser & { posts?: SystemPost[]; post_ids?: number[] }

function UserTagList({
  items,
  emptyText = '—',
  colorize = false,
}: {
  items?: Array<{ id: number; name?: string; code?: string }>
  emptyText?: string
  colorize?: boolean
}) {
  const { t } = useTranslation()
  if (!items?.length) return <span className="cell-muted">{t(emptyText)}</span>

  const visible = items.slice(0, 2)
  const remaining = items.length - visible.length
  return (
    <Space size={6} className="user-tag-list">
      {visible.map((item, index) => (
        <Tag
          key={item.id}
          color={colorize ? roleTagPalette[index % roleTagPalette.length] : undefined}
          variant="filled"
        >
          {item.name || item.code}
        </Tag>
      ))}
      {remaining > 0 && (
        <Tooltip title={items.slice(2).map((item) => item.name || item.code).join('、')}>
          <Tag className="user-tag-more" variant="filled">+{remaining}</Tag>
        </Tooltip>
      )}
    </Space>
  )
}

export default function UserPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<UserRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<UserRow | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [roles, setRoles] = useState<SystemRole[]>([])
  const [depts, setDepts] = useState<Department[]>([])
  const [posts, setPosts] = useState<SystemPost[]>([])
  const [importOpen, setImportOpen] = useState(false)
  const [inviteOpen, setInviteOpen] = useState(false)
  const [inviteLink, setInviteLink] = useState<string | null>(null)
  const [inviting, setInviting] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [resetRecord, setResetRecord] = useState<UserRow | null>(null)
  const [resetting, setResetting] = useState(false)
  const [form] = Form.useForm()
  const [resetForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const [inviteForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const deptNameMap = useMemo(() => {
    const m = new Map<number, string>()
    depts.forEach((d) => m.set(d.id, d.name))
    return m
  }, [depts])

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await UserAPI.getUserList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取用户列表失败'))
    } finally {
      setLoading(false)
    }
  }

  const fetchRoles = async () => {
    try {
      const res = await getRoleList({ page: 1, page_size: 200 })
      setRoles(res.list)
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  useEffect(() => {
    fetchRoles()
    getDepartmentList({ page: 1, page_size: 200 })
      .then((res) => setDepts(res.list))
      .catch(() => {
        // ignore
      })
    getAllPosts()
      .then((res) => setPosts(res ?? []))
      .catch(() => {
        // ignore
      })
  }, [])

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, keyword: values.keyword, status: values.status })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: UserRow) => {
    setEditRecord(record)
    form.setFieldsValue({
      username: record.username,
      nickname: record.nickname,
      email: record.email,
      phone: record.phone,
      status: record.status,
      department_id: record.department_id,
      role_ids: record.roles?.map((r) => r.id) ?? [],
      post_ids: record.post_ids ?? record.posts?.map((p) => p.id) ?? [],
    })
    setModalOpen(true)
  }

  const openReset = (record: UserRow) => {
    setResetRecord(record)
    resetForm.resetFields()
  }

  const submitReset = async () => {
    const values = await resetForm.validateFields().catch(() => null)
    if (!values || !resetRecord) return
    setResetting(true)
    try {
      await UserAPI.resetUserPassword(resetRecord.id, values.password, values.must_change)
      message.success(t('密码已重置，该用户的登录会话已全部失效'))
      setResetRecord(null)
    } catch (err: unknown) {
      message.error(err instanceof Error ? err.message : t('重置失败'))
    } finally {
      setResetting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await UserAPI.deleteUser(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const { role_ids, ...rest } = values
      if (editRecord) {
        // 部门被清空时显式传 0（移出部门）；不传后端按"不修改"处理
        await UserAPI.updateUser(editRecord.id, { ...rest, department_id: rest.department_id ?? 0 })
        if (typeof rest.status === 'number' && rest.status !== editRecord.status) {
          await UserAPI.updateUserStatus(editRecord.id, rest.status)
        }
        if (role_ids !== undefined) {
          await UserAPI.assignUserRoles(editRecord.id, role_ids ?? [])
        }
        message.success(t('更新成功'))
      } else {
        const created = await UserAPI.createUser(rest)
        if (role_ids?.length) {
          await UserAPI.assignUserRoles(created.id, role_ids)
        }
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<UserRow> = [
    {
      title: t('用户'),
      dataIndex: 'username',
      width: 220,
      render: (_, record) => (
        <div className="user-cell">
          <Avatar
            size={40}
            // antd 优先级 icon > children：有名字时不能传 icon，否则首字母永远不显示
            icon={record.nickname || record.username ? undefined : <UserOutlined />}
            style={{ background: avatarPalette[record.id % avatarPalette.length], flexShrink: 0 }}
          >
            {(record.nickname || record.username)?.slice(0, 1).toUpperCase()}
          </Avatar>
          <div className="user-cell-text">
            <div className="user-cell-name">{record.username}</div>
            <div className="user-cell-sub">
              {record.nickname || <span className="cell-muted">{t('未设置昵称')}</span>}
            </div>
          </div>
        </div>
      ),
    },
    {
      title: t('联系方式'),
      key: 'contact',
      width: 240,
      ellipsis: true,
      responsive: ['sm'],
      render: (_, record) => (
        <div className="user-contact">
          <div className="user-contact-main">
            {record.email || <span className="cell-muted">—</span>}
          </div>
          {record.phone && <div className="user-contact-sub">{record.phone}</div>}
        </div>
      ),
    },
    {
      title: t('部门'),
      dataIndex: 'department_id',
      width: 140,
      ellipsis: true,
      responsive: ['md'],
      render: (id?: number) =>
        id && deptNameMap.get(id) ? (
          deptNameMap.get(id)
        ) : (
          <span className="cell-muted">—</span>
        ),
    },
    {
      title: t('岗位'),
      dataIndex: 'posts',
      width: 180,
      responsive: ['lg'],
      render: (userPosts: UserRow['posts']) => <UserTagList items={userPosts} />,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 96,
      responsive: ['sm'],
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: t('角色'),
      dataIndex: 'roles',
      width: 220,
      responsive: ['md'],
      render: (roles: SystemUser['roles']) => (
        <UserTagList items={roles} emptyText="未分配" colorize />
      ),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 168,
      className: 'cell-time',
      responsive: ['lg'],
      render: formatDateTime,
    },
    {
      title: t('操作'),
      width: 132,
      render: (_, record) => (
        <Space size={4} className="table-actions user-row-actions">
          {hasPerm('system:user:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={t('编辑用户')} icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:user:update') && (
            <Tooltip title={t('重置密码')}>
              <Button type="text" size="small" aria-label={t('重置用户密码')} icon={<KeyOutlined />} onClick={() => openReset(record)} />
            </Tooltip>
          )}
          {hasPerm('system:user:delete') && (
            <Popconfirm title={t('确认删除该用户？')} description={t('删除后不可恢复')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除用户')} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  async function onInvite() {
    const values = await inviteForm.validateFields().catch(() => null)
    if (!values) return
    setInviting(true)
    try {
      const res = await createInvite({ role_id: values.role_id, email: values.email })
      setInviteLink(res.link)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('创建邀请失败'))
    } finally {
      setInviting(false)
    }
  }

  return (
    <div className="page-list user-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input
              placeholder={t('搜索用户名 / 邮箱 / 手机')}
              prefix={<SearchOutlined />}
              allowClear
              style={{ width: 260 }}
            />
          </Form.Item>
          <Form.Item name="status">
            <Select
              placeholder={t('全部状态')}
              style={{ width: 128 }}
              allowClear
              options={[
                { label: t('启用'), value: 1 },
                { label: t('禁用'), value: 0 },
              ]}
            />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                {t('查询')}
              </Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>
                {t('重置')}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="用户列表"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>
                {t('刷新')}
              </Button>
              <Button
                icon={<DownloadOutlined />}
                loading={exporting}
                onClick={() => {
                  setExporting(true)
                  void UserAPI.exportUsers({ keyword: params.keyword, status: params.status })
                    .catch(() => {})
                    .finally(() => setExporting(false))
                }}
              >
                {t('导出')}
              </Button>
              {hasPerm('system:user:create') && (
                <Button icon={<UploadOutlined />} onClick={() => setImportOpen(true)}>
                  {t('导入')}
                </Button>
              )}
              {hasPerm('system:user:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                  {t('新增用户')}
                </Button>
              )}
              {hasPerm('system:user:create') && (
                <Button icon={<MailOutlined />} onClick={() => { setInviteLink(null); inviteForm.resetFields(); setInviteOpen(true) }}>
                  {t('邀请用户')}
                </Button>
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
          rowClassName={(record) => (record.status === 0 ? 'user-row-disabled' : '')}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无用户" compact /> }}
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

      <Modal
        className="user-form-modal"
        title={editRecord ? t('编辑用户') : t('新增用户')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
        okText={editRecord ? t('保存') : t('创建')}
      >
        <Form form={form} layout="vertical" className="user-form" style={{ marginTop: 8 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="username" label={t('用户名')} rules={[{ required: true, message: t('请输入用户名') }]}>
                <Input disabled={!!editRecord} placeholder={t('登录账号')} autoComplete="off" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="nickname" label={t('昵称')}>
                <Input placeholder={t('显示名称')} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name="email"
                label={t('邮箱')}
                rules={[
                  { type: 'email', message: t('邮箱格式不正确') },
                  ...(editRecord ? [{ required: true, message: t('请输入邮箱') }] : []),
                ]}
              >
                <Input placeholder="name@example.com" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="phone" label={t('手机号')}>
                <Input placeholder={t('可选')} />
              </Form.Item>
            </Col>
          </Row>
          {!editRecord && (
            <Form.Item
              name="password"
              label={t('密码')}
              rules={[
                { required: true, message: t('请输入密码') },
                { min: 6, message: t('密码至少 6 位') },
              ]}
            >
              <Input.Password placeholder={t('至少 6 位')} autoComplete="new-password" />
            </Form.Item>
          )}
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="status" label={t('状态')} initialValue={1}>
                <Select
                  options={[
                    { label: t('启用'), value: 1 },
                    { label: t('禁用'), value: 0 },
                  ]}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="department_id" label={t('部门')}>
                <Select
                  allowClear
                  showSearch
                  placeholder={t('请选择部门')}
                  optionFilterProp="label"
                  options={depts.map((d) => ({ label: d.name, value: d.id }))}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="role_ids" label={t('角色')}>
            <Select
              mode="multiple"
              placeholder={t('请选择角色')}
              optionFilterProp="label"
              options={roles.map((r) => ({ label: r.name, value: r.id }))}
            />
          </Form.Item>
          <Form.Item name="post_ids" label={t('岗位')}>
            <Select
              mode="multiple"
              allowClear
              placeholder={t('请选择岗位')}
              optionFilterProp="label"
              options={posts.map((p) => ({ label: p.name, value: p.id, disabled: p.status !== 1 }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('重置密码 · {{name}}', { name: resetRecord?.username ?? '' })}
        open={!!resetRecord}
        onCancel={() => setResetRecord(null)}
        onOk={submitReset}
        confirmLoading={resetting}
        okText={t('重置')}
        destroyOnClose
      >
        <Form form={resetForm} layout="vertical" preserve={false}>
          <Form.Item
            name="password"
            label={t('新密码')}
            rules={[{ required: true, message: t('请输入新密码') }, { min: 6, message: t('至少 6 位') }]}
          >
            <Input.Password autoComplete="new-password" placeholder={t('须满足系统密码强度策略')} />
          </Form.Item>
          <Form.Item
            name="confirm"
            label={t('确认新密码')}
            dependencies={['password']}
            rules={[
              { required: true, message: t('请再次输入新密码') },
              ({ getFieldValue }) => ({
                validator: (_, value) =>
                  !value || value === getFieldValue('password')
                    ? Promise.resolve()
                    : Promise.reject(new Error(t('两次输入的密码不一致'))),
              }),
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="must_change"
            label={t('要求该用户下次登录后修改密码')}
            initialValue={true}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <div className="form-hint">{t('重置后该用户的所有登录会话会立即失效，需用新密码重新登录。')}</div>
        </Form>
      </Modal>

      <ExcelImportModal
        open={importOpen}
        title={t('批量导入用户')}
        hint={t('请使用「下载导入模板」生成的 .xlsx 文件；密码留空用默认初始密码，部门须为已存在的部门名称')}
        onClose={() => setImportOpen(false)}
        onDone={() => fetchList(params)}
        downloadTemplate={UserAPI.downloadUserImportTemplate}
        doImport={UserAPI.importUsers}
      />

      {inviteLink ? (
        <Modal
          title={t('邀请链接已生成')}
          open
          onCancel={() => setInviteLink(null)}
          footer={<Button type="primary" onClick={() => setInviteLink(null)}>{t('我已复制，关闭')}</Button>}
          maskClosable={false}
        >
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message={t('此链接只显示这一次，请复制后发送给受邀人；对方通过链接注册后将自动加入当前租户。')}
          />
          <Input.TextArea
            rows={2}
            readOnly
            value={inviteLink}
            onFocus={(e) => e.target.select()}
            className="cell-mono"
            style={{ fontSize: 12 }}
          />
        </Modal>
      ) : (
        <Modal
          title={t('邀请用户')}
          open={inviteOpen}
          onCancel={() => setInviteOpen(false)}
          onOk={() => void onInvite()}
          okText={t('生成邀请链接')}
          confirmLoading={inviting}
          destroyOnHidden
        >
          <Form form={inviteForm} layout="vertical">
            <Form.Item
              name="role_id"
              label={t('分配角色（可选）')}
              tooltip={t('受邀人注册后将自动获得该角色；不选则无角色')}
            >
              <Select
                allowClear
                placeholder={t('选择角色')}
                options={roles.map((r) => ({ label: r.name, value: r.id }))}
                prefix={<TeamOutlined />}
              />
            </Form.Item>
            <Form.Item name="email" label={t('受邀人邮箱（可选）')}>
              <Input placeholder={t('仅登记，不校验')} allowClear />
            </Form.Item>
          </Form>
        </Modal>
      )}
    </div>
  )
}
