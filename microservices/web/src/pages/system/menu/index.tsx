import { useEffect, useMemo, useState } from 'react'
import {
  Table, Button, Space, Popconfirm, Modal, Form, Input, Select,
  Card, InputNumber, Switch, TreeSelect, Segmented, Row, Col,
} from 'antd'
import { message } from '@/utils/feedback'
import { PlusOutlined, SearchOutlined, ReloadOutlined, MenuOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Menu } from '@/types'
import * as MenuAPI from '@/api/system/menu'
import TableToolbar from '@/components/TableToolbar'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'
import StatusPill, { EnableStatusPill } from '@/components/StatusPill'

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

type MenuTreeNode = { title: string; value: number; children?: MenuTreeNode[] }

function toTreeSelectData(nodes: Menu[]): MenuTreeNode[] {
  return nodes.map((n) => ({
    title: n.title || n.name,
    value: n.id,
    children: n.children?.length ? toTreeSelectData(n.children) : undefined,
  }))
}

function countTree(nodes: Menu[]): number {
  return nodes.reduce((acc, n) => acc + 1 + (n.children ? countTree(n.children) : 0), 0)
}

export default function MenuPage() {
  const [list, setList] = useState<Menu[]>([])
  const [tree, setTree] = useState<Menu[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<'tree' | 'list'>('tree')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Menu | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await MenuAPI.getMenuList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取菜单列表失败')
    } finally {
      setLoading(false)
    }
  }

  const fetchTree = async () => {
    setLoading(true)
    try {
      const res = await MenuAPI.getMenuTree()
      setTree(res ?? [])
    } catch {
      message.error('获取菜单树失败')
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
    // 搜索结果是扁平匹配，切到列表视图展示
    setView('list')
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  // 编辑时禁止把自己挂为自己的子级（避免成环）
  const treeSelectData = useMemo(() => {
    const prune = (nodes: MenuTreeNode[]): MenuTreeNode[] =>
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

  const openEdit = (record: Menu) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      title: record.title,
      path: record.path,
      component: record.component,
      icon: record.icon,
      parent_id: record.parent_id === 0 ? undefined : record.parent_id,
      sort: record.sort,
      status: record.status,
      hidden: record.hidden === 1,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await MenuAPI.deleteMenu(id)
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
      // 后端 hidden 是 0/1 整型，Switch 给出布尔值；parent_id 空表示顶级
      const payload = { ...values, hidden: values.hidden ? 1 : 0, parent_id: values.parent_id ?? 0 }
      if (editRecord) {
        await MenuAPI.updateMenu(editRecord.id, payload)
        message.success('更新成功')
      } else {
        await MenuAPI.createMenu(payload)
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

  const columns: ColumnsType<Menu> = [
    {
      title: '标题',
      dataIndex: 'title',
      render: (v: string, record) => (
        <span style={{ fontWeight: 500 }}>
          <MenuOutlined className="tree-title-icon" />
          {v || record.name}
        </span>
      ),
    },
    { title: '名称', dataIndex: 'name', width: 150, render: (v: string) => <span className="cell-mono" style={{ fontSize: 12 }}>{v}</span> },
    {
      title: '路径',
      dataIndex: 'path',
      render: (v: string) => v && <span className="cell-mono cell-dim">{v}</span>,
    },
    { title: '排序', dataIndex: 'sort', width: 70 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: '隐藏',
      dataIndex: 'hidden',
      width: 70,
      render: (v: number) =>
        v === 1 ? (
          <StatusPill tone="warning" label="隐藏" pulse={false} />
        ) : (
          <StatusPill tone="muted" label="显示" />
        ),
    },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
          {hasPerm('system:menu:update') && (
            <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:menu:delete') && (
            <Popconfirm title="确认删除该菜单?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  const isTree = view === 'tree'

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch} initialValues={params}>
          <Form.Item name="keyword">
            <Input placeholder="名称/路径" prefix={<SearchOutlined />} allowClear />
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

      <Card>
        <TableToolbar
          title="菜单结构"
          total={isTree ? countTree(tree) : total}
          extra={
            <>
              <Button icon={<ReloadOutlined />} onClick={refresh}>刷新</Button>
              {hasPerm('system:menu:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增菜单</Button>
              )}
            </>
          }
        />
        <Table
          rowKey="id"
          columns={columns}
          dataSource={isTree ? tree : list}
          loading={loading}
          expandable={isTree ? { defaultExpandAllRows: true } : undefined}
          pagination={
            isTree
              ? false
              : {
                  total,
                  current: params.page,
                  pageSize: params.page_size,
                  showSizeChanger: true,
                  showTotal: (t) => `共 ${t} 条`,
                  onChange: (page, page_size) => setParams({ ...params, page, page_size }),
                }
          }
        />
      </Card>

      <Modal
        title={editRecord ? '编辑菜单' : '新增菜单'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={600}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
                <Input placeholder="菜单显示名，如：用户管理" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
                <Input placeholder="唯一标识，如：user" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="path" label="路径">
                <Input placeholder="/system/user" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="component" label="组件">
                <Input placeholder="pages/system/user" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="parent_id" label="上级菜单">
                <TreeSelect
                  treeData={treeSelectData}
                  placeholder="不选则为顶级菜单"
                  allowClear
                  showSearch
                  treeDefaultExpandAll
                  treeNodeFilterProp="title"
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="icon" label="图标">
                <Input placeholder="图标名称" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="sort" label="排序" initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="status" label="状态" initialValue={1}>
                <Select>
                  <Select.Option value={1}>启用</Select.Option>
                  <Select.Option value={0}>禁用</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="hidden" label="隐藏" valuePropName="checked" initialValue={false}>
                <Switch />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Modal>
    </div>
  )
}
