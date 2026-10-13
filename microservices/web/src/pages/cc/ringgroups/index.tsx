import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Input, InputNumber, Form, Modal, Radio, Select, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined,
  BellOutlined, PhoneOutlined, TeamOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listCcRingGroups, createCcRingGroup, updateCcRingGroup, deleteCcRingGroup,
  CC_RING_STRATEGIES,
  type CcRingGroup, type CcRingGroupPayload, type CcRingStrategy,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

const strategyLabel = (v: CcRingStrategy) =>
  CC_RING_STRATEGIES.find((s) => s.value === v)?.label ?? v

type FormValues = {
  name: string
  access_number: string
  strategy: CcRingStrategy
  exts: string[] // Select mode="tags"
  ring_timeout: number
  failover_target?: string
}

export default function CcRingGroupsPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [modal, setModal] = useState(false)
  const [editing, setEditing] = useState<CcRingGroup | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<FormValues>()

  const fetcher = useCallback(async () => {
    const res = await listCcRingGroups()
    return { list: res.list ?? [], total: res.list?.length ?? 0 }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取响铃组失败')), [t]),
  })

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ strategy: 'simultaneous', ring_timeout: 20, exts: [] })
    setModal(true)
  }

  const openEdit = (g: CcRingGroup) => {
    setEditing(g)
    form.setFieldsValue({
      name: g.name,
      access_number: g.access_number,
      strategy: g.strategy,
      exts: g.exts.split(',').map((e) => e.trim()).filter(Boolean),
      ring_timeout: g.ring_timeout,
      failover_target: g.failover_target || undefined,
    })
    setModal(true)
  }

  const handleSave = async (values: FormValues) => {
    const exts = (values.exts || []).map((e) => e.trim()).filter(Boolean)
    if (exts.length === 0) {
      message.error(t('请至少添加一个成员分机'))
      return
    }
    if (!exts.every((e) => /^\d{2,16}$/.test(e))) {
      message.error(t('分机号必须是 2-16 位数字'))
      return
    }
    const payload: CcRingGroupPayload = {
      name: values.name.trim(),
      access_number: values.access_number.trim(),
      strategy: values.strategy,
      exts: exts.join(','),
      ring_timeout: values.ring_timeout,
      failover_target: values.failover_target?.trim() || undefined,
    }
    setSaving(true)
    try {
      if (editing) {
        await updateCcRingGroup(editing.name, payload)
        message.success(t('响铃组 {{name}} 已更新', { name: editing.name }))
      } else {
        await createCcRingGroup(payload)
        message.success(t('响铃组已创建，拨 {{number}} 触发', { number: payload.access_number }))
      }
      setModal(false)
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteCcRingGroup(name)
      message.success(t('响铃组 {{name}} 已删除', { name }))
      reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除失败'))
    }
  }

  const columns: ColumnsType<CcRingGroup> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 150,
      render: (v: string) => (
        <Space size={6}><BellOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('接入号'),
      dataIndex: 'access_number',
      width: 100,
      responsive: ['sm'],
      render: (v: string) => <Tag icon={<PhoneOutlined />} color="geekblue" variant="filled">{v}</Tag>,
    },
    {
      title: t('策略'),
      dataIndex: 'strategy',
      width: 100,
      responsive: ['md'],
      render: (v: CcRingStrategy) => (
        <Tag color={v === 'simultaneous' ? 'blue' : 'purple'} variant="filled">{t(strategyLabel(v))}</Tag>
      ),
    },
    {
      title: t('成员'),
      dataIndex: 'exts',
      responsive: ['sm'],
      render: (v: string) => {
        const exts = v.split(',').filter(Boolean)
        return (
          <Space size={4} wrap>
            <TeamOutlined />
            {exts.map((e) => <Tag key={e} variant="filled">{e}</Tag>)}
          </Space>
        )
      },
    },
    {
      title: t('振铃超时'),
      dataIndex: 'ring_timeout',
      width: 100,
      responsive: ['lg'],
      render: (v: number) => <span className="cell-dim">{v}s</span>,
    },
    {
      title: t('无人接转'),
      dataIndex: 'failover_target',
      width: 120,
      responsive: ['md'],
      render: (v: string) =>
        v ? <span className="cell-mono">{v}</span> : <span className="cell-dim">{t('挂断')}</span>,
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, g) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: g.name })}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(g) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除响铃组 {{name}}？', { name: g.name }),
              onClick: () => { void handleDelete(g.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-ringgroups-page"
      toolbar={
        <TableToolbar
          title="响铃组"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建响铃组')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Table
          rowKey="name"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="暂无响铃组（拨接入号同振/顺振一组分机，全都没接转兜底）" compact /> }}
        />

      <Modal
        title={editing ? t('编辑响铃组：{{name}}', { name: editing.name }) : t('新建响铃组')}
        open={modal}
        onCancel={() => setModal(false)}
        onOk={() => form.submit()}
        confirmLoading={saving}
        okText={editing ? t('保存并重载') : t('创建')}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" onFinish={handleSave} style={{ marginTop: 8 }}>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="name"
              label={t('名称')}
              style={{ flex: 1 }}
              rules={[
                { required: true, message: t('请输入名称') },
                { pattern: /^[a-z0-9][a-z0-9_-]{1,31}$/, message: t('2-32 位小写字母/数字/_-') },
              ]}
            >
              <Input placeholder={t('如 sales')} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="access_number"
              label={t('接入号')}
              style={{ flex: 1 }}
              rules={[{ required: true, message: t('请输入接入号') }, { pattern: /^[0-9]{3,6}$/, message: t('3-6 位数字') }]}
            >
              <Input placeholder={t('如 8000')} prefix={<PhoneOutlined />} />
            </Form.Item>
          </Space>

          <Form.Item name="strategy" label={t('振铃策略')}>
            <Radio.Group>
              <Radio.Button value="simultaneous">{t('同振（一起响）')}</Radio.Button>
              <Radio.Button value="sequential">{t('顺振（依次响）')}</Radio.Button>
            </Radio.Group>
          </Form.Item>

          <Form.Item
            name="exts"
            label={t('成员分机')}
            tooltip={t('输入分机号后回车添加；同振时全部一起响，顺振时按顺序依次响')}
            rules={[{ required: true, message: t('请添加至少一个成员分机') }]}
          >
            <Select mode="tags" placeholder={t('输入分机号回车，如 1000')} open={false} tokenSeparators={[',']} />
          </Form.Item>

          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="ring_timeout"
              label={t('振铃超时(秒)')}
              style={{ width: 160 }}
              tooltip={t('整组振铃多久没接就走无人接转/挂断')}
              rules={[{ required: true, message: t('请填写超时') }]}
            >
              <InputNumber min={5} max={600} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item
              name="failover_target"
              label={t('无人接转')}
              style={{ flex: 1 }}
              tooltip={t('全都没接时转到的目标号码；留空=直接挂断')}
              rules={[{ pattern: /^[0-9*#+]{0,32}$/, message: t('号码格式不对') }]}
            >
              <Input placeholder={t('如 6000（转 IVR）；留空=挂断')} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
