import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Alert, Button, DatePicker, Descriptions, Form, Grid,
  Input, Modal, Popover, Select, Space, Switch, Table, Tag, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  DeleteOutlined, EditOutlined, EyeOutlined, MoreOutlined,
  PlusOutlined, ReloadOutlined, RobotOutlined, SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Notice } from '@/types'
import * as NoticeAPI from '@/api/system/notice'
import { compose as aiCompose } from '@/api/ai'
import EntityDetailDrawer from '@/components/common/EntityDetailDrawer'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableRowActions from '@/components/common/TableRowActions'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
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
  color: string
  rowClass: string
}

const typeLabels: Record<number, string> = { 1: '通知', 2: '公告' }

function validTime(value?: string) {
  if (!value) return null
  const parsed = dayjs(value)
  return parsed.isValid() ? parsed : null
}

function getLifecycle(record: Notice): LifecycleMeta {
  if (record.status !== 1) {
    return { label: '已停用', color: 'default', rowClass: 'notice-row-muted' }
  }

  const now = dayjs()
  const start = validTime(record.start_time)
  const end = validTime(record.end_time)
  if (start?.isAfter(now)) {
    return { label: '待生效', color: 'gold', rowClass: 'notice-row-upcoming' }
  }
  if (end?.isBefore(now)) {
    return { label: '已结束', color: 'default', rowClass: 'notice-row-muted' }
  }
  if (!start && !end) {
    return { label: '长期有效', color: 'green', rowClass: 'notice-row-active' }
  }
  return { label: '生效中', color: 'green', rowClass: 'notice-row-active' }
}

const formatBoundary = (value?: string) => value ? formatDateTime(value) : '不限'

export default function NoticePage() {
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Notice | null>(null)
  const [detailRecord, setDetailRecord] = useState<Notice | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [statusUpdatingID, setStatusUpdatingID] = useState<number | null>(null)
  const [confirmModal, confirmContextHolder] = Modal.useModal()
  // AI 生成：小弹层输入一句话需求，已有内容作为 draft 交给后端润色
  const [aiOpen, setAiOpen] = useState(false)
  const [aiPrompt, setAiPrompt] = useState('')
  const [aiGenerating, setAiGenerating] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactTable = !screens.md

  const fetchList = (p: SearchParams) => NoticeAPI.getNoticeList(p)
  const { list, total, loading, error, reload, patchList } = useTableQuery({ params, fetcher: fetchList })
  const [listLoaded, setListLoaded] = useState(false)
  const loadFailed = error
  useEffect(() => {
    if (!loading && !error) setListLoaded(true)
  }, [error, loading])

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
    setAiOpen(false)
    setAiPrompt('')
    setModalOpen(true)
  }

  const openEdit = (record: Notice) => {
    setEditRecord(record)
    form.setFieldsValue({
      title: record.title,
      content: record.content,
      type: record.type,
      status: record.status,
      start_time: validTime(record.start_time) ?? undefined,
      end_time: validTime(record.end_time) ?? undefined,
    })
    setAiOpen(false)
    setAiPrompt('')
    setModalOpen(true)
  }

  const handleDelete = async (record: Notice) => {
    try {
      await NoticeAPI.deleteNotice(record.id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        await reload()
      }
    } catch (error) {
      message.error(t('删除失败'))
      throw error
    }
  }

  const confirmDelete = (record: Notice) => {
    confirmModal.confirm({
      className: 'notice-delete-confirm',
      title: t('删除通知公告'),
      content: t('确认删除“{{title}}”？删除后无法恢复。', { title: record.title }),
      okText: t('删除'),
      cancelText: t('取消'),
      okButtonProps: { danger: true },
      onOk: () => handleDelete(record),
    })
  }

  const handleToggleStatus = async (record: Notice, checked: boolean) => {
    if (statusUpdatingID !== null) return
    setStatusUpdatingID(record.id)
    try {
      const status = checked ? 1 : 0
      await NoticeAPI.updateNoticeStatus(record.id, status)
      patchList((current) => current.map((item) => item.id === record.id ? { ...item, status } : item))
      setDetailRecord((current) => current?.id === record.id ? { ...current, status } : current)
      message.success(checked ? t('已启用') : t('已停用'))
    } catch {
      message.error(t('状态更新失败'))
    } finally {
      setStatusUpdatingID(null)
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const { start_time, end_time, ...rest } = values
      // 后端按 time.Time 解析，需 RFC3339 格式
      const payload = {
        ...rest,
        status: editRecord ? rest.status ?? editRecord.status : 1,
        start_time: start_time?.toISOString(),
        end_time: end_time?.toISOString(),
      }
      if (editRecord) {
        await NoticeAPI.updateNotice(editRecord.id, payload)
        message.success(t('更新成功'))
      } else {
        await NoticeAPI.createNotice(payload)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      await reload()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleAiGenerate = async () => {
    const prompt = aiPrompt.trim()
    if (!prompt) {
      message.warning(t('请先输入一句话需求'))
      return
    }
    setAiGenerating(true)
    try {
      const draft = (form.getFieldValue('content') || '').trim()
      const res = await aiCompose({ kind: 'notice', prompt, ...(draft ? { draft } : {}) })
      form.setFieldsValue({ content: res.content })
      setAiOpen(false)
      setAiPrompt('')
      message.success(draft ? t('AI 已润色内容') : t('AI 已生成内容'))
    } catch {
      // 拦截器已提示
    } finally {
      setAiGenerating(false)
    }
  }

  const renderSubject = (record: Notice, compact = false) => {
    const lifecycle = getLifecycle(record)
    const summary = record.content?.trim() || t('暂无正文')
    return (
      <div className={`notice-subject${compact ? ' notice-subject-compact' : ''}`}>
        <Tooltip title={record.title} placement="topLeft">
          <button
            type="button"
            className="notice-title-button"
            aria-label={t('查看公告：{{title}}', { title: record.title })}
            onClick={() => setDetailRecord(record)}
          >
            {record.title}
          </button>
        </Tooltip>
        <span className="notice-summary">{summary}</span>
        {compact && (
          <div className="notice-subject-meta">
            <Tag variant="filled" color={record.type === 1 ? 'blue' : 'orange'}>
              {typeLabels[record.type] ? t(typeLabels[record.type] ?? '') : record.type}
            </Tag>
            <Tag variant="filled" color={lifecycle.color}>{t(lifecycle.label)}</Tag>
          </div>
        )}
      </div>
    )
  }

  const renderStatusSwitch = (record: Notice, compact = false) => (
    <Switch
      size="small"
      checked={record.status === 1}
      checkedChildren={compact ? undefined : t('启用')}
      unCheckedChildren={compact ? undefined : t('停用')}
      loading={statusUpdatingID === record.id}
      disabled={!hasPerm('system:notice:update') || statusUpdatingID !== null}
      aria-label={t(record.status === 1 ? '停用公告：{{title}}' : '启用公告：{{title}}', { title: record.title })}
      onChange={(checked) => handleToggleStatus(record, checked)}
    />
  )

  const rowActions = (record: Notice) => [
    {
      key: 'view',
      label: t('查看详情'),
      icon: <EyeOutlined />,
      onClick: () => setDetailRecord(record),
    },
    {
      key: 'edit',
      label: t('编辑'),
      icon: <EditOutlined />,
      show: hasPerm('system:notice:update'),
      onClick: () => openEdit(record),
    },
    {
      key: 'delete',
      label: t('删除'),
      icon: <DeleteOutlined />,
      danger: true,
      show: hasPerm('system:notice:delete'),
      onClick: () => confirmDelete(record),
    },
  ]

  const desktopColumns: ColumnsType<Notice> = [
    { title: 'ID', dataIndex: 'id', width: 64, responsive: ['xl'] },
    {
      title: t('通知公告'),
      width: 320,
      render: (_, record) => renderSubject(record),
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 80,
      render: (value: number) => (
        <Tag variant="filled" color={value === 1 ? 'blue' : 'orange'}>{typeLabels[value] ? t(typeLabels[value] ?? '') : value}</Tag>
      ),
    },
    {
      title: t('有效期'),
      width: 248,
      render: (_, record) => {
        const lifecycle = getLifecycle(record)
        return (
          <div className="notice-period">
            <Tag variant="filled" color={lifecycle.color}>{t(lifecycle.label)}</Tag>
            <span className="notice-period-copy">{t('始')} {formatBoundary(record.start_time)}</span>
            <span className="notice-period-copy">{t('止')} {formatBoundary(record.end_time)}</span>
          </div>
        )
      },
    },
    {
      title: t('启用'),
      width: 90,
      align: 'center',
      render: (_, record) => renderStatusSwitch(record),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 170,
      className: 'cell-time',
      responsive: ['xl'],
      render: formatDateTime,
    },
    {
      title: t('操作'),
      width: 116,
      align: 'center',
      fixed: 'right',
      className: 'notice-action-cell',
      render: (_, record) => (
        <TableRowActions
          maxInline={3}
          ariaLabel={t('更多操作：{{title}}', { title: record.title })}
          className="notice-actions"
          actions={rowActions(record)}
        />
      ),
    },
  ]

  const compactColumns: ColumnsType<Notice> = [
    {
      title: t('公告'),
      width: 206,
      render: (_, record) => renderSubject(record, true),
    },
    {
      title: t('启用'),
      width: 66,
      align: 'center',
      className: 'notice-compact-status',
      render: (_, record) => renderStatusSwitch(record, true),
    },
    {
      title: <Tooltip title={t('操作')}><MoreOutlined /></Tooltip>,
      width: 48,
      align: 'center',
      className: 'notice-action-cell',
      render: (_, record) => (
        <TableRowActions
          menuOnly
          ariaLabel={t('更多操作：{{title}}', { title: record.title })}
          className="notice-actions"
          actions={rowActions(record)}
        />
      ),
    },
  ]

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
              <Input placeholder={t('搜索标题')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
            </Form.Item>
            <Form.Item name="type">
              <Select placeholder={t('类型')} style={{ width: 100 }} allowClear>
                <Select.Option value={1}>{t('通知')}</Select.Option>
                <Select.Option value={2}>{t('公告')}</Select.Option>
              </Select>
            </Form.Item>
            <Form.Item name="status">
              <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
                <Select.Option value={1}>{t('启用')}</Select.Option>
                <Select.Option value={0}>{t('停用')}</Select.Option>
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
            title="通知公告"
            total={total}
            extra={(
              <Space wrap>
                <Button icon={<ReloadOutlined />} loading={loading} onClick={() => { void reload() }}>{t('刷新')}</Button>
                {hasPerm('system:notice:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增通知')}</Button>
                )}
              </Space>
            )}
          />
        )}
      >
        {loadFailed && (
          <Alert
            className="notice-load-alert"
            type={listLoaded ? 'warning' : 'error'}
            showIcon
            message={listLoaded ? t('刷新失败，当前保留上次加载的数据') : t('通知公告暂时无法加载')}
            action={<Button size="small" loading={loading} onClick={() => { void reload() }}>{t('重试')}</Button>}
          />
        )}
        <Table
          rowKey="id"
          className="list-table notice-table"
          columns={compactTable ? compactColumns : desktopColumns}
          dataSource={list}
          loading={loading}
          tableLayout={compactTable ? 'fixed' : 'auto'}
          scroll={{ x: compactTable ? 320 : 'max-content' }}
          rowClassName={(record) => getLifecycle(record).rowClass}
          locale={{
            emptyText: (
              <GlassEmpty
                text={loadFailed && !listLoaded ? '通知公告加载失败' : '暂无通知公告'}
                compact
              />
            ),
          }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: !compactTable,
            showQuickJumper: !compactTable,
            responsive: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </ListPageShell>

      <EntityFormModal
        className="notice-form-modal"
        title={editRecord ? t('编辑通知') : t('新增通知')}
        open={modalOpen}
        form={form}
        onClose={() => setModalOpen(false)}
        onSubmit={handleSubmit}
        submitting={submitting}
        width="min(640px, calc(100vw - 24px))"
        formProps={{ className: 'notice-form' }}
      >
          <Form.Item name="title" label={t('标题')} rules={[{ required: true, message: t('请输入标题') }]}>
            <Input maxLength={200} showCount />
          </Form.Item>
          <Form.Item
            name="content"
            label={
              <Space size={8}>
                {t('内容')}
                <Popover
                  open={aiOpen}
                  onOpenChange={(open) => { if (!aiGenerating) setAiOpen(open) }}
                  trigger="click"
                  placement="topLeft"
                  content={
                    <div className="notice-ai-popover">
                      <Input.TextArea
                        value={aiPrompt}
                        onChange={(event) => setAiPrompt(event.target.value)}
                        placeholder={t('一句话描述需求，如：写一则周六凌晨系统停机维护公告')}
                        autoSize={{ minRows: 2, maxRows: 4 }}
                        disabled={aiGenerating}
                      />
                      <div className="notice-ai-actions">
                        <Button
                          type="primary"
                          size="small"
                          icon={<RobotOutlined />}
                          loading={aiGenerating}
                          onClick={handleAiGenerate}
                        >
                          {(form.getFieldValue('content') || '').trim() ? t('润色现有内容') : t('生成内容')}
                        </Button>
                      </div>
                    </div>
                  }
                >
                  <Button size="small" icon={<RobotOutlined />} onClick={() => setAiOpen(true)}>
                    {t('AI 生成')}
                  </Button>
                </Popover>
              </Space>
            }
            rules={[{ required: true, message: t('请输入内容') }]}
          >
            <Input.TextArea autoSize={{ minRows: 6, maxRows: 12 }} showCount maxLength={10000} />
          </Form.Item>
          <div className={`notice-form-grid${editRecord ? '' : ' is-create'}`}>
            <Form.Item name="type" label={t('类型')} initialValue={1}>
              <Select>
                <Select.Option value={1}>{t('通知')}</Select.Option>
                <Select.Option value={2}>{t('公告')}</Select.Option>
              </Select>
            </Form.Item>
            {editRecord && (
              <Form.Item name="status" label={t('状态')} preserve={false}>
                <Select>
                  <Select.Option value={1}>{t('启用')}</Select.Option>
                  <Select.Option value={0}>{t('停用')}</Select.Option>
                </Select>
              </Form.Item>
            )}
          </div>
          <div className="notice-time-grid">
            <Form.Item name="start_time" label={t('开始时间')}>
              <DatePicker
                className="notice-time-picker"
                showTime
                format="YYYY-MM-DD HH:mm:ss"
                placeholder={t('不限制开始时间')}
              />
            </Form.Item>
            <Form.Item
              name="end_time"
              label={t('结束时间')}
              dependencies={['start_time']}
              rules={[
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    const start = getFieldValue('start_time')
                    if (!start || !value || !value.isBefore(start)) return Promise.resolve()
                    return Promise.reject(new Error(t('结束时间不能早于开始时间')))
                  },
                }),
              ]}
            >
              <DatePicker
                className="notice-time-picker"
                showTime
                format="YYYY-MM-DD HH:mm:ss"
                placeholder={t('不限制结束时间')}
              />
            </Form.Item>
          </div>
      </EntityFormModal>

      <EntityDetailDrawer
        className="notice-detail-drawer"
        title={t('公告详情')}
        open={!!detailRecord}
        onClose={() => setDetailRecord(null)}
        width="min(640px, 100vw)"
        extra={detailRecord && hasPerm('system:notice:update') ? (
          <Button
            icon={<EditOutlined />}
            onClick={() => {
              const record = detailRecord
              setDetailRecord(null)
              openEdit(record)
            }}
          >
            {t('编辑')}
          </Button>
        ) : null}
      >
        {detailRecord && (
          <div className="notice-detail">
            <h3 className="notice-detail-title">{detailRecord.title}</h3>
            <Descriptions bordered size="small" column={{ xs: 1, sm: 2 }}>
              <Descriptions.Item label={t('类型')}>
                <Tag variant="filled" color={detailRecord.type === 1 ? 'blue' : 'orange'}>
                  {typeLabels[detailRecord.type] ? t(typeLabels[detailRecord.type] ?? '') : detailRecord.type}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label={t('状态')}>
                <Tag variant="filled" color={detailRecord.status === 1 ? 'green' : 'default'}>
                  {detailRecord.status === 1 ? t('已启用') : t('已停用')}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label={t('有效状态')}>
                <Tag variant="filled" color={getLifecycle(detailRecord).color}>
                  {t(getLifecycle(detailRecord).label)}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label={t('创建时间')}>{formatDateTime(detailRecord.created_at)}</Descriptions.Item>
              <Descriptions.Item label={t('开始时间')}>{formatBoundary(detailRecord.start_time)}</Descriptions.Item>
              <Descriptions.Item label={t('结束时间')}>{formatBoundary(detailRecord.end_time)}</Descriptions.Item>
            </Descriptions>
            <section className="notice-detail-body">
              <div className="notice-detail-label">{t('公告正文')}</div>
              <div className="notice-detail-copy">{detailRecord.content || t('暂无正文')}</div>
            </section>
          </div>
        )}
      </EntityDetailDrawer>
    </>
  )
}
