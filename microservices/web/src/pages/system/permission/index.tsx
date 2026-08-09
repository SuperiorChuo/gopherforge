import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Row, Col, Segmented, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined,
  EditOutlined, DeleteOutlined, ArrowsAltOutlined, ShrinkOutlined,
  SafetyOutlined, MenuOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Permission, MenuItem } from '@/types'
import * as PermAPI from '@/api/system/permission'
import * as MenuAPI from '@/api/system/menu'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

const typeLabels: Record<number, string> = { 1: '菜单', 2: '按钮', 3: 'API' }
const typeColors: Record<number, string> = { 1: 'blue', 2: 'green', 3: 'orange' }
const methodColors: Record<string, string> = { GET: 'blue', POST: 'green', PUT: 'orange', DELETE: 'red' }

interface PermRow {
  key: string
  id: number
  name: string
  code: string
  type: number
  path?: string
  method?: string
  description?: string
  created_at?: string
  isMenu: boolean
  children?: PermRow[]
}

function buildPermTree(menus: MenuItem[], allPerms: Permission[]): PermRow[] {
  const permByCode = new Map<string, Permission[]>()
  // 递归展平：后端 /permissions/tree 返回带 children 的嵌套树，只遍历顶层会丢子权限
  // （按钮/API 等叶子权限），导致菜单树下关联权限显示不全。
  const flatten = (list: Permission[]) => {
    for (const p of list) {
      const arr = permByCode.get(p.code) || []
      arr.push(p)
      permByCode.set(p.code, arr)
      if (p.children?.length) flatten(p.children)
    }
  }
  flatten(allPerms)
  function walk(ms: MenuItem[]): PermRow[] {
    return ms.map(m => {
      const menuPermCode = m.permission || ''
      const perms = permByCode.get(menuPermCode) || []
      return {
        key: `menu-${m.id}`,
        id: m.id,
        name: m.title || m.name,
        code: m.path || '',
        type: 1,
        path: m.path,
        description: menuPermCode,
        isMenu: true,
        children: [
          ...perms.map(p => ({
            key: `perm-${p.id}`,
            id: p.id,
            name: p.name,
            code: p.code,
            type: p.type,
            path: p.path,
            method: p.method,
            description: p.description,
            created_at: p.created_at,
            isMenu: false,
          })),
          ...(m.children?.length ? walk(m.children) : []),
        ].flat(),
      }
    })
  }
  return walk(menus)
}

function collectExpandableKeys(nodes: PermRow[]): string[] {
  const keys: string[] = []
  for (const n of nodes) {
    if (n.children?.length) {
      keys.push(n.key, ...collectExpandableKeys(n.children))
    }
  }
  return keys
}

export default function PermissionPage() {
  const [list, setList] = useState<Permission[]>([])
  const [tree, setTree] = useState<PermRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<'tree' | 'list'>('tree')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Permission | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [expandedKeys, setExpandedKeys] = useState<readonly React.Key[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await PermAPI.getPermissionList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取权限列表失败'))
    } finally {
      setLoading(false)
    }
  }

  const fetchTree = async () => {
    setLoading(true)
    try {
      const [menuTree, permTree] = await Promise.all([
        MenuAPI.getMenuTree(),
        PermAPI.getPermissionTree(),
      ])
      const combined = buildPermTree(menuTree ?? [], permTree ?? [])
      setTree(combined)
      setExpandedKeys(collectExpandableKeys(combined))
    } catch {
      message.error(t('获取权限树失败'))
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

  const handleSearch = (values: { keyword?: string }) => {
    setView('list')
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

  const openEdit = (record: Permission) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      type: record.type,
      path: record.path ?? undefined,
      method: record.method ?? undefined,
      parent_id: record.parent_id === 0 ? undefined : record.parent_id,
      description: record.description,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await PermAPI.deletePermission(id)
      message.success(t('删除成功'))
      refresh()
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const payload = { ...values, parent_id: values.parent_id ?? 0 }
      if (editRecord) {
        await PermAPI.updatePermission(editRecord.id, payload)
        message.success(t('更新成功'))
      } else {
        await PermAPI.createPermission(payload)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      refresh()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<PermRow> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (v: string, record: PermRow) => (
        <span className="menu-title-cell">
          {record.isMenu ? <MenuOutlined className="tree-title-icon" /> : <SafetyOutlined className="tree-title-icon" />}
          <span className="menu-title-text" style={{ paddingLeft: record.isMenu ? 0 : 8 }}>{v}</span>
        </span>
      ),
    },
    {
      title: t('编码'),
      dataIndex: 'code',
      width: 200,
      responsive: ['sm'],
      render: (v: string, record: PermRow) =>
        record.isMenu ? <span className="cell-dim cell-mono">{v}</span> : <Tag variant="filled" className="cell-mono list-code-tag">{v}</Tag>,
    },
    {
      title: t('路径'),
      dataIndex: 'path',
      width: 200,
      ellipsis: true,
      responsive: ['md'],
      render: (v: string) => v ? <code className="cell-dim" style={{ fontSize: 11 }}>{v}</code> : <span className="cell-dim">--</span>,
    },
    {
      title: 'Method',
      dataIndex: 'method',
      width: 75,
      render: (v: string) => v ? <Tag variant="filled" color={methodColors[v]}>{v}</Tag> : <span className="cell-dim">--</span>,
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 70,
      render: (v: number, record: PermRow) =>
        record.isMenu ? <Tag variant="filled" color="blue">菜单</Tag> : <Tag variant="filled" color={typeColors[v]}>{typeLabels[v] ?? v}</Tag>,
    },
    {
      title: t('操作'),
      width: 96,
      fixed: 'right',
      render: (_: unknown, record: PermRow) =>
        record.isMenu ? null : (
          <Space size={4} className="table-actions compact-table-actions">
            {hasPerm('system:permission:update') && (
              <Tooltip title={t('编辑')}>
                <Button type="text" size="small" aria-label={t('编辑权限')} icon={<EditOutlined />} onClick={() => openEdit(record as unknown as Permission)} />
              </Tooltip>
            )}
            {hasPerm('system:permission:delete') && (
              <Popconfirm title={t('确认删除该权限?')} onConfirm={() => handleDelete(record.id)}>
                <Tooltip title={t('删除')}>
                  <Button type="text" size="small" danger aria-label={t('删除权限')} icon={<DeleteOutlined />} />
                </Tooltip>
              </Popconfirm>
            )}
          </Space>
        ),
    },
  ]

  const isTree = view === 'tree'

  return (
    <div className="page-list permission-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索名称 / 编码')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
          <Form.Item className="menu-view-switch">
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'tree' | 'list')}
              options={[
                { label: t('树形'), value: 'tree' },
                { label: t('列表'), value: 'list' },
              ]}
            />
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title={t('权限结构')}
          total={isTree ? tree.reduce((acc, n) => acc + 1 + (n.children?.length ?? 0), 0) : total}
          extra={
            <Space wrap>
              {isTree && (
                <>
                  <Tooltip title={t('全部展开')}>
                    <Button aria-label={t('全部展开')} icon={<ArrowsAltOutlined />} onClick={() => setExpandedKeys(collectExpandableKeys(tree))} />
                  </Tooltip>
                  <Tooltip title={t('全部收起')}>
                    <Button aria-label={t('全部收起')} icon={<ShrinkOutlined />} onClick={() => setExpandedKeys([])} />
                  </Tooltip>
                </>
              )}
              <Button icon={<ReloadOutlined />} onClick={refresh}>{t('刷新')}</Button>
              {hasPerm('system:permission:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增权限')}</Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="key"
          className="list-table"
          columns={columns}
          dataSource={isTree ? tree : list.map(p => ({ ...p, key: `perm-${p.id}`, isMenu: false } as PermRow))}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无权限" compact /> }}
          expandable={isTree ? { expandedRowKeys: expandedKeys, onExpandedRowsChange: setExpandedKeys, rowExpandable: (r: PermRow) => (r.children?.length ?? 0) > 0 } : undefined}
          pagination={
            isTree
              ? false
              : {
                  total,
                  current: params.page,
                  pageSize: params.page_size,
                  showSizeChanger: true,
                  showQuickJumper: true,
                  showTotal: (n) => t('共 {{n}} 条', { n }),
                  onChange: (page, page_size) => setParams({ ...params, page, page_size }),
                }
          }
        />
      </Card>

      <Modal
        title={editRecord ? t('编辑权限') : t('新增权限')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="code" label={t('编码')} rules={[{ required: true, message: t('请输入编码') }]}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="type" label={t('类型')} initialValue={1}>
                <Select>
                  <Select.Option value={1}>{t('菜单')}</Select.Option>
                  <Select.Option value={2}>{t('按钮')}</Select.Option>
                  <Select.Option value={3}>API</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="method" label="Method">
                <Select allowClear placeholder={t('API 权限选填')}>
                  <Select.Option value="GET">GET</Select.Option>
                  <Select.Option value="POST">POST</Select.Option>
                  <Select.Option value="PUT">PUT</Select.Option>
                  <Select.Option value="DELETE">DELETE</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="path" label={t('路径')}>
            <Input placeholder={t('API 路径或路由 path')} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
