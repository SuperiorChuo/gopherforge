import { useEffect, useMemo, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, InputNumber, Row, Col, TreeSelect, Segmented,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, ApartmentOutlined,
  EditOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Department } from '@/types'
import * as DeptAPI from '@/api/system/department'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'
import { displayUserName, useUserNameMap } from '@/hooks/useUserNameMap'

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

type DeptTreeNode = { title: string; value: number; children?: DeptTreeNode[] }

function toTreeSelectData(nodes: Department[]): DeptTreeNode[] {
  return nodes.map((n) => ({
    title: n.name,
    value: n.id,
    children: n.children?.length ? toTreeSelectData(n.children) : undefined,
  }))
}

function countTree(nodes: Department[]): number {
  return nodes.reduce((acc, n) => acc + 1 + (n.children ? countTree(n.children) : 0), 0)
}

export default function DepartmentPage() {
  const [list, setList] = useState<Department[]>([])
  const [tree, setTree] = useState<Department[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<'tree' | 'list'>('tree')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Department | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()
  // 部门主管选人与列表主管姓名展示共用一份用户映射（模块级缓存，403 静默降级）
  const userMap = useUserNameMap()
  const userOptions = useMemo(
    () => Object.entries(userMap).map(([id, name]) => ({ value: Number(id), label: name })),
    [userMap],
  )

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await DeptAPI.getDepartmentList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取部门列表失败')
    } finally {
      setLoading(false)
    }
  }

  const fetchTree = async () => {
    setLoading(true)
    try {
      const res = await DeptAPI.getDepartmentTree()
      setTree(res ?? [])
    } catch {
      message.error('获取部门树失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (view === 'list') fetchList(params)
  }, [params, view])

  useEffect(() => {
    fetchTree()
  }, [])

  const refresh = () => {
    fetchTree()
    if (view === 'list') fetchList(params)
  }

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setView('list')
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  // 编辑时禁止把自己选为上级（避免成环）
  const treeSelectData = useMemo(() => {
    const prune = (nodes: DeptTreeNode[]): DeptTreeNode[] =>
      nodes
        .filter((n) => n.value !== editRecord?.id)
        .map((n) => ({ ...n, children: n.children ? prune(n.children) : undefined }))
    return prune(toTreeSelectData(tree))
  }, [tree, editRecord])

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: Department) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      parent_id: record.parent_id === 0 ? undefined : record.parent_id,
      leader: record.leader,
      leader_user_id: record.leader_user_id || undefined,
      phone: record.phone,
      email: record.email,
      sort: record.sort,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await DeptAPI.deleteDepartment(id)
      message.success('删除成功')
      refresh()
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      // leader_user_id 可清空：清空时显式传 0（identity 侧按 0 置空）
      const payload = { ...values, parent_id: values.parent_id ?? 0, leader_user_id: values.leader_user_id ?? 0 }
      if (editRecord) {
        await DeptAPI.updateDepartment(editRecord.id, payload)
        message.success('更新成功')
      } else {
        await DeptAPI.createDepartment(payload)
        message.success('创建成功')
      }
      setModalOpen(false)
      refresh()
    } catch {
      message.error('操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<Department> = [
    {
      title: '名称',
      dataIndex: 'name',
      render: (v: string) => (
        <span style={{ fontWeight: 500 }}>
          <ApartmentOutlined className="tree-title-icon" />
          {v}
        </span>
      ),
    },
    {
      title: '编码',
      dataIndex: 'code',
      width: 180,
      render: (v: string) => <Tag variant="filled" className="cell-mono">{v}</Tag>,
    },
    {
      title: '部门主管',
      dataIndex: 'leader_user_id',
      width: 130,
      // 优先展示主管选人（leader_user_id → 姓名），未设置时回退旧的负责人文本字段
      render: (v: number | undefined, record) =>
        v ? displayUserName(userMap, v) : record.leader || <span className="cell-muted">—</span>,
    },
    { title: '排序', dataIndex: 'sort', width: 70 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:department:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:department:delete') && (
            <Popconfirm title="确认删除该部门?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  const isTree = view === 'tree'

  return (
    <div className="page-list department-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索名称 / 编码" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
          <Form.Item style={{ marginInlineEnd: 0, marginLeft: 'auto' }}>
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'tree' | 'list')}
              options={[
                { label: '树形', value: 'tree' },
                { label: '列表', value: 'list' },
              ]}
            />
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="部门架构"
          total={isTree ? countTree(tree) : total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={refresh}>刷新</Button>
              {hasPerm('system:department:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增部门</Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={isTree ? tree : list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无部门" compact /> }}
          expandable={isTree ? { defaultExpandAllRows: true } : undefined}
          pagination={
            isTree
              ? false
              : {
                  total,
                  current: params.page,
                  pageSize: params.page_size,
                  showSizeChanger: true,
                  showQuickJumper: true,
                  showTotal: (t) => `共 ${t} 条`,
                  onChange: (page, page_size) => setParams({ ...params, page, page_size }),
                }
          }
        />
      </Card>

      <Modal
        title={editRecord ? '编辑部门' : '新增部门'}
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
              <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="code" label="编码" rules={[{ required: true, message: '请输入编码' }]}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="parent_id" label="上级部门">
                <TreeSelect
                  treeData={treeSelectData}
                  placeholder="不选则为顶级部门"
                  allowClear
                  showSearch
                  treeDefaultExpandAll
                  treeNodeFilterProp="title"
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="sort" label="排序" initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name="leader_user_id"
                label="部门主管"
                tooltip="审批流「部门主管」规则据此取主管；可清空"
              >
                <Select
                  showSearch
                  allowClear
                  optionFilterProp="label"
                  placeholder="选择主管用户（可清空）"
                  options={userOptions}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="leader" label="负责人（备注名）">
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="phone" label="电话">
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="email" label="邮箱" rules={[{ type: 'email', message: '邮箱格式不正确' }]}>
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="status" label="状态" initialValue={1}>
                <Select>
                  <Select.Option value={1}>启用</Select.Option>
                  <Select.Option value={0}>禁用</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </div>
  )
}
