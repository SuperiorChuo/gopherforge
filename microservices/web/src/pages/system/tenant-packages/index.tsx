import { useEffect, useMemo, useState } from 'react'
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tree } from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DataNode } from 'antd/es/tree'
import { message } from '@/utils/feedback'
import type { Permission, TenantPackageInfo } from '@/types'
import * as PackageAPI from '@/api/system/tenantPackages'
import { getPermissionList } from '@/api/system/permission'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

/** 按 parent_id 将权限平铺列表组树（数据源与权限管理页一致） */
function buildPermissionTree(perms: Permission[]): DataNode[] {
  const byParent = new Map<number, Permission[]>()
  const ids = new Set(perms.map((p) => p.id))
  for (const p of perms) {
    // 父节点缺失（如分页截断）时归入根，避免节点丢失
    const parent = p.parent_id && ids.has(p.parent_id) ? p.parent_id : 0
    const list = byParent.get(parent) ?? []
    list.push(p)
    byParent.set(parent, list)
  }
  const build = (parentId: number): DataNode[] =>
    (byParent.get(parentId) ?? []).map((p) => ({
      key: p.code,
      title: `${p.name}（${p.code}）`,
      children: build(p.id),
    }))
  return build(0)
}

export default function TenantPackagePage() {
  const [list, setList] = useState<TenantPackageInfo[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<TenantPackageInfo | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [allPerms, setAllPerms] = useState<Permission[]>([])
  const [checkedCodes, setCheckedCodes] = useState<string[]>([])
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const treeData = useMemo(() => buildPermissionTree(allPerms), [allPerms])
  const allCodes = useMemo(() => allPerms.map((p) => p.code), [allPerms])

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await PackageAPI.getTenantPackageList({
        page: p.page,
        page_size: p.page_size,
        keyword: p.keyword || undefined,
      })
      setList(res.list || [])
      setTotal(res.total ?? 0)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '获取套餐列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params])

  useEffect(() => {
    // 权限树复用权限管理页的数据源（平铺列表，前端组树）
    getPermissionList({ page: 1, page_size: 500 })
      .then((res) => setAllPerms(res.list || []))
      .catch(() => message.error('加载权限树失败'))
  }, [])

  const handleSearch = (values: { keyword?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  function openCreate() {
    setEditRecord(null)
    form.resetFields()
    form.setFieldsValue({ status: 1 })
    setCheckedCodes([])
    setModalOpen(true)
  }

  function openEdit(row: TenantPackageInfo) {
    setEditRecord(row)
    form.setFieldsValue({ name: row.name, status: row.status, remark: row.remark })
    setCheckedCodes(row.permission_codes || [])
    setModalOpen(true)
  }

  async function onSubmit() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await PackageAPI.updateTenantPackage(editRecord.id, {
          name: values.name,
          permission_codes: checkedCodes,
          status: values.status,
          remark: values.remark ?? '',
        })
        message.success('套餐已更新（改小套餐不回收存量角色权限，仅拦截新分配）')
      } else {
        await PackageAPI.createTenantPackage({
          name: values.name,
          permission_codes: checkedCodes,
          status: values.status,
          remark: values.remark ?? '',
        })
        message.success('套餐已创建')
      }
      setModalOpen(false)
      fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function onDelete(row: TenantPackageInfo) {
    try {
      await PackageAPI.deleteTenantPackage(row.id)
      message.success('套餐已删除')
      fetchList(params)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '删除失败（有租户绑定时需先解绑）')
    }
  }

  const columns: ColumnsType<TenantPackageInfo> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '名称', dataIndex: 'name', width: 200 },
    {
      title: '权限数',
      dataIndex: 'permission_codes',
      width: 100,
      render: (v: string[]) => <Tag variant="filled">{v?.length ?? 0}</Tag>,
    },
    { title: '备注', dataIndex: 'remark', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: number) =>
        v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 160,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:tenant-package:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(row)}>
              编辑
            </Button>
          )}
          {hasPerm('system:tenant-package:delete') && (
            <Popconfirm title="确定删除该套餐？有租户绑定时将拒绝删除。" onConfirm={() => void onDelete(row)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list tenant-package-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索套餐名称" prefix={<SearchOutlined />} allowClear style={{ width: 240 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="租户套餐"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:tenant-package:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                  新建套餐
                </Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          dataSource={list}
          columns={columns}
          locale={{ emptyText: <GlassEmpty text="暂无套餐" compact /> }}
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
        title={editRecord ? `编辑套餐 #${editRecord.id}` : '新建套餐'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void onSubmit()}
        confirmLoading={submitting}
        width={640}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="套餐名称" rules={[{ required: true, message: '必填' }]}>
            <Input placeholder="如：基础版 / 专业版" maxLength={128} />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select
              options={[
                { label: '启用', value: 1 },
                { label: '停用', value: 0 },
              ]}
            />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} maxLength={255} placeholder="套餐说明（可选）" />
          </Form.Item>
          <Form.Item
            label={`套餐权限（已选 ${checkedCodes.length} 项）`}
            extra="严格勾选（父子不联动），与角色授权页的平铺勾选语义一致"
          >
            <Space style={{ marginBottom: 8 }}>
              <Button size="small" onClick={() => setCheckedCodes(allCodes)}>全选</Button>
              <Button size="small" onClick={() => setCheckedCodes([])}>清空</Button>
            </Space>
            <div style={{ maxHeight: 320, overflow: 'auto', border: '1px solid rgba(148, 163, 184, 0.25)', borderRadius: 8, padding: 8 }}>
              <Tree
                checkable
                checkStrictly
                selectable={false}
                defaultExpandAll
                treeData={treeData}
                checkedKeys={{ checked: checkedCodes, halfChecked: [] }}
                onCheck={(keys) => {
                  const checked = Array.isArray(keys) ? keys : keys.checked
                  setCheckedCodes(checked.map(String))
                }}
              />
            </div>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
