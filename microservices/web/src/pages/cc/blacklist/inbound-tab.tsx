import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Input, Form, Modal } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, PlusOutlined, DeleteOutlined, StopOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listBlacklist, addBlacklist, removeBlacklist, type BlacklistEntry } from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

export default function InboundBlacklistTab() {
  const { t } = useTranslation()
  const [modalOpen, setModalOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [form] = Form.useForm<{ number: string; remark?: string }>()

  const fetcher = useCallback(async () => {
    const res = await listBlacklist()
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取黑名单失败')), [t]),
  })

  const handleAdd = async (values: { number: string; remark?: string }) => {
    setAdding(true)
    try {
      await addBlacklist(values.number.trim(), values.remark?.trim())
      message.success(t('{{number}} 已拉黑，来电将被直接拒接', { number: values.number }))
      setModalOpen(false)
      form.resetFields()
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('添加失败'))
    } finally {
      setAdding(false)
    }
  }

  const handleRemove = async (number: string) => {
    try {
      await removeBlacklist(number)
      message.success(t('{{number}} 已移出黑名单', { number }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('移除失败'))
    }
  }

  const columns: ColumnsType<BlacklistEntry> = [
    {
      title: t('号码'),
      dataIndex: 'number',
      width: 200,
      render: (v: string) => (
        <Space size={6}><StopOutlined style={{ color: 'var(--c-error-strong)' }} /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('备注'),
      dataIndex: 'remark',
      render: (v: string) => v || <span className="cell-dim">—</span>,
    },
    { title: t('拉黑时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
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
              confirm: t('把 {{number}} 移出黑名单？', { number: r.number }),
              onClick: () => { void handleRemove(r.number) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-blacklist-page"
      toolbar={
        <TableToolbar
          title="来电黑名单"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>{t('添加号码')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
      <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
        {t('名单内号码经外线打入时直接拒接（605 Rejected），不振铃、不进队列；增删即时生效。仅平台管理员可改。')}
      </div>
      <Table
        rowKey="number"
        scroll={{ x: 800 }}
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ pageSize: 20, hideOnSinglePage: true, showSizeChanger: false }}
        locale={{ emptyText: <GlassEmpty text="暂无拉黑号码" compact /> }}
      />

      <Modal
        title={t('添加黑名单号码')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={adding}
        okText={t('拉黑')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleAdd} style={{ marginTop: 8 }}>
          <Form.Item
            name="number"
            label={t('号码')}
            rules={[
              { required: true, message: t('请输入号码') },
              { pattern: /^\+?[0-9*#]{3,32}$/, message: t('3-32 位数字（可带 +）') },
            ]}
          >
            <Input placeholder={t('来电显示的主叫号码，如 13800001111')} />
          </Form.Item>
          <Form.Item name="remark" label={t('备注（可选）')}>
            <Input placeholder={t('如：骚扰电话 / 恶意呼叫')} maxLength={128} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
