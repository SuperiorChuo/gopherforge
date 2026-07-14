import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Row, Col, Avatar,
} from 'antd'
import { message } from '@/utils/feedback'
import { PlusOutlined, SearchOutlined, ReloadOutlined, UserOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemUser, SystemRole, Department } from '@/types'
import * as UserAPI from '@/api/system/user'
import { getRoleList } from '@/api/system/role'
import { getDepartmentList } from '@/api/system/department'
import TableToolbar from '@/components/TableToolbar'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

const avatarPalette = ['#6366f1', '#0ea5e9', '#10b981', '#f59e0b', '#ec4899', '#8b5cf6']
const roleTagPalette = ['geekblue', 'cyan', 'purple', 'magenta', 'gold']

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

export default function UserPage() {
  const [list, setList] = useState<SystemUser[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SystemUser | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [roles, setRoles] = useState<SystemRole[]>([])
  const [depts, setDepts] = useState<Department[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await UserAPI.getUserList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取用户列表失败')
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

  const openEdit = (record: SystemUser) => {
    setEditRecord(record)
    form.setFieldsValue({
      username: record.username,
      nickname: record.nickname,
      email: record.email,
      phone: record.phone,
      status: record.status,
      department_id: record.department_id,
      role_ids: record.roles?.map((r) => r.id) ?? [],
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await UserAPI.deleteUser(id)
      message.success('删除成功')
      // 删除的是当前页最后一条时回退一页，避免落在空页
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const { role_ids, ...rest } = values
      if (editRecord) {
        await UserAPI.updateUser(editRecord.id, rest)
        // 后端的用户更新接口不含状态字段，状态走独立接口
        if (typeof rest.status === 'number' && rest.status !== editRecord.status) {
          await UserAPI.updateUserStatus(editRecord.id, rest.status)
        }
        if (role_ids !== undefined) {
          await UserAPI.assignUserRoles(editRecord.id, role_ids ?? [])
        }
        message.success('更新成功')
      } else {
        const created = await UserAPI.createUser(rest)
        if (role_ids?.length) {
          await UserAPI.assignUserRoles(created.id, role_ids)
        }
        message.success('创建成功')
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      message.error('操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<SystemUser> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: '用户',
      dataIndex: 'username',
      render: (_, record) => (
        <div className="user-cell">
          <Avatar
            size={36}
            icon={<UserOutlined />}
            style={{ background: avatarPalette[record.id % avatarPalette.length], flexShrink: 0 }}
          >
            {(record.nickname || record.username)?.slice(0, 1).toUpperCase()}
          </Avatar>
          <div>
            <div className="user-cell-name">{record.username}</div>
            {record.nickname && <div className="user-cell-sub">{record.nickname}</div>}
          </div>
        </div>
      ),
    },
    { title: '邮箱', dataIndex: 'email' },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <Tag color={v === 1 ? 'success' : 'default'}>{v === 1 ? '启用' : '禁用'}</Tag>,
    },
    {
      title: '角色',
      dataIndex: 'roles',
      render: (roles: SystemUser['roles']) =>
        roles?.map((r, i) => (
          <Tag key={r.id} color={roleTagPalette[i % roleTagPalette.length]} variant="filled">
            {r.code}
          </Tag>
        )),
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
          {hasPerm('system:user:update') && (
            <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:user:delete') && (
            <Popconfirm title="确认删除该用户?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch} initialValues={params}>
          <Form.Item name="keyword">
            <Input placeholder="用户名/邮箱" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
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
        <TableToolbar
          title="用户列表"
          total={total}
          extra={
            <>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:user:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增用户</Button>
              )}
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
        title={editRecord ? '编辑用户' : '新增用户'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
                <Input disabled={!!editRecord} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="nickname" label="昵称">
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name="email"
                label="邮箱"
                rules={[
                  { type: 'email', message: '邮箱格式不正确' },
                  // 后端更新接口要求邮箱必填且合法
                  ...(editRecord ? [{ required: true, message: '请输入邮箱' }] : []),
                ]}
              >
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="phone" label="手机号">
                <Input />
              </Form.Item>
            </Col>
          </Row>
          {!editRecord && (
            <Form.Item
              name="password"
              label="密码"
              rules={[
                { required: true, message: '请输入密码' },
                { min: 6, message: '密码至少 6 位' },
              ]}
            >
              <Input.Password />
            </Form.Item>
          )}
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="status" label="状态" initialValue={1}>
                <Select>
                  <Select.Option value={1}>启用</Select.Option>
                  <Select.Option value={0}>禁用</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="department_id"
                label="部门"
                tooltip={editRecord ? '后端暂不支持修改已有用户的部门' : undefined}
              >
                <Select
                  allowClear
                  showSearch
                  placeholder="请选择部门"
                  optionFilterProp="children"
                  disabled={!!editRecord}
                >
                  {depts.map((d) => (
                    <Select.Option key={d.id} value={d.id}>{d.name}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="role_ids" label="角色">
            <Select mode="multiple" placeholder="请选择角色" optionFilterProp="children">
              {roles.map((r) => (
                <Select.Option key={r.id} value={r.id}>{r.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
