import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Grid, Space, Tag, Form, Input, Select,
  InputNumber, Row, Col,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, IdcardOutlined,
  EditOutlined, DeleteOutlined, PoweroffOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as PostAPI from '@/api/system/posts'
import type { SystemPost } from '@/api/system/posts'
import EntityFormModal from '@/components/EntityFormModal'
import ListFilterForm from '@/components/ListFilterForm'
import ListPageShell from '@/components/ListPageShell'
import TableRowActions from '@/components/TableRowActions'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

export default function PostPage() {
  const [list, setList] = useState<SystemPost[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SystemPost | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await PostAPI.getPostList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取岗位列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params])

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

  const openEdit = (record: SystemPost) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      sort: record.sort,
      status: record.status,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await PostAPI.deletePost(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      // 删除失败原因（如岗位仍有用户关联）由 request 拦截器统一弹出
    }
  }

  // 启用 / 停用切换
  const handleToggleStatus = async (record: SystemPost) => {
    const next = record.status === 1 ? 0 : 1
    try {
      await PostAPI.updatePost(record.id, { status: next })
      message.success(next === 1 ? t('已启用') : t('已停用'))
      fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await PostAPI.updatePost(editRecord.id, values)
        message.success(t('更新成功'))
      } else {
        await PostAPI.createPost(values)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<SystemPost> = [
    {
      title: t('岗位名称'),
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (v: string) => (
        <span className="post-name-cell">
          <IdcardOutlined className="tree-title-icon" />
          <span className="post-name-text">{v}</span>
        </span>
      ),
    },
    {
      title: t('编码'),
      dataIndex: 'code',
      width: 200,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono post-code-tag">{v}</Tag>,
    },
    { title: t('排序'), dataIndex: 'sort', width: 70 },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: t('备注'),
      dataIndex: 'remark',
      width: 280,
      ellipsis: true,
      responsive: ['md'],
      render: (v: string) => v || <span className="cell-muted">—</span>,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: compactActions ? 48 : 132,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          className="post-row-actions"
          actions={[
            {
              key: 'status',
              label: record.status === 1 ? t('停用') : t('启用'),
              icon: <PoweroffOutlined />,
              danger: record.status === 1,
              show: hasPerm('system:post:update'),
              onClick: () => handleToggleStatus(record),
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:post:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:post:delete'),
              confirm: {
                title: t('确认删除该岗位?'),
                description: t('仍有用户关联时将无法删除'),
              },
              onClick: () => handleDelete(record.id),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <ListPageShell
        className="post-page"
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
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
            title="岗位列表"
            total={total}
            extra={(
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>{t('刷新')}</Button>
                {hasPerm('system:post:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增岗位')}</Button>
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
          rowClassName={(record) => (record.status === 0 ? 'post-row-disabled' : '')}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无岗位" compact /> }}
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

      <EntityFormModal
        title={editRecord ? t('编辑岗位') : t('新增岗位')}
        open={modalOpen}
        form={form}
        onClose={() => setModalOpen(false)}
        onSubmit={handleSubmit}
        submitting={submitting}
        width={560}
        formProps={{ style: { marginTop: 16 } }}
      >
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="name" label={t('岗位名称')} rules={[{ required: true, message: t('请输入岗位名称') }]}>
                <Input placeholder={t('如：研发工程师')} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="code" label={t('岗位编码')} rules={[{ required: true, message: t('请输入岗位编码') }]}>
                <Input placeholder={t('如：dev')} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <Form.Item name="sort" label={t('排序')} initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item name="status" label={t('状态')} initialValue={1}>
                <Select>
                  <Select.Option value={1}>{t('启用')}</Select.Option>
                  <Select.Option value={0}>{t('禁用')}</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="remark" label={t('备注')}>
            <Input.TextArea rows={3} maxLength={500} placeholder={t('可选')} />
          </Form.Item>
      </EntityFormModal>
    </>
  )
}
