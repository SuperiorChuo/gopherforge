import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Modal, Form, Input,
  Checkbox, Grid,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined,
  EditOutlined, DeleteOutlined, SafetyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { SystemRole, Permission } from '@/types'
import * as RoleAPI from '@/api/system/role'
import { getPermissionList } from '@/api/system/permission'

// 权限全集是准静态数据（只随迁移/代码生成变化），模块级缓存一次拉取结果，
// 反复开授权弹窗不再重拉 500 条；失败不缓存，下次重试（同 useUserNameMap 范式）。
let permListCache: Promise<Permission[]> | null = null
function fetchAllPermissions(): Promise<Permission[]> {
  if (!permListCache) {
    permListCache = getPermissionList({ page: 1, page_size: 500 })
      .then((res) => res.list)
      .catch((err) => {
        permListCache = null
        throw err
      })
  }
  return permListCache
}
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
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
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await RoleAPI.getRoleList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取角色列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
      if (editRecord) {
        await RoleAPI.updateRole(editRecord.id, values)
        message.success(t('更新成功'))
      } else {
        await RoleAPI.createRole(values)
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

  const openPermModal = async (record: SystemRole) => {
    setPermRole(record)
    setPermFilter('')
    try {
      const [perms, assignedIds] = await Promise.all([
        fetchAllPermissions(),
        RoleAPI.getRolePermissions(record.id),
      ])
      setAllPerms(perms)
      setSelectedPerms(assignedIds)
    } catch {
      message.error(t('加载权限失败'))
      return
    }
    setPermModalOpen(true)
  }

  const handleAssignPerms = async () => {
    if (!permRole) return
    setPermSubmitting(true)
    try {
      await RoleAPI.assignRolePermissions(permRole.id, selectedPerms)
      message.success(t('权限分配成功'))
      setPermModalOpen(false)
    } catch {
      message.error(t('权限分配失败'))
    } finally {
      setPermSubmitting(false)
    }
  }

  const filteredPerms = permFilter
    ? allPerms.filter((p) => p.name.includes(permFilter) || p.code.includes(permFilter))
    : allPerms

  const columns: ColumnsType<SystemRole> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 180,
      ellipsis: true,
      render: (value: string) => <span className="role-cell-name">{value}</span>,
    },
    {
      title: t('编码'),
      dataIndex: 'code',
      width: 220,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono role-code-tag">{v}</Tag>,
    },
    { title: t('描述'), dataIndex: 'description', width: 280, ellipsis: true, responsive: ['md'] },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: compactActions ? 48 : 132,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, record) => (
        <TableRowActions
          className="role-row-actions"
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:role:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'perm',
              label: t('分配权限'),
              icon: <SafetyOutlined />,
              show: hasPerm('system:role:update'),
              onClick: () => openPermModal(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:role:delete'),
              confirm: t('确认删除该角色?'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <ListPageShell
      className="role-page"
      filter={(
        <ListFilterForm
          form={searchForm}
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
        </ListFilterForm>
      )}
      toolbar={(
        <TableToolbar
          title="角色列表"
          total={total}
          extra={(
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>{t('刷新')}</Button>
              {hasPerm('system:role:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增角色')}</Button>
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
          dataSource={list}
          loading={loading}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无角色" compact /> }}
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
      </ListPageShell>

      <Modal
        title={editRecord ? t('编辑角色') : t('新增角色')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="code"
            label={t('编码')}
            rules={[{ required: true, message: t('请输入编码') }]}
            tooltip={editRecord ? t('编码创建后不可修改') : undefined}
          >
            <Input disabled={!!editRecord} />
          </Form.Item>
          <Form.Item name="description" label={t('描述')}>
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('分配权限 - {{name}}', { name: permRole?.name })}
        open={permModalOpen}
        onOk={handleAssignPerms}
        onCancel={() => setPermModalOpen(false)}
        confirmLoading={permSubmitting}
        width={640}
      >
        <div className="perm-assign-bar">
          <Input
            placeholder={t('搜索权限名称/编码')}
            prefix={<SearchOutlined />}
            allowClear
            value={permFilter}
            onChange={(e) => setPermFilter(e.target.value)}
            style={{ width: 240 }}
          />
          <Space>
            <span className="perm-assign-count">
              {t('已选')} <b>{selectedPerms.length}</b> / {allPerms.length}
            </span>
            <Button
              size="small"
              onClick={() =>
                setSelectedPerms(Array.from(new Set([...selectedPerms, ...filteredPerms.map((p) => p.id)])))
              }
            >
              {t('全选')}
            </Button>
            <Button size="small" onClick={() => setSelectedPerms([])}>{t('清空')}</Button>
          </Space>
        </div>
        <div className="perm-assign-list">
          {filteredPerms.length === 0 ? (
            <GlassEmpty compact text="无匹配权限" />
          ) : (
            filteredPerms.map((p) => {
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
            })
          )}
        </div>
      </Modal>
    </>
  )
}
