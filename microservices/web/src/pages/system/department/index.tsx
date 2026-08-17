import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Grid, Space, Tag, Form, Input, Select,
  InputNumber, Row, Col, TreeSelect, Segmented,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, ApartmentOutlined,
  EditOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Department } from '@/types'
import * as DeptAPI from '@/api/system/department'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableRowActions from '@/components/common/TableRowActions'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/common/StatusPill'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useTreeQuery } from '@/hooks/useTreeQuery'
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

// 有子节点的行 id（树是异步拉的，defaultExpandAllRows 在首挂空数据时算一次
// 就失效——必须受控展开）
function collectExpandableKeys(nodes: Department[]): number[] {
  const keys: number[] = []
  for (const n of nodes) {
    if (n.children?.length) {
      keys.push(n.id, ...collectExpandableKeys(n.children))
    }
  }
  return keys
}

export default function DepartmentPage() {
  const [view, setView] = useState<'tree' | 'list'>('tree')
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Department | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [expandedKeys, setExpandedKeys] = useState<readonly React.Key[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  // 部门主管选人与列表主管姓名展示共用一份用户映射（模块级缓存，403 静默降级）
  const userMap = useUserNameMap()
  const userOptions = useMemo(
    () => Object.entries(userMap).map(([id, name]) => ({ value: Number(id), label: name })),
    [userMap],
  )

  const fetchList = useCallback((p: SearchParams) => DeptAPI.getDepartmentList(p), [])
  const onListError = useCallback(() => message.error(t('获取部门列表失败')), [t])
  const { list, total, loading: listLoading, reload: reloadList } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onListError,
    enabled: view === 'list',
  })

  const fetchTree = useCallback(async () => {
    const res = await DeptAPI.getDepartmentTree()
    const next = res ?? []
    setExpandedKeys(collectExpandableKeys(next))
    return next
  }, [])
  const onTreeError = useCallback(() => message.error(t('获取部门树失败')), [t])
  const { tree, loading: treeLoading, reload: reloadTree } = useTreeQuery({
    fetcher: fetchTree,
    onError: onTreeError,
  })
  const loading = view === 'list' ? listLoading : treeLoading

  const refresh = () => {
    void reloadTree()
    if (view === 'list') void reloadList()
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
      // leader_user_id 可清空：清空时显式传 0（identity 侧按 0 置空）
      const payload = { ...values, parent_id: values.parent_id ?? 0, leader_user_id: values.leader_user_id ?? 0 }
      if (editRecord) {
        await DeptAPI.updateDepartment(editRecord.id, payload)
        message.success(t('更新成功'))
      } else {
        await DeptAPI.createDepartment(payload)
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

  const columns: ColumnsType<Department> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 300,
      ellipsis: true,
      render: (v: string) => (
        <span className="department-name-cell">
          <ApartmentOutlined className="tree-title-icon" />
          <span className="department-name-text">{v}</span>
        </span>
      ),
    },
    {
      title: t('编码'),
      dataIndex: 'code',
      width: 200,
      render: (v: string) => <Tag variant="filled" className="cell-mono department-code-tag">{v}</Tag>,
    },
    {
      title: t('部门主管'),
      dataIndex: 'leader_user_id',
      width: 130,
      // 优先展示主管选人（leader_user_id → 姓名），未设置时回退旧的负责人文本字段
      render: (v: number | undefined, record) =>
        v ? displayUserName(userMap, v) : record.leader || <span className="cell-muted">—</span>,
    },
    { title: t('排序'), dataIndex: 'sort', width: 70 },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      width: compactActions ? 48 : 96,
      align: 'center',
      fixed: 'right',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          className="department-row-actions"
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:department:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:department:delete'),
              confirm: t('确认删除该部门?'),
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
        className="department-page"
        filter={(
          <ListFilterForm
            form={searchForm}
            onFinish={handleSearch}
            initialValues={params}
          >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索名称 / 编码')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
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
            <Form.Item style={{ marginInlineEnd: 0, marginLeft: 'auto' }}>
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
            title="部门架构"
            total={isTree ? countTree(tree) : total}
            extra={(
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={refresh}>{t('刷新')}</Button>
                {hasPerm('system:department:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增部门')}</Button>
                )}
              </Space>
            )}
          />
        )}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={isTree ? tree : list}
          loading={loading}
          rowClassName={(record) => (record.status === 0 ? 'department-row-disabled' : '')}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无部门" compact /> }}
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
        title={editRecord ? t('编辑部门') : t('新增部门')}
        open={modalOpen}
        form={form}
        onClose={() => setModalOpen(false)}
        onSubmit={handleSubmit}
        submitting={submitting}
        width={560}
        formProps={{ style: { marginTop: 16 } }}
      >
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="code" label={t('编码')} rules={[{ required: true, message: t('请输入编码') }]}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="parent_id" label={t('上级部门')}>
                <TreeSelect
                  treeData={treeSelectData}
                  placeholder={t('不选则为顶级部门')}
                  allowClear
                  showSearch
                  treeDefaultExpandAll
                  treeNodeFilterProp="title"
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="sort" label={t('排序')} initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item
                name="leader_user_id"
                label={t('部门主管')}
                tooltip={t('审批流「部门主管」规则据此取主管；可清空')}
              >
                <Select
                  showSearch
                  allowClear
                  optionFilterProp="label"
                  placeholder={t('选择主管用户（可清空）')}
                  options={userOptions}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="leader" label={t('负责人（备注名）')}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="phone" label={t('电话')}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="email" label={t('邮箱')} rules={[{ type: 'email', message: t('邮箱格式不正确') }]}>
                <Input />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="status" label={t('状态')} initialValue={1}>
                <Select>
                  <Select.Option value={1}>{t('启用')}</Select.Option>
                  <Select.Option value={0}>{t('禁用')}</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
      </EntityFormModal>
    </>
  )
}
