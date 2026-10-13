import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Table, Button, Space, Input, Form, Modal, Tag } from 'antd'
import { message } from '@/utils/feedback'
import { ReloadOutlined, PlusOutlined, AudioOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { listCcVoiceNotifies, createCcVoiceNotify, type CcVoiceNotify, HANGUP_CLASS_LABEL } from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'

const STATUS_COLOR: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  queued: 'blue',
  answered: 'processing',
  completed: 'success',
  noanswer: 'default',
  denied: 'warning',
  failed: 'error',
}

export default function CcVoicePage() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<{ to: string; text: string; gateway?: string }>()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  const fetcher = useCallback(async () => {
    const res = await listCcVoiceNotifies({ page, page_size: pageSize })
    return { list: res.list ?? [], total: res.total ?? 0 }
  }, [page, pageSize])
  const { list, total, loading, reload } = useTableQuery({
    params: { page, pageSize },
    fetcher,
    onError: useCallback(() => message.error(t('获取语音通知失败')), [t]),
  })

  const handleCreate = async (values: { to: string; text: string; gateway?: string }) => {
    setSaving(true)
    try {
      await createCcVoiceNotify({
        to: values.to.trim(),
        text: values.text.trim(),
        gateway: values.gateway?.trim() || undefined,
      })
      message.success(t('已入队，接通后将播放合成语音'))
      setOpen(false)
      form.resetFields()
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('发送失败'))
    } finally {
      setSaving(false)
    }
  }

  const columns: ColumnsType<CcVoiceNotify> = [
    { title: t('时间'), dataIndex: 'created_at', width: 180, className: 'cell-time', render: formatDateTime },
    { title: t('被叫'), dataIndex: 'to', width: 150, render: (v: string) => <span className="cell-mono">{v}</span> },
    { title: t('话术'), dataIndex: 'text', ellipsis: true },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 110,
      render: (v: string) => <Tag color={STATUS_COLOR[v] || 'default'}>{t(v)}</Tag>,
    },
    {
      title: t('结果'),
      dataIndex: 'hangup_class',
      width: 110,
      render: (cls: string, row: CcVoiceNotify) => {
        const label = cls ? (HANGUP_CLASS_LABEL[cls] || cls) : ''
        if (label) return <span>{t(label)}</span>
        return row.error || row.reply || <span className="cell-dim">—</span>
      },
    },
  ]

  return (
    <ListPageShell
      className="cc-voice-page"
      toolbar={
        <TableToolbar
          title="语音通知"
          total={total}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>{t('发送语音')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
      <div className="cell-dim" style={{ fontSize: 13, marginBottom: 12 }}>
        {t('文字转语音后直呼被叫播放，不振铃坐席。走外呼风控（名单/时段/频次/字冠）。需已配置 TTS 与出局线路。')}
      </div>
      <Table
        rowKey="id"
        scroll={{ x: 960 }}
        className="list-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: (n) => t('共 {{n}} 条', { n }),
          onChange: (p, ps) => { setPage(p); setPageSize(ps) },
        }}
        locale={{ emptyText: <GlassEmpty text="暂无语音通知" compact /> }}
      />

      <Modal
        title={<Space><AudioOutlined />{t('发送语音通知')}</Space>}
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saving}
        okText={t('入队')}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleCreate} style={{ marginTop: 8 }}>
          <Form.Item
            name="to"
            label={t('被叫号码')}
            rules={[
              { required: true, message: t('请输入号码') },
              { pattern: /^[0-9*#+]{2,32}$/, message: t('2-32 位号码') },
            ]}
          >
            <Input placeholder={t('如 13800138000')} />
          </Form.Item>
          <Form.Item
            name="text"
            label={t('播报文本')}
            rules={[{ required: true, message: t('请输入话术') }, { max: 500, message: t('≤500 字') }]}
          >
            <Input.TextArea rows={4} maxLength={500} showCount placeholder={t('接通后播放的内容')} />
          </Form.Item>
          <Form.Item name="gateway" label={t('指定线路（可选）')}>
            <Input placeholder={t('空则按 LCR 首选出局网关')} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
