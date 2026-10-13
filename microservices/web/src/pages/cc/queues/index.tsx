import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Input, InputNumber, Form, Badge,
  Popconfirm, Modal, Select, Switch, Dropdown, Collapse, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined, TeamOutlined,
  PhoneOutlined, UserAddOutlined, DownOutlined, ClockCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listCcQueues, createCcQueue, updateCcQueue, deleteCcQueue,
  upsertCcQueueAgent, removeCcQueueAgent, setCcAgentStatus,
  listMoh, listPrompts, CC_STRATEGIES, CC_NO_ANSWER_STATUSES,
  type CcQueue, type CcQueueAgent, type CcQueueMember, type CcQueuePayload,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

// 播报音下拉选项：提示音库 + 排队音乐合并（都可作 announce-sound）
type SoundOption = { path: string; name: string; group: string }

function agentStatusBadge(status: string, t: TFunction) {
  switch (status) {
    case 'Available':
      return <Badge status="success" text={t('已签入')} />
    case 'On Break':
      return <Badge status="warning" text={t('示忙')} />
    case 'Logged Out':
      return <Badge status="default" text={t('已签出')} />
    default:
      return <Badge status="default" text={status || t('未知')} />
  }
}

const strategyLabel = (v: string) =>
  CC_STRATEGIES.find((s) => s.value === v)?.label ?? v ?? '默认'

function formatWait(sec: number) {
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${m}m${s.toString().padStart(2, '0')}s`
}

export default function CcQueuesPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [soundOptions, setSoundOptions] = useState<SoundOption[]>([])
  const [queueModal, setQueueModal] = useState(false)
  const [editing, setEditing] = useState<CcQueue | null>(null)
  const [saving, setSaving] = useState(false)
  const [queueForm] = Form.useForm<CcQueuePayload>()
  // 挂坐席弹窗
  const [agentModal, setAgentModal] = useState<string | null>(null) // queue name
  const [agentForm] = Form.useForm<{
    ext: string; level: number; wrap_up_time: number
    reject_delay_time?: number; busy_delay_time?: number; no_answer_delay_time?: number
  }>()
  const [addingAgent, setAddingAgent] = useState(false)

  const fetcher = useCallback(async () => {
    const res = await listCcQueues()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取队列失败')), [t]),
  })

  useEffect(() => {
    // 播报音下拉：提示音库 + 排队音乐合并
    Promise.all([
      listPrompts().then((r) => r.list ?? []).catch(() => []),
      listMoh().then((r) => r.list ?? []).catch(() => []),
    ]).then(([prompts, moh]) => {
      setSoundOptions([
        ...prompts.map((p) => ({ path: p.path, name: p.name, group: t('提示音库') })),
        ...moh.map((m) => ({ path: m.path, name: m.name, group: t('排队音乐') })),
      ])
    })
  }, [t])

  const openCreate = () => {
    setEditing(null)
    queueForm.resetFields()
    queueForm.setFieldsValue({
      strategy: 'longest-idle-agent', max_wait_time: 120,
      announce_frequency: 30, agent_no_answer_status: '',
    })
    setQueueModal(true)
  }

  const openEdit = (q: CcQueue) => {
    setEditing(q)
    queueForm.setFieldsValue({
      name: q.name,
      access_number: q.access_number,
      strategy: q.strategy || 'longest-idle-agent',
      moh_sound: q.moh_sound || undefined,
      max_wait_time: q.max_wait_time,
      timeout_number: q.timeout_number || undefined,
      tier_rules_apply: q.tier_rules_apply,
      tier_rule_wait_sec: q.tier_rule_wait_sec || undefined,
      announce_sound: q.announce_sound || undefined,
      announce_frequency: q.announce_frequency || undefined,
      record_template: q.record_template,
      abandoned_resume_allowed: q.abandoned_resume_allowed,
      ring_progressively_delay: q.ring_progressively_delay || undefined,
      skip_agents_with_external_calls: q.skip_agents_with_external_calls,
      agent_no_answer_status: q.agent_no_answer_status || '',
    })
    setQueueModal(true)
  }

  const handleSaveQueue = async (values: CcQueuePayload) => {
    setSaving(true)
    try {
      if (editing) {
        await updateCcQueue(editing.name, values)
        message.success(t('队列 {{name}} 已更新', { name: editing.name }))
      } else {
        await createCcQueue(values)
        message.success(t('队列已创建，拨 {{number}} 进队', { number: values.access_number }))
      }
      setQueueModal(false)
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('保存队列失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDeleteQueue = async (name: string) => {
    try {
      await deleteCcQueue(name)
      message.success(t('队列 {{name}} 已删除', { name }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除队列失败'))
    }
  }

  const handleAddAgent = async (values: {
    ext: string; level: number; wrap_up_time: number
    reject_delay_time?: number; busy_delay_time?: number; no_answer_delay_time?: number
  }) => {
    if (!agentModal) return
    setAddingAgent(true)
    try {
      await upsertCcQueueAgent(agentModal, {
        ext: values.ext.trim(),
        level: values.level,
        wrap_up_time: values.wrap_up_time,
        reject_delay_time: values.reject_delay_time,
        busy_delay_time: values.busy_delay_time,
        no_answer_delay_time: values.no_answer_delay_time,
      })
      message.success(t('坐席 {{ext}} 已挂入 {{queue}}（初始为签出态）', { ext: values.ext, queue: agentModal }))
      setAgentModal(null)
      agentForm.resetFields()
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('挂入坐席失败'))
    } finally {
      setAddingAgent(false)
    }
  }

  const handleRemoveAgent = async (queue: string, ext: string) => {
    try {
      await removeCcQueueAgent(queue, ext)
      message.success(t('坐席 {{ext}} 已移出', { ext }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('移出失败'))
    }
  }

  const handleAgentStatus = async (ext: string, status: 'available' | 'on-break' | 'logged-out') => {
    try {
      await setCcAgentStatus(ext, status)
      message.success(t('坐席 {{ext}} 状态已更新', { ext }))
      setTimeout(() => { void reload() }, 500)
    } catch (e) {
      message.error((e as Error)?.message || t('切换状态失败'))
    }
  }

  const agentColumns = (queue: string): ColumnsType<CcQueueAgent> => [
    {
      title: t('坐席分机'),
      dataIndex: 'ext',
      width: 110,
      render: (v: string) => <span className="cell-mono">{v}</span>,
    },
    { title: t('签入状态'), dataIndex: 'status', width: 120, render: (v: string) => agentStatusBadge(v, t) },
    {
      title: t('通话态'),
      dataIndex: 'state',
      width: 130,
      render: (v: string) =>
        v === 'In a queue call'
          ? <Tag color="success" variant="filled">{t('队列通话中')}</Tag>
          : v === 'Receiving'
            ? <Tag color="processing" variant="filled">{t('振铃中')}</Tag>
            : <span className="cell-dim">{v || '—'}</span>,
    },
    { title: t('层级'), dataIndex: 'level', width: 70 },
    {
      title: t('话后处理'),
      dataIndex: 'wrap_up_time',
      width: 100,
      render: (v: number) => (v ? `${v}s` : t('默认')),
    },
    {
      title: t('操作'),
      key: 'action',
      render: (_, a) => (
        <Space size={8}>
          <Dropdown
            menu={{
              items: [
                { key: 'available', label: t('签入（开始接队列电话）') },
                { key: 'on-break', label: t('示忙（小休）') },
                { key: 'logged-out', label: t('签出') },
              ],
              onClick: ({ key }) => handleAgentStatus(a.ext, key as 'available' | 'on-break' | 'logged-out'),
            }}
          >
            <Button size="small">{t('签入/签出')} <DownOutlined /></Button>
          </Dropdown>
          <Popconfirm title={t('把 {{ext}} 移出队列？', { ext: a.ext })} onConfirm={() => handleRemoveAgent(queue, a.ext)}>
            <Button danger size="small" icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const memberColumns: ColumnsType<CcQueueMember> = [
    {
      title: t('位次'),
      dataIndex: 'position',
      width: 70,
      render: (v: number) => <Tag color={v === 1 ? 'volcano' : 'default'} variant="filled">#{v}</Tag>,
    },
    {
      title: t('来电号码'),
      dataIndex: 'cid_number',
      render: (v: string) => <span className="cell-mono">{v || t('匿名')}</span>,
    },
    {
      title: t('已等待'),
      dataIndex: 'wait_sec',
      width: 120,
      render: (v: number) => (
        <Space size={4}><ClockCircleOutlined /><span>{formatWait(v)}</span></Space>
      ),
    },
  ]

  const columns: ColumnsType<CcQueue> = [
    {
      title: t('队列'),
      dataIndex: 'name',
      width: 150,
      render: (v: string) => (
        <Space size={6}><TeamOutlined /><span className="cell-mono">{v}</span></Space>
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
      title: t('派单策略'),
      dataIndex: 'strategy',
      width: 160,
      responsive: ['md'],
      render: (v: string) => <Tag variant="filled">{t(strategyLabel(v))}</Tag>,
    },
    {
      title: t('排队中'),
      dataIndex: 'members',
      width: 90,
      responsive: ['sm'],
      render: (v: number) =>
        v > 0 ? <Badge count={v} color="volcano" /> : <span className="cell-dim">0</span>,
    },
    {
      title: t('坐席'),
      key: 'agents',
      width: 120,
      responsive: ['md'],
      render: (_, q) => {
        const online = q.agents.filter((a) => a.status === 'Available').length
        return <span>{online}/{q.agents.length} <span className="cell-dim">{t('签入')}</span></span>
      },
    },
    {
      title: t('超时'),
      key: 'timeout',
      width: 150,
      responsive: ['lg'],
      render: (_, q) =>
        q.max_wait_time
          ? <span className="cell-dim" style={{ fontSize: 12 }}>{q.max_wait_time}s → {q.timeout_number || t('提示后挂断')}</span>
          : <span className="cell-dim">{t('不限')}</span>,
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 120,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, q) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: q.name })}
          actions={[
            { key: 'agents', label: t('挂坐席'), icon: <UserAddOutlined />, onClick: () => setAgentModal(q.name) },
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(q) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除队列 {{name}}？', { name: q.name }),
              onClick: () => { void handleDeleteQueue(q.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-queues-page"
      toolbar={
        <TableToolbar
          title="呼叫队列（ACD）"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建队列')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Table
          rowKey="name"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          scroll={{ x: 1700 }}
          expandable={{
            expandedRowRender: (q) => (
              <Space direction="vertical" size={16} style={{ display: 'flex' }}>
                <div>
                  <div className="cell-dim" style={{ fontSize: 13, marginBottom: 8 }}>{t('坐席')}</div>
                  <Table
                    rowKey="ext"
                    size="small"
                    columns={agentColumns(q.name)}
                    dataSource={q.agents}
                    pagination={false}
                    locale={{ emptyText: <GlassEmpty text="还没挂坐席，点上方「挂坐席」" compact /> }}
                  />
                </div>
                <div>
                  <div className="cell-dim" style={{ fontSize: 13, marginBottom: 8 }}>
                    {t('排队中的客户')}{q.members_detail?.length ? t('（{{n}}）', { n: q.members_detail.length }) : ''}
                  </div>
                  <Table
                    rowKey={(m) => `${m.position}-${m.cid_number}`}
                    size="small"
                    columns={memberColumns}
                    dataSource={q.members_detail ?? []}
                    pagination={false}
                    locale={{ emptyText: <GlassEmpty text="当前无人排队" compact /> }}
                  />
                </div>
              </Space>
            ),
          }}
          locale={{ emptyText: <GlassEmpty text="暂无队列（新建后拨接入号排队，坐席签入即派单）" compact /> }}
        />

      <Modal
        title={editing ? t('编辑队列：{{name}}', { name: editing.name }) : t('新建队列')}
        open={queueModal}
        onCancel={() => setQueueModal(false)}
        onOk={() => queueForm.submit()}
        confirmLoading={saving}
        okText={editing ? t('保存并重载') : t('创建')}
        destroyOnHidden
        width={560}
      >
        <Form form={queueForm} layout="vertical" onFinish={handleSaveQueue} style={{ marginTop: 8 }}>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="name"
              label={t('队列名')}
              style={{ flex: 1 }}
              rules={[
                { required: true, message: t('请输入队列名') },
                { pattern: /^[a-z0-9][a-z0-9_-]{1,31}$/, message: t('2-32 位小写字母/数字/_-') },
              ]}
            >
              <Input placeholder={t('如 support')} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="access_number"
              label={t('接入号')}
              style={{ flex: 1 }}
              rules={[{ required: true, message: t('请输入接入号') }, { pattern: /^[0-9]{4,6}$/, message: t('4-6 位数字') }]}
            >
              <Input placeholder={t('如 7000')} prefix={<PhoneOutlined />} />
            </Form.Item>
          </Space>
          <Form.Item name="strategy" label={t('派单策略')}>
            <Select options={CC_STRATEGIES} />
          </Form.Item>
          <Form.Item name="moh_sound" label={t('排队等待音乐')} tooltip={t('默认循环播放「排队音乐」页上传的全部曲目；也可指定单曲')}>
            <Select allowClear placeholder={t('默认（moh 流：全部上传曲目循环）')}>
              {soundOptions.map((f) => (
                <Select.Option key={f.path} value={f.path}>{f.name}<span className="cell-dim"> · {t(f.group)}</span></Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item name="max_wait_time" label={t('最大等待(秒)')} tooltip={t('0=不限；超时转下面的号码')} style={{ width: 140 }}>
              <InputNumber min={0} max={7200} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item
              name="timeout_number"
              label={t('超时转接号码')}
              tooltip={t('超时转 IVR/分机/语音提示；留空=播放无人接听提示后挂断')}
              style={{ flex: 1 }}
              rules={[{ pattern: /^[0-9]{0,16}$/, message: t('数字') }]}
            >
              <Input placeholder={t('如 6000（转 IVR）')} />
            </Form.Item>
          </Space>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="tier_rules_apply"
              label={t('启用层级派单')}
              valuePropName="checked"
              tooltip={t('开启后先派 1 级坐席，等待若干秒才放开下一层级（VIP 组优先）')}
            >
              <Switch />
            </Form.Item>
            <Form.Item name="tier_rule_wait_sec" label={t('层级放开等待(秒)')} style={{ width: 160 }}>
              <InputNumber min={0} max={600} style={{ width: '100%' }} />
            </Form.Item>
          </Space>

          <div className="cell-dim" style={{ fontSize: 13, margin: '4px 0 8px' }}>{t('排队播报')}</div>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="announce_sound"
              label={t('播报音')}
              style={{ flex: 1 }}
              tooltip={t('排队过程中周期播放的提示音（如前方等待人数录音/TTS）；数据源为提示音库 + 排队音乐')}
            >
              <Select allowClear placeholder={t('不播报')}>
                {soundOptions.map((f) => (
                  <Select.Option key={f.path} value={f.path}>{f.name}<span className="cell-dim"> · {t(f.group)}</span></Select.Option>
                ))}
              </Select>
            </Form.Item>
            <Form.Item
              name="announce_frequency"
              label={t('播报间隔(秒)')}
              tooltip={t('每隔多少秒播一次；0 或空=不播报')}
              style={{ width: 150 }}
            >
              <InputNumber min={0} max={300} style={{ width: '100%' }} />
            </Form.Item>
          </Space>

          <Collapse
            ghost
            items={[{
              key: 'advanced',
              label: t('高级（队列深水区参数）'),
              children: (
                <>
                  <Space size={16} style={{ display: 'flex' }} align="start">
                    <Form.Item
                      name="record_template"
                      label={t('队列自动录音')}
                      valuePropName="checked"
                      tooltip={t('进队即录到 recordings 卷（队列级，独立于分机侧录音）')}
                    >
                      <Switch />
                    </Form.Item>
                    <Form.Item
                      name="abandoned_resume_allowed"
                      label={t('掉线回队保位')}
                      valuePropName="checked"
                      tooltip={t('客户掉线后短时内重连，恢复原排队位置')}
                    >
                      <Switch />
                    </Form.Item>
                    <Form.Item
                      name="skip_agents_with_external_calls"
                      label={t('跳过外呼中坐席')}
                      valuePropName="checked"
                      tooltip={t('正在外呼的坐席不参与本队列派单')}
                    >
                      <Switch />
                    </Form.Item>
                  </Space>
                  <Space size={16} style={{ display: 'flex' }} align="start">
                    <Form.Item
                      name="ring_progressively_delay"
                      label={t('渐进振铃间隔(秒)')}
                      tooltip={t('ring-progressively 策略下，逐个放开下一个坐席的间隔')}
                      style={{ width: 180 }}
                    >
                      <InputNumber min={0} max={300} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item
                      name="agent_no_answer_status"
                      label={t('坐席无应答后')}
                      tooltip={t('坐席响铃未接后自动置为该状态，避免继续派单空振')}
                      style={{ flex: 1 }}
                    >
                      <Select options={CC_NO_ANSWER_STATUSES} />
                    </Form.Item>
                  </Space>
                </>
              ),
            }]}
          />
        </Form>
      </Modal>

      <Modal
        title={t('挂坐席到队列：{{queue}}', { queue: agentModal ?? '' })}
        open={!!agentModal}
        onCancel={() => setAgentModal(null)}
        onOk={() => agentForm.submit()}
        confirmLoading={addingAgent}
        okText={t('挂入')}
        destroyOnHidden
        width={520}
      >
        <Form
          form={agentForm}
          layout="vertical"
          onFinish={handleAddAgent}
          initialValues={{ level: 1, wrap_up_time: 10 }}
          style={{ marginTop: 8 }}
        >
          <Form.Item
            name="ext"
            label={t('坐席分机')}
            rules={[{ required: true, message: t('请输入分机号') }, { pattern: /^[0-9]{2,16}$/, message: t('数字') }]}
          >
            <Input placeholder={t('如 1000')} />
          </Form.Item>
          <Space size={16} style={{ display: 'flex' }}>
            <Form.Item name="level" label={t('层级（小的先派）')} style={{ flex: 1 }}>
              <InputNumber min={1} max={9} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="wrap_up_time" label={t('话后处理(秒)')} style={{ flex: 1 }}>
              <InputNumber min={0} max={600} style={{ width: '100%' }} />
            </Form.Item>
          </Space>
          <div className="cell-dim" style={{ fontSize: 13, margin: '0 0 8px' }}>{t('冷却时长（可选，0/留空=用队列默认）')}</div>
          <Space size={16} style={{ display: 'flex' }}>
            <Form.Item name="reject_delay_time" label={t('拒接冷却(秒)')} style={{ flex: 1 }}>
              <InputNumber min={0} max={600} style={{ width: '100%' }} placeholder={t('默认 10')} />
            </Form.Item>
            <Form.Item name="busy_delay_time" label={t('示忙冷却(秒)')} style={{ flex: 1 }}>
              <InputNumber min={0} max={600} style={{ width: '100%' }} placeholder={t('默认 60')} />
            </Form.Item>
            <Form.Item name="no_answer_delay_time" label={t('无应答冷却(秒)')} style={{ flex: 1 }}>
              <InputNumber min={0} max={600} style={{ width: '100%' }} placeholder={t('默认 10')} />
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
