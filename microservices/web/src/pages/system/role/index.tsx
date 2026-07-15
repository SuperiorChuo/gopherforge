import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input,
  Card, Checkbox,
} from 'antd'
import { message } from '@/utils/feedback'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemRole, Permission } from '@/types'
import * as RoleAPI from '@/api/system/role'
import { getPermissionList } from '@/api/system/permission'
import TableToolbar from '@/components/TableToolbar'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

export default function RolePage() {
  const [list, setList] = useState<SystemRole[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SystemRole | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [permModalOpen, setPermModalOpen] = useState(false)
  const [permRole, setPermRole] = useState<SystemRole | null>(null)
  const [allPerms, setAllPerms] = useState<Permission[]>([])
  const [selectedPerms, setSelectedPerms] = useState<number[]>([])
  const [permSubmitting, setPermSubmitting] = useState(false)
  const [permFilter, setPermFilter] = useState('')
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

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

  const handleSearch = (values: { keyword?: string }) => {
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
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await RoleAPI.deleteRole(id)
      message.success('删除成功')
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
    setPermFilter('')
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

  const filteredPerms = permFilter
    ? allPerms.filter((p) => p.name.includes(permFilter) || p.code.includes(permFilter))
    : allPerms

  const columns: ColumnsType<SystemRole> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name' },
    {
      title: '编码',
      dataIndex: 'code',
      render: (v: string) => <Tag variant="filled" className="cell-mono">{v}</Tag>,
    },
    { title: '描述', dataIndex: 'description', ellipsis: true },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 200,
      render: (_, record) => (
        <Space>
          {hasPerm('system:role:update') && (
            <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:role:delete') && (
            <Popconfirm title="确认删除该角色?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger>删除</Button>
            </Popconfirm>
          )}
          {hasPerm('system:role:update') && (
            <Button type="link" size="small" onClick={() => openPermModal(record)}>分配权限</Button>
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
            <Input placeholder="名称/编码" prefix={<SearchOutlined />} allowClear />
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
          title="角色列表"
          total={total}
          extra={
            <>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:role:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增角色</Button>
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
        title={editRecord ? '编辑角色' : '新增角色'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="code"
            label="编码"
            rules={[{ required: true, message: '请输入编码' }]}
            tooltip={editRecord ? '编码创建后不可修改' : undefined}
          >
            <Input disabled={!!editRecord} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
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
        <div className="perm-assign-bar">
          <Input
            placeholder="搜索权限名称/编码"
            prefix={<SearchOutlined />}
            allowClear
            value={permFilter}
            onChange={(e) => setPermFilter(e.target.value)}
            style={{ width: 240 }}
          />
          <Space>
            <span className="perm-assign-count">
              已选 <b>{selectedPerms.length}</b> / {allPerms.length}
            </span>
            <Button
              size="small"
              onClick={() =>
                setSelectedPerms(Array.from(new Set([...selectedPerms, ...filteredPerms.map((p) => p.id)])))
              }
            >
              全选
            </Button>
            <Button size="small" onClick={() => setSelectedPerms([])}>清空</Button>
          </Space>
        </div>
        <div className="perm-assign-list">
          {filteredPerms.map((p) => {
            const checked = selectedPerms.includes(p.id)
            return (
              <div
                key={p.id}
                className={`perm-pill${checked ? ' perm-pill-on' : ''}`}
                onClick={() =>
                  setSelectedPerms(
                    checked ? selectedPerms.filter((id) => id !== p.id) : [...selectedPerms, p.id],
                  )
                }
              >
                <Checkbox checked={checked} style={{ pointerEvents: 'none' }} />
                <span className="perm-pill-name">{p.name}</span>
                <span className="cell-mono perm-pill-code">{p.code}</span>
              </div>
            )
          })}
        </div>
      </Modal>
    </div>
  )
}
