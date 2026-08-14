import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Form, Grid, Input, Select,
  InputNumber, Switch, TreeSelect, Segmented, Row, Col, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, MenuOutlined,
  EditOutlined, DeleteOutlined, ArrowsAltOutlined, ShrinkOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Menu } from '@/types'
import * as MenuAPI from '@/api/system/menu'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableRowActions from '@/components/common/TableRowActions'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'
import { useConfirmAction } from '@/hooks/useConfirmAction'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import StatusPill, { EnableStatusPill } from '@/components/common/StatusPill'

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

// 有子节点的行 id（树是异步拉的，defaultExpandAllRows 在首挂空数据时算一次
// 就失效——必须受控展开）
function collectExpandableKeys(nodes: Menu[]): number[] {
  const keys: number[] = []
  for (const n of nodes) {
    if (n.children?.length) {
      keys.push(n.id, ...collectExpandableKeys(n.children))
    }
  }
  return keys
}

export default function MenuPage() {
  const [tree, setTree] = useState<Menu[]>([])
  const [view, setView] = useState<'tree' | 'list'>('tree')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [treeLoading, setTreeLoading] = useState(false)
  const editModal = useCrudModal<Menu>()
  const [submitting, setSubmitting] = useState(false)
  const [expandedKeys, setExpandedKeys] = useState<readonly React.Key[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = useCallback((p: SearchParams) => (
    view === 'list' ? MenuAPI.getMenuList(p) : Promise.resolve({ list: [], total: 0 })
  ), [view])
  const onListError = useCallback(() => message.error(t('获取菜单列表失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onListError,
  })

  const fetchTree = async () => {
    setTreeLoading(true)
    try {
      const res = await MenuAPI.getMenuTree()
      setTree(res ?? [])
      setExpandedKeys(collectExpandableKeys(res ?? []))
    } catch {
      message.error(t('获取菜单树失败'))
    } finally {
      setTreeLoading(false)
    }
  }

  useEffect(() => {
    void fetchTree()
  }, [])

  const refresh = () => {
    void fetchTree()
    if (view === 'list') void reload()
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
        .filter((n) => n.value !== editModal.record?.id)
        .map((n) => ({ ...n, children: n.children ? prune(n.children) : undefined }))
    return prune(toTreeSelectData(tree))
  }, [tree, editModal.record])

  const openCreate = () => {
    form.resetFields()
    editModal.openCreate()
  }

  const openEdit = (record: Menu) => {
    editModal.openEdit(record)
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
  }

  const confirmAction = useConfirmAction()
  const handleDelete = (id: number) => {
    void confirmAction.run({
      action: () => MenuAPI.deleteMenu(id),
      onSuccess: () => {
        message.success(t('删除成功'))
        refresh()
      },
      onError: () => message.error(t('删除失败')),
    })
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      // 后端 hidden 是 0/1 整型，Switch 给出布尔值；parent_id 空表示顶级
      const payload = { ...values, hidden: values.hidden ? 1 : 0, parent_id: values.parent_id ?? 0 }
      if (editModal.record) {
        await MenuAPI.updateMenu(editModal.record.id, payload)
        message.success(t('更新成功'))
      } else {
        await MenuAPI.createMenu(payload)
        message.success(t('创建成功'))
      }
      editModal.close()
      refresh()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<Menu> = [
    {
      title: t('标题'),
      dataIndex: 'title',
      width: 300,
      ellipsis: true,
      render: (v: string, record) => (
        <span className="menu-title-cell">
          <MenuOutlined className="tree-title-icon" />
          <span className="menu-title-text">{v || record.name}</span>
        </span>
      ),
    },
    { title: t('名称'), dataIndex: 'name', width: 170, ellipsis: true, responsive: ['sm'], render: (v: string) => <span className="cell-mono menu-name-cell">{v}</span> },
    {
      title: t('路径'),
      dataIndex: 'path',
      width: 260,
      ellipsis: true,
      responsive: ['md'],
      render: (v: string) => v ? <span className="cell-mono cell-dim menu-path-cell">{v}</span> : <span className="cell-muted">—</span>,
    },
    { title: t('排序'), dataIndex: 'sort', width: 70 },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: t('隐藏'),
      dataIndex: 'hidden',
      width: 70,
      responsive: ['sm'],
      render: (v: number) =>
        v === 1 ? (
          <StatusPill tone="warning" label="隐藏" pulse={false} />
        ) : (
          <StatusPill tone="muted" label="显示" />
        ),
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 96,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.title || record.name })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:menu:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:menu:delete'),
              confirm: {
                title: t('确认删除该菜单?'),
                description: t('存在子菜单时将无法删除'),
              },
              onClick: () => handleDelete(record.id),
            },
          ]}
        />
      ),
    },
  ]

  const isTree = view === 'tree'

  return (
    <>
      <ListPageShell
        className="menu-page"
        filter={(
          <ListFilterForm
            form={searchForm}
            onFinish={handleSearch}
            initialValues={params}
          >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索名称 / 路径')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
              <Select.Option value={1}>{t('启用')}</Select.Option>
              <Select.Option value={0}>{t('禁用')}</Select.Option>
            </Select>
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
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
          title="菜单结构"
          total={isTree ? countTree(tree) : total}
          extra={
            <Space wrap>
              {isTree && (
                <>
                  <Tooltip title={t('全部展开')}>
                    <Button aria-label={t('全部展开菜单')} icon={<ArrowsAltOutlined />} onClick={() => setExpandedKeys(collectExpandableKeys(tree))} />
                  </Tooltip>
                  <Tooltip title={t('全部收起')}>
                    <Button aria-label={t('全部收起菜单')} icon={<ShrinkOutlined />} onClick={() => setExpandedKeys([])} />
                  </Tooltip>
                </>
              )}
              <Button icon={<ReloadOutlined />} onClick={refresh}>{t('刷新')}</Button>              {hasPerm('system:menu:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增菜单')}</Button>
              )}
            </Space>
          }
          />
        )}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={isTree ? tree : list}
          loading={view === 'tree' ? treeLoading : loading}
          rowClassName={(record) => (record.status === 0 ? 'menu-row-disabled' : '')}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无菜单" compact /> }}
          expandable={isTree ? { expandedRowKeys: expandedKeys, onExpandedRowsChange: setExpandedKeys } : undefined}
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
      </ListPageShell>

      <EntityFormModal
        title={editModal.record ? t('编辑菜单') : t('新增菜单')}
        open={editModal.open}
        form={form}
        onClose={editModal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        width={600}
        formProps={{ style: { marginTop: 16 } }}
      >
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('请输入标题') }]}>
                <Input placeholder={t('菜单显示名，如：用户管理')} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
                <Input placeholder={t('唯一标识，如：user')} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="path" label={t('路径')}>
                <Input placeholder="/system/user" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="component" label={t('组件')}>
                <Input placeholder="pages/system/user" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="parent_id" label={t('上级菜单')}>
                <TreeSelect
                  treeData={treeSelectData}
                  placeholder={t('不选则为顶级菜单')}
                  allowClear
                  showSearch
                  treeDefaultExpandAll
                  treeNodeFilterProp="title"
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="icon" label={t('图标')}>
                <Input placeholder={t('图标名称')} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={8}>
              <Form.Item name="sort" label={t('排序')} initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={8}>
              <Form.Item name="status" label={t('状态')} initialValue={1}>
                <Select>
                  <Select.Option value={1}>{t('启用')}</Select.Option>
                  <Select.Option value={0}>{t('禁用')}</Select.Option>
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} sm={8}>
              <Form.Item name="hidden" label={t('隐藏')} valuePropName="checked" initialValue={false}>
                <Switch />
              </Form.Item>
            </Col>
          </Row>
      </EntityFormModal>
    </>
  )
}
