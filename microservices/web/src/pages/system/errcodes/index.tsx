import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Alert, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined,
  EditOutlined, DeleteOutlined, PoweroffOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as ErrCodeAPI from '@/api/system/errcodes'
import type { ErrorCodeItem } from '@/api/system/errcodes'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
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
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<ErrorCodeItem | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

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

  useEffect(() => { fetchList(params) }, [params])

  const handleSearch = (values: { keyword?: string; scope?: string; status?: number }) => {
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

  const openEdit = (record: ErrorCodeItem) => {
    setEditRecord(record)
    form.setFieldsValue({
      code: record.code,
      message: record.message,
      memo: record.memo,
      scope: record.scope,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await ErrCodeAPI.deleteErrCode(id)
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

  // 启停开关：停用后各服务回落到代码默认文案
  const handleToggleStatus = async (record: ErrorCodeItem) => {
    const next = record.status === 1 ? 0 : 1
    try {
      await ErrCodeAPI.updateErrCode(record.id, { status: next })
      message.success(next === 1 ? t('已启用') : t('已停用'))
      fetchList(params)
    } catch {
      message.error(t('操作失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        // code 是稳定标识不可改，更新时不提交
        const { code: _code, ...data } = values
        await ErrCodeAPI.updateErrCode(editRecord.id, data)
        message.success(t('更新成功，约 30 秒内热生效'))
      } else {
        await ErrCodeAPI.createErrCode(values)
        message.success(t('创建成功，约 30 秒内热生效'))
      }
      setModalOpen(false)
      fetchList(params)
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
      width: 132,
      fixed: 'right',
      render: (_, record) => (
        <Space size={2} className="table-actions errcode-row-actions">
          {hasPerm('system:errcode:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={`${t('编辑错误码')} ${record.code}`} icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:errcode:update') && (
            <Popconfirm
              title={record.status === 1 ? t('停用后各服务将回落到默认文案，确认停用?') : t('确认启用?')}
              onConfirm={() => handleToggleStatus(record)}
            >
              <Tooltip title={record.status === 1 ? t('停用') : t('启用')}>
                <Button
                  type="text"
                  size="small"
                  danger={record.status === 1}
                  className={record.status === 1 ? 'errcode-status-stop' : 'errcode-status-start'}
                  aria-label={`${record.status === 1 ? t('停用') : t('启用')}${t('错误码')} ${record.code}`}
                  icon={<PoweroffOutlined />}
                />
              </Tooltip>
            </Popconfirm>
          )}
          {hasPerm('system:errcode:delete') && (
            <Popconfirm title={t('确认删除?')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={`${t('删除错误码')} ${record.code}`} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list errcode-page">
      <Alert
        className="errcode-alert"
        type="info"
        showIcon
        message={t('错误文案在线修改，保存后各服务约 30 秒内热生效，无需重启')}
        description={t('错误码标识（code）与后端代码对齐，创建后不可修改；停用或删除某错误码后，对应接口回落到代码里的默认文案。')}
      />
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" onFinish={handleSearch}>
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
        </Form>
      </Card>
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="错误码"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>{t('刷新')}</Button>
              {hasPerm('system:errcode:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增错误码')}</Button>
              )}
            </Space>
          }
        />
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
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>
      <Modal
        title={editRecord ? t('编辑错误码') : t('新增错误码')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="code"
            label={t('错误码标识')}
            tooltip={t('与后端 response.ErrorCode 常量对齐，如 DICT_TYPE_NOT_FOUND；创建后不可修改')}
            rules={[{ required: true, message: t('请输入错误码标识') }]}
          >
            <Input placeholder={t('如 DICT_TYPE_NOT_FOUND')} disabled={!!editRecord} className="cell-mono" />
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
        </Form>
      </Modal>
    </div>
  )
}
