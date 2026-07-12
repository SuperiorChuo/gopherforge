import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  message, Card, Checkbox,
} from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemRole, Permission } from '@/types'
import * as RoleAPI from '@/api/system/role'
import { getPermissionList } from '@/api/system/permission'

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

export default function RolePage() {
  const [list, setList] = useState<SystemRole[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SystemRole | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [permModalOpen, setPermModalOpen] = useState(false)
  const [permRole, setPermRole] = useState<SystemRole | null>(null)
  const [allPerms, setAllPerms] = useState<Permission[]>([])
  const [selectedPerms, setSelectedPerms] = useState<number[]>([])
  const [permSubmitting, setPermSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await RoleAPI.getRoleList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取角色列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
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

  const openEdit = (record: SystemRole) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      description: record.description,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await RoleAPI.deleteRole(id)
      message.success('删除成功')
      fetchList(params)
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      if (editRecord) {
        await RoleAPI.updateRole(editRecord.id, values)
        message.success('更新成功')
      } else {
        await RoleAPI.createRole(values)
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

  const openPermModal = async (record: SystemRole) => {
    setPermRole(record)
    try {
      const [permsRes, assignedIds] = await Promise.all([
        getPermissionList({ page: 1, page_size: 500 }),
        RoleAPI.getRolePermissions(record.id),
      ])
      setAllPerms(permsRes.list)
      setSelectedPerms(assignedIds)
    } catch {
      message.error('加载权限失败')
    }
    setPermModalOpen(true)
  }

  const handleAssignPerms = async () => {
    if (!permRole) return
    setPermSubmitting(true)
    try {
      await RoleAPI.assignRolePermissions(permRole.id, selectedPerms)
      message.success('权限分配成功')
      setPermModalOpen(false)
    } catch {
      message.error('权限分配失败')
    } finally {
      setPermSubmitting(false)
    }
  }

  const columns: ColumnsType<SystemRole> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code' },
    { title: '描述', dataIndex: 'description' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={v === 1 ? 'success' : 'default'}>{v === 1 ? '启用' : '禁用'}</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 200,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          <Popconfirm title="确认删除该角色?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
          <Button type="link" size="small" onClick={() => openPermModal(record)}>分配权限</Button>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch}>
          <Form.Item name="keyword">
            <Input placeholder="名称/编码" prefix={<SearchOutlined />} allowClear />
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
        <div style={{ marginBottom: 16 }}>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增角色</Button>
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

      <Modal
        title={editRecord ? '编辑角色' : '新增角色'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="编码" rules={[{ required: true, message: '请输入编码' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`分配权限 - ${permRole?.name}`}
        open={permModalOpen}
        onOk={handleAssignPerms}
        onCancel={() => setPermModalOpen(false)}
        confirmLoading={permSubmitting}
        width={640}
      >
        <Checkbox.Group
          value={selectedPerms}
          onChange={(vals) => setSelectedPerms(vals as number[])}
          style={{ display: 'flex', flexWrap: 'wrap', gap: 8, padding: '16px 0' }}
        >
          {allPerms.map((p) => (
            <Checkbox key={p.id} value={p.id} style={{ marginInlineStart: 0 }}>
              {p.name} ({p.code})
            </Checkbox>
          ))}
        </Checkbox.Group>
      </Modal>
    </div>
  )
}
