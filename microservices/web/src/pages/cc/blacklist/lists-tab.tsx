import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Input, Form, Modal, Radio, Tag } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, PlusOutlined, DeleteOutlined, StopOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listCcRiskLists, createCcRiskList, deleteCcRiskList, type CcRiskListEntry } from '@/api/cc'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

export default function OutboundListsTab() {
  const { t } = useTranslation()
  const [modalOpen, setModalOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [form] = Form.useForm<{ number: string; match: 'exact' | 'prefix'; list_type: 'black' | 'white'; remark?: string }>()

  const fetcher = useCallback(async () => {
    const res = await listCcRiskLists()
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取外呼名单失败')), [t]),
  })

  const handleAdd = async (values: { number: string; match: 'exact' | 'prefix'; list_type: 'black' | 'white'; remark?: string }) => {
    setAdding(true)
    try {
      await createCcRiskList({
        number: values.number.trim(),
        match: values.match,
        list_type: values.list_type,
        remark: values.remark?.trim(),
      })
      message.success(values.list_type === 'white'
        ? t('{{number}} 已加入白名单，外呼将跳过其余风控', { number: values.number })
        : t('{{number}} 已加入外呼黑名单', { number: values.number }))
      setModalOpen(false)
      form.resetFields()
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('添加失败'))
    } finally {
      setAdding(false)
    }
  }

  const handleRemove = async (row: CcRiskListEntry) => {
    try {
      await deleteCcRiskList(row.id)
      message.success(t('{{number}} 已移出名单', { number: row.number }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('移除失败'))
    }
  }

  const columns: ColumnsType<CcRiskListEntry> = [
    {
      title: t('号码 / 前缀'),
      dataIndex: 'number',
      width: 180,
      render: (v: string) => (
        <Space size={6}><StopOutlined style={{ color: 'var(--c-error-strong)' }} /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'list_type',
      width: 100,
      render: (v: string) => v === 'white'
        ? <Tag color="green" variant="filled">{t('白名单')}</Tag>
        : <Tag color="red" variant="filled">{t('黑名单')}</Tag>,
    },
    {
      title: t('匹配'),
      dataIndex: 'match',
      width: 100,
      render: (v: string) => v === 'prefix' ? t('前缀') : t('精确'),
    },
    {
      title: t('备注'),
      dataIndex: 'remark',
      render: (v: string) => v || <span className="cell-dim">—</span>,
    },
    { title: t('添加时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
    {
      title: t('操作'),
      key: 'action',
      width: 48,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, r) => (
        <TableRowActions
          maxInline={1}
          actions={[
            {
              key: 'remove',
              label: t('移除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('把 {{number}} 移出名单？', { number: r.number }),
              onClick: () => { void handleRemove(r) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <div className="list-main-card">
      <TableToolbar
        title="外呼名单"
        total={list.length}
        extra={
          <Space size={16}>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.setFieldsValue({ match: 'exact', list_type: 'black' }); setModalOpen(true) }}>{t('添加')}</Button>
            <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
          </Space>
        }
      />
      <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
        {t('只拦坐席台 / CRM 点击外呼。白名单命中后跳过时段、频次和字冠规则。软电话直拨本批不拦。')}
      </div>
      <Table
        rowKey="id"
        scroll={{ x: 900 }}
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
        locale={{ emptyText: <GlassEmpty text="暂无外呼名单" compact /> }}
      />

      <Modal
        title={t('添加外呼名单')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={adding}
        okText={t('添加')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleAdd} style={{ marginTop: 8 }} initialValues={{ match: 'exact', list_type: 'black' }}>
          <Form.Item
            name="number"
            label={t('号码 / 前缀')}
            dependencies={['match']}
            rules={[
              { required: true, message: t('请输入号码') },
              ({ getFieldValue }) => ({
                validator(_, value: string) {
                  const v = (value || '').trim()
                  const prefix = getFieldValue('match') === 'prefix'
                  const re = prefix ? /^\+?[0-9*#]{1,16}$/ : /^\+?[0-9*#]{3,32}$/
                  if (!v || re.test(v)) return Promise.resolve()
                  return Promise.reject(new Error(prefix ? t('前缀为 1-16 位数字') : t('3-32 位数字（可带 +）')))
                },
              }),
            ]}
          >
            <Input placeholder={t('如 13800138000 或 00')} />
          </Form.Item>
          <Form.Item name="list_type" label={t('名单类型')} rules={[{ required: true }]}>
            <Radio.Group>
              <Radio.Button value="black">{t('黑名单')}</Radio.Button>
              <Radio.Button value="white">{t('白名单')}</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item name="match" label={t('匹配方式')} rules={[{ required: true }]}>
            <Radio.Group>
              <Radio.Button value="exact">{t('精确')}</Radio.Button>
              <Radio.Button value="prefix">{t('前缀')}</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item name="remark" label={t('备注（可选）')}>
            <Input placeholder={t('如：投诉号码 / VIP 客户')} maxLength={128} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
