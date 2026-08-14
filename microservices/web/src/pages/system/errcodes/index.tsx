import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Grid, Space, Tag, Form, Input, Select,
  Alert, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined,
  EditOutlined, DeleteOutlined, PoweroffOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as ErrCodeAPI from '@/api/system/errcodes'
import type { ErrorCodeItem } from '@/api/system/errcodes'
import EntityFormModal from '@/components/common/EntityFormModal'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableRowActions from '@/components/common/TableRowActions'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/common/StatusPill'
import { useCrudModal } from '@/hooks/useCrudModal'
import { usePagination } from '@/hooks/usePagination'
import './styles.css'

interface PageParams {
  page: number
  page_size: number
  keyword?: string
  scope?: string
  status?: number
}

export default function ErrCodesPage() {
  const [list, setList] = useState<ErrorCodeItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<Omit<PageParams, 'page' | 'page_size'>>({})
  const pagination = usePagination()
  const modal = useCrudModal<ErrorCodeItem>()
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = async (p: PageParams) => {
    setLoading(true)
    try {
      const res = await ErrCodeAPI.getErrCodeList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取错误码列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchList({ ...filters, page: pagination.page, page_size: pagination.page_size })
    // fetchList 仅依赖查询参数，避免请求函数引用变化触发额外加载
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters, pagination.page, pagination.page_size])

  const handleSearch = (values: { keyword?: string; scope?: string; status?: number }) => {
    setFilters(values)
    pagination.resetPagination()
  }

  const handleReset = () => {
    searchForm.resetFields()
    setFilters({})
    pagination.resetPagination()
  }

  const openCreate = () => {
    form.resetFields()
    modal.openCreate()
  }

  const openEdit = (record: ErrorCodeItem) => {
    modal.openEdit(record)
    form.setFieldsValue({
      code: record.code,
      message: record.message,
      memo: record.memo,
      scope: record.scope,
      status: record.status,
    })
  }

  const handleDelete = async (id: number) => {
    try {
      await ErrCodeAPI.deleteErrCode(id)
      message.success(t('删除成功'))
      if (list.length === 1 && pagination.page > 1) {
        pagination.updatePagination({ page: pagination.page - 1 })
      } else {
        void fetchList({ ...filters, page: pagination.page, page_size: pagination.page_size })
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  // 启停开关：停用后各服务回落到代码默认文案
  const handleToggleStatus = async (record: ErrorCodeItem) => {
    const next = record.status === 1 ? 0 : 1
    try {
      await ErrCodeAPI.updateErrCode(record.id, { status: next })
      message.success(next === 1 ? t('已启用') : t('已停用'))
      void fetchList({ ...filters, page: pagination.page, page_size: pagination.page_size })
    } catch {
      message.error(t('操作失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (modal.record) {
        // code 是稳定标识不可改，更新时不提交
        const { code: _code, ...data } = values
        await ErrCodeAPI.updateErrCode(modal.record.id, data)
        message.success(t('更新成功，约 30 秒内热生效'))
      } else {
        await ErrCodeAPI.createErrCode(values)
        message.success(t('创建成功，约 30 秒内热生效'))
      }
      modal.close()
      void fetchList({ ...filters, page: pagination.page, page_size: pagination.page_size })
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<ErrorCodeItem> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    {
      title: t('错误码'),
      dataIndex: 'code',
      width: 260,
      ellipsis: { showTitle: false },
      render: (v: string) => (
        <Tooltip title={v} placement="topLeft">
          <Tag variant="filled" className="cell-mono errcode-code-tag">{v}</Tag>
        </Tooltip>
      ),
    },
    {
      title: t('对外文案'),
      dataIndex: 'message',
      width: 320,
      ellipsis: { showTitle: false },
      render: (v: string) => (
        <Tooltip title={v} placement="topLeft">
          <span className="errcode-message-text">{v}</span>
        </Tooltip>
      ),
    },
    {
      title: t('来源'),
      dataIndex: 'scope',
      width: 110,
      responsive: ['sm'],
      render: (v: string) => <Tag className="cell-mono errcode-scope-tag">{v || 'global'}</Tag>,
    },
    {
      title: t('内部备注'),
      dataIndex: 'memo',
      width: 300,
      ellipsis: { showTitle: false },
      responsive: ['md'],
      render: (v: string) => v ? (
        <Tooltip title={v} placement="topLeft">
          <span className="errcode-memo-text">{v}</span>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    { title: t('更新时间'), dataIndex: 'updated_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: compactActions ? 48 : 132,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: record.code })}
          className="errcode-row-actions"
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:errcode:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'status',
              label: record.status === 1 ? t('停用') : t('启用'),
              icon: <PoweroffOutlined />,
              danger: record.status === 1,
              show: hasPerm('system:errcode:update'),
              confirm: record.status === 1 ? t('停用后各服务将回落到默认文案，确认停用?') : t('确认启用?'),
              onClick: () => handleToggleStatus(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:errcode:delete'),
              confirm: t('确认删除?'),
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
        className="errcode-page"
        filter={(
          <>
            <Alert
              className="errcode-alert"
              type="info"
              showIcon
              message={t('错误文案在线修改，保存后各服务约 30 秒内热生效，无需重启')}
              description={t('错误码标识（code）与后端代码对齐，创建后不可修改；停用或删除某错误码后，对应接口回落到代码里的默认文案。')}
            />
            <ListFilterForm form={searchForm} onFinish={handleSearch}>
              <Form.Item name="keyword" className="errcode-filter-keyword">
                <Input placeholder={t('搜索错误码 / 文案 / 备注')} prefix={<SearchOutlined />} allowClear />
              </Form.Item>
              <Form.Item name="scope" className="errcode-filter-scope">
                <Input placeholder={t('来源（如 system）')} allowClear />
              </Form.Item>
              <Form.Item name="status" className="errcode-filter-status">
                <Select placeholder={t('状态')} allowClear>
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
          </>
        )}
        toolbar={(
          <TableToolbar
            title="错误码"
            total={total}
            extra={(
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => void fetchList({ ...filters, page: pagination.page, page_size: pagination.page_size })}>{t('刷新')}</Button>
                {hasPerm('system:errcode:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增错误码')}</Button>
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
          rowClassName={(record) => record.status === 0 ? 'errcode-row-disabled' : ''}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无错误码" compact /> }}
          pagination={{
            total,
            current: pagination.page,
            pageSize: pagination.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => pagination.setPage(page, page_size),
          }}
        />
      </ListPageShell>
      <EntityFormModal
        title={modal.record ? t('编辑错误码') : t('新增错误码')}
        open={modal.open}
        form={form}
        onClose={modal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        formProps={{ style: { marginTop: 16 } }}
      >
        <Form.Item
          name="code"
          label={t('错误码标识')}
          tooltip={t('与后端 response.ErrorCode 常量对齐，如 DICT_TYPE_NOT_FOUND；创建后不可修改')}
          rules={[{ required: true, message: t('请输入错误码标识') }]}
        >
          <Input placeholder={t('如 DICT_TYPE_NOT_FOUND')} disabled={!!modal.record} className="cell-mono" />
        </Form.Item>
        <Form.Item name="message" label={t('对外文案')} rules={[{ required: true, message: t('请输入对外文案') }]}>
          <Input.TextArea rows={2} maxLength={512} showCount placeholder={t('用户可见的错误提示文案')} />
        </Form.Item>
        <Form.Item name="memo" label={t('内部备注')}>
          <Input.TextArea rows={2} maxLength={255} placeholder={t('排查提示、默认文案对照等（不对外返回）')} />
        </Form.Item>
        <Form.Item name="scope" label={t('来源')} initialValue="global" tooltip={t('产生该错误码的服务/模块，便于筛选')}>
          <Input placeholder={t('如 system / auth / global')} />
        </Form.Item>
        <Form.Item name="status" label={t('状态')} initialValue={1}>
          <Select>
            <Select.Option value={1}>{t('启用')}</Select.Option>
            <Select.Option value={0}>{t('禁用')}</Select.Option>
          </Select>
        </Form.Item>
      </EntityFormModal>
    </>
  )
}
