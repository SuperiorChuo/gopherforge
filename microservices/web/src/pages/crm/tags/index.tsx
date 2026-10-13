import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Tag, Modal, Form, Input, ColorPicker, Grid } from 'antd'
import { message } from '@/utils/feedback'
import { PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listTags, createTag, updateTag, deleteTag, type CrmTag } from '@/api/crm/tag'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

export default function CrmTagsPage() {
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<CrmTag | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetcher = useCallback(async () => {
    const res = await listTags()
    const list = res ?? []
    return { list, total: list.length }
  }, [])
  const { list, total, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取标签列表失败')), [t]),
  })

  const openCreate = () => { setEditRecord(null); form.resetFields(); form.setFieldValue('color', '#1677ff'); setModalOpen(true) }
  const openEdit = (r: CrmTag) => { setEditRecord(r); form.setFieldsValue(r); setModalOpen(true) }

  const handleDelete = async (id: number) => {
    try { await deleteTag(id); message.success(t('删除成功')); void reload() }
    catch { message.error(t('删除失败')) }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord?.id) { await updateTag(editRecord.id, values); message.success(t('更新成功')) }
      else { await createTag(values); message.success(t('创建成功')) }
      setModalOpen(false); void reload()
    } catch { message.error(t('操作失败')) }
    finally { setSubmitting(false) }
  }

  const columns: ColumnsType<CrmTag> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('名称'), dataIndex: 'name', width: 200,
      render: (v: string, r: CrmTag) => <Space><span style={{ width: 14, height: 14, borderRadius: 4, background: r.color, display: 'inline-block' }} /><span>{v}</span></Space>,
    },
    { title: t('颜色'), dataIndex: 'color', width: 120, render: (v: string) => <Tag color={v}>{v}</Tag> },
    {
      title: t('操作'), width: compactActions ? 48 : 88, fixed: 'right' as const, align: 'center' as const,
      render: (_, r) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(r) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除?'),
              onClick: () => { void handleDelete(r.id!) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="crm-tags-page"
      toolbar={
        <TableToolbar
          title="客户标签"
          total={total}
          extra={<Space><Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button><Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增标签')}</Button></Space>}
        />}
      >
        <Table rowKey="id" className="list-table" columns={columns} dataSource={list} loading={loading} locale={{ emptyText: <GlassEmpty text="暂无标签" compact /> }} pagination={false}  scroll={{ x: 960 }} />
      <Modal title={editRecord ? t('编辑标签') : t('新增标签')} open={modalOpen} onOk={handleSubmit} onCancel={() => setModalOpen(false)} confirmLoading={submitting} destroyOnHidden>
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true }]}><Input placeholder={t('如：重点客户、VIP')} /></Form.Item>
          <Form.Item name="color" label={t('颜色')}><ColorPicker format="hex" onChange={(c) => form.setFieldValue('color', c.toHexString())} /></Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
