import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Alert,
  Button,
  DatePicker,
  Descriptions,
  Dropdown,
  Form,
  Grid,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  type MenuProps,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Notice } from '@/types'
import * as NoticeAPI from '@/api/system/notice'
import EntityDetailDrawer from '@/components/EntityDetailDrawer'
import EntityFormModal from '@/components/EntityFormModal'
import ListFilterForm from '@/components/ListFilterForm'
import ListPageShell from '@/components/ListPageShell'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import dayjs from 'dayjs'
import './style.css'

interface SearchParams {
  keyword?: string
  type?: number
  status?: number
  page: number
  page_size: number
}

interface LifecycleMeta {
  label: string
  color?: string
}

const typeLabels: Record<number, string> = { 1: '通知', 2: '公告' }

function getLifecycle(record: Notice): LifecycleMeta {
  if (record.status !== 1) return { label: '已停用' }

  const now = dayjs()
  if (record.start_time && dayjs(record.start_time).isAfter(now)) {
    return { label: '待生效', color: 'gold' }
  }
  if (record.end_time && !dayjs(record.end_time).isAfter(now)) {
    return { label: '已结束' }
  }
  if (!record.start_time && !record.end_time) {
    return { label: '长期有效', color: 'green' }
  }
  return { label: '生效中', color: 'green' }
}

const formatBoundary = (value?: string) => value ? formatDateTime(value) : '不限'

export default function NoticePage() {
  const [list, setList] = useState<Notice[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loaded, setLoaded] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Notice | null>(null)
  const [viewRecord, setViewRecord] = useState<Notice | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [updatingId, setUpdatingId] = useState<number | null>(null)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const [confirmModal, confirmContextHolder] = Modal.useModal()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactTable = !screens.md
  const requestSequenceRef = useRef(0)

  const fetchList = useCallback(async (nextParams: SearchParams) => {
    const requestSequence = ++requestSequenceRef.current
    setLoading(true)
    try {
      const res = await NoticeAPI.getNoticeList(nextParams)
      if (requestSequence !== requestSequenceRef.current) return
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
      setLoaded(true)
      setLoadFailed(false)
    } catch {
      if (requestSequence !== requestSequenceRef.current) return
      setLoadFailed(true)
    } finally {
      if (requestSequence === requestSequenceRef.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchList(params)
  }, [fetchList, params])

  const handleSearch = (values: { keyword?: string; type?: number; status?: number }) => {
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

  const openEdit = (record: Notice) => {
    setEditRecord(record)
    form.setFieldsValue({
      title: record.title,
      content: record.content,
      type: record.type,
      status: record.status,
      start_time: record.start_time ? dayjs(record.start_time) : undefined,
      end_time: record.end_time ? dayjs(record.end_time) : undefined,
    })
    setModalOpen(true)
  }

  const handleDelete = async (record: Notice) => {
    try {
      await NoticeAPI.deleteNotice(record.id)
      message.success('删除成功')
      if (viewRecord?.id === record.id) setViewRecord(null)
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        await fetchList(params)
      }
    } catch (error) {
      message.error('删除失败')
      throw error
    }
  }

  const confirmDelete = (record: Notice) => {
    confirmModal.confirm({
      className: 'notice-delete-confirm',
      title: '删除通知公告',
      content: `确认删除“${record.title}”？删除后无法恢复。`,
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: () => handleDelete(record),
    })
  }

  const handleToggleStatus = async (record: Notice, checked: boolean) => {
    if (updatingId !== null) return
    setUpdatingId(record.id)
    try {
      const status = checked ? 1 : 0
      await NoticeAPI.updateNoticeStatus(record.id, status)
      setList((current) => current.map((item) => item.id === record.id ? { ...item, status } : item))
      setViewRecord((current) => current?.id === record.id ? { ...current, status } : current)
      message.success(checked ? '已启用' : '已停用')
    } catch {
      message.error('状态更新失败')
    } finally {
      setUpdatingId(null)
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const { start_time, end_time, ...rest } = values
      const payload = {
        ...rest,
        status: editRecord ? rest.status ?? editRecord.status : 1,
        start_time: start_time?.toISOString(),
        end_time: end_time?.toISOString(),
      }
      if (editRecord) {
        await NoticeAPI.updateNotice(editRecord.id, payload)
        message.success('更新成功')
      } else {
        await NoticeAPI.createNotice(payload)
        message.success('创建成功')
      }
      setModalOpen(false)
      await fetchList(params)
    } catch {
      message.error('操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const renderSubject = (record: Notice, compact = false) => {
    const lifecycle = getLifecycle(record)
    return (
      <div className={`notice-subject${compact ? ' notice-subject-compact' : ''}`}>
        <Tooltip title={record.title} placement="topLeft">
          <button
            type="button"
            className="notice-title-button"
            onClick={() => setViewRecord(record)}
          >
            {record.title}
          </button>
        </Tooltip>
        <span className="notice-summary">{record.content || '暂无正文'}</span>
        {compact && (
          <span className="notice-subject-meta">
            <Tag variant="filled" color={record.type === 1 ? 'blue' : 'orange'}>
              {typeLabels[record.type] ?? record.type}
            </Tag>
            <Tag color={lifecycle.color}>{lifecycle.label}</Tag>
          </span>
        )}
      </div>
    )
  }

  const renderSwitch = (record: Notice, compact = false) => (
    <Switch
      size="small"
      checked={record.status === 1}
      checkedChildren={compact ? undefined : '启用'}
      unCheckedChildren={compact ? undefined : '停用'}
      loading={updatingId === record.id}
      disabled={!hasPerm('system:notice:update') || updatingId !== null}
      aria-label={`${record.title}：${record.status === 1 ? '停用' : '启用'}`}
      onChange={(checked) => handleToggleStatus(record, checked)}
    />
  )

  const compactActions = (): MenuProps['items'] => [
    {
      key: 'view',
      icon: <EyeOutlined />,
      label: '查看详情',
    },
    hasPerm('system:notice:update') ? {
      key: 'edit',
      icon: <EditOutlined />,
      label: '编辑通知',
    } : null,
    hasPerm('system:notice:delete') ? {
      key: 'delete',
      icon: <DeleteOutlined />,
      label: '删除通知',
      danger: true,
    } : null,
  ].filter((item): item is NonNullable<typeof item> => item !== null)

  const runCompactAction = (key: string, record: Notice) => {
    if (key === 'view') setViewRecord(record)
    if (key === 'edit') openEdit(record)
    if (key === 'delete') confirmDelete(record)
  }

  const desktopColumns: ColumnsType<Notice> = [
    { title: 'ID', dataIndex: 'id', width: 64, responsive: ['xl'] },
    {
      title: '公告主体',
      width: 320,
      render: (_, record) => renderSubject(record),
    },
    {
      title: '类型',
      dataIndex: 'type',
      width: 80,
      render: (value: number) => (
        <Tag variant="filled" color={value === 1 ? 'blue' : 'orange'} className="notice-type-tag">
          {typeLabels[value] ?? value}
        </Tag>
      ),
    },
    {
      title: '有效期',
      width: 248,
      render: (_, record) => {
        const lifecycle = getLifecycle(record)
        return (
          <div className="notice-period">
            <Tag color={lifecycle.color}>{lifecycle.label}</Tag>
            <span>始 {formatBoundary(record.start_time)}</span>
            <span>止 {formatBoundary(record.end_time)}</span>
          </div>
        )
      },
    },
    {
      title: '启用',
      width: 90,
      align: 'center',
      render: (_, record) => renderSwitch(record),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 170,
      className: 'cell-time',
      responsive: ['xl'],
      render: formatDateTime,
    },
    {
      title: '操作',
      width: 116,
      fixed: 'right',
      align: 'center',
      className: 'notice-actions-cell',
      render: (_, record) => (
        <Space size={2} className="table-actions compact-table-actions notice-actions">
          <Tooltip title="查看详情">
            <Button
              type="text"
              size="small"
              icon={<EyeOutlined />}
              aria-label={`查看通知：${record.title}`}
              onClick={() => setViewRecord(record)}
            />
          </Tooltip>
          {hasPerm('system:notice:update') && (
            <Tooltip title="编辑">
              <Button
                type="text"
                size="small"
                icon={<EditOutlined />}
                aria-label={`编辑通知：${record.title}`}
                onClick={() => openEdit(record)}
              />
            </Tooltip>
          )}
          {hasPerm('system:notice:delete') && (
            <Tooltip title="删除">
              <Button
                type="text"
                size="small"
                danger
                icon={<DeleteOutlined />}
                aria-label={`删除通知：${record.title}`}
                onClick={() => confirmDelete(record)}
              />
            </Tooltip>
          )}
        </Space>
      ),
    },
  ]

  const compactColumns: ColumnsType<Notice> = [
    {
      title: '公告主体',
      width: 206,
      className: 'notice-subject-cell',
      render: (_, record) => renderSubject(record, true),
    },
    {
      title: '启用',
      width: 66,
      align: 'center',
      className: 'notice-switch-cell',
      render: (_, record) => renderSwitch(record, true),
    },
    {
      title: <Tooltip title="更多操作"><MoreOutlined /></Tooltip>,
      width: 48,
      align: 'center',
      className: 'notice-more-cell',
      render: (_, record) => (
        <Dropdown
          trigger={['click']}
          placement="bottomRight"
          menu={{ items: compactActions(), onClick: ({ key }) => runCompactAction(key, record) }}
        >
          <Button
            type="text"
            size="small"
            icon={<MoreOutlined />}
            aria-label={`更多操作：${record.title}`}
          />
        </Dropdown>
      ),
    },
  ]

  const columns = compactTable ? compactColumns : desktopColumns

  return (
    <>
      {confirmContextHolder}
      <ListPageShell
        className="notice-page"
        filter={(
          <ListFilterForm
            form={searchForm}
            onFinish={handleSearch}
            initialValues={params}
          >
          <Form.Item name="keyword">
            <Input placeholder="搜索标题" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="type">
            <Select placeholder="类型" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>通知</Select.Option>
              <Select.Option value={2}>公告</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>停用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
            title="通知公告"
            total={total}
            extra={(
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>刷新</Button>
                {hasPerm('system:notice:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增通知</Button>
                )}
              </Space>
            )}
          />
        )}
      >
        {loadFailed && (
          <Alert
            className="notice-load-alert"
            type={loaded ? 'warning' : 'error'}
            showIcon
            message={loaded ? '公告列表刷新失败，当前显示上次成功数据' : '公告列表暂不可用'}
            action={(
              <Button size="small" icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>
                重试
              </Button>
            )}
          />
        )}
        <Table
          rowKey="id"
          className="list-table notice-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          tableLayout={compactTable ? 'fixed' : 'auto'}
          scroll={{ x: compactTable ? 320 : 'max-content' }}
          rowClassName={(record) => ['已停用', '已结束'].includes(getLifecycle(record).label) ? 'notice-row-muted' : ''}
          locale={{
            emptyText: <GlassEmpty text={loadFailed && !loaded ? '公告列表暂不可用' : '暂无通知公告'} compact />,
          }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: !compactTable,
            showQuickJumper: !compactTable,
            showTotal: (count) => `共 ${count} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </ListPageShell>

      <EntityDetailDrawer
        title="公告详情"
        open={Boolean(viewRecord)}
        onClose={() => setViewRecord(null)}
        width="min(640px, 100vw)"
        destroyOnHidden
        className="notice-detail-drawer"
      >
        {viewRecord && (
          <div className="notice-detail">
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label="标题">
                <span className="notice-detail-title">{viewRecord.title}</span>
              </Descriptions.Item>
              <Descriptions.Item label="类型">
                <Tag variant="filled" color={viewRecord.type === 1 ? 'blue' : 'orange'}>
                  {typeLabels[viewRecord.type] ?? viewRecord.type}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Space size={[6, 6]} wrap>
                  <Tag color={viewRecord.status === 1 ? 'green' : undefined}>
                    {viewRecord.status === 1 ? '启用' : '停用'}
                  </Tag>
                  <Tag color={getLifecycle(viewRecord).color}>{getLifecycle(viewRecord).label}</Tag>
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="开始时间">{formatBoundary(viewRecord.start_time)}</Descriptions.Item>
              <Descriptions.Item label="结束时间">{formatBoundary(viewRecord.end_time)}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{formatDateTime(viewRecord.created_at)}</Descriptions.Item>
            </Descriptions>
            <section className="notice-detail-body">
              <h3>公告正文</h3>
              <div className="notice-detail-copy">{viewRecord.content || '暂无正文'}</div>
            </section>
          </div>
        )}
      </EntityDetailDrawer>

      <EntityFormModal
        className="notice-form-modal"
        title={editRecord ? '编辑通知' : '新增通知'}
        open={modalOpen}
        form={form}
        onClose={() => setModalOpen(false)}
        onSubmit={handleSubmit}
        submitting={submitting}
        width="min(640px, calc(100vw - 24px))"
        formProps={{ className: 'notice-form' }}
      >
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="content"
            label="内容"
            rules={[{ required: true, message: '请输入内容' }]}
          >
            <Input.TextArea rows={5} />
          </Form.Item>
          <div className={`notice-form-grid${editRecord ? '' : ' is-create'}`}>
            <Form.Item name="type" label="类型" initialValue={1}>
              <Select>
                <Select.Option value={1}>通知</Select.Option>
                <Select.Option value={2}>公告</Select.Option>
              </Select>
            </Form.Item>
            {editRecord && (
              <Form.Item name="status" label="状态" preserve={false}>
                <Select>
                  <Select.Option value={1}>启用</Select.Option>
                  <Select.Option value={0}>停用</Select.Option>
                </Select>
              </Form.Item>
            )}
          </div>
          <div className="notice-time-grid">
            <Form.Item name="start_time" label="开始时间">
              <DatePicker
                className="notice-time-picker"
                showTime
                format="YYYY-MM-DD HH:mm:ss"
                placeholder="不限制开始时间"
              />
            </Form.Item>
            <Form.Item
              name="end_time"
              label="结束时间"
              dependencies={['start_time']}
              rules={[{
                validator: (_, value) => {
                  const startTime = form.getFieldValue('start_time')
                  if (!startTime || !value || !value.isBefore(startTime)) return Promise.resolve()
                  return Promise.reject(new Error('结束时间不得早于开始时间'))
                },
              }]}
            >
              <DatePicker
                className="notice-time-picker"
                showTime
                format="YYYY-MM-DD HH:mm:ss"
                placeholder="不限制结束时间"
              />
            </Form.Item>
          </div>
      </EntityFormModal>
    </>
  )
}
