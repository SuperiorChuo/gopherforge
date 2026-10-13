import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Input, InputNumber, Form, Badge, Modal, Switch, Tooltip, Alert,
  Grid,
} from 'antd'
import { message } from '@/utils/feedback'
import { useVisibilityInterval } from '@/hooks/useVisibilityInterval'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined, VideoCameraOutlined,
  PhoneOutlined, LockOutlined, UnlockOutlined, AudioOutlined, AudioMutedOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  listCcConferences, createCcConference, updateCcConference, deleteCcConference,
  ccConferenceAction,
  type CcConference, type CcConferenceMember, type CcConferencePayload,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

export default function CcConferencePage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [modal, setModal] = useState(false)
  const [editing, setEditing] = useState<CcConference | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<CcConferencePayload>()
  const silentRef = useRef(false)

  const fetcher = useCallback(async () => {
    const res = await listCcConferences()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取会议室失败')), [t]),
  })

  // 5s 静默轮询在会人数/录音/成员静音态；后台标签页暂停（immediate=false：
  // 挂载首拉由 useTableQuery 负责）
  useVisibilityInterval(() => {
    if (silentRef.current) return
    silentRef.current = true
    void reload({ silent: true }).finally(() => { silentRef.current = false })
  }, 5000, false)

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ max_members: 0, auto_record: false })
    setModal(true)
  }

  const openEdit = (r: CcConference) => {
    setEditing(r)
    form.setFieldsValue({
      name: r.name,
      number: r.number,
      pin: r.pin || undefined,
      max_members: r.max_members,
      auto_record: r.auto_record,
      description: r.description || undefined,
    })
    setModal(true)
  }

  const handleSave = async (values: CcConferencePayload) => {
    setSaving(true)
    try {
      if (editing) {
        await updateCcConference(editing.name, values)
        message.success(t('会议室 {{name}} 已更新', { name: editing.name }))
      } else {
        await createCcConference(values)
        message.success(t('会议室已创建，拨 {{number}} 进会', { number: values.number }))
      }
      setModal(false)
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('保存会议室失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteCcConference(name)
      message.success(t('会议室 {{name}} 已删除', { name }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除会议室失败'))
    }
  }

  const runAction = async (
    name: string,
    action: Parameters<typeof ccConferenceAction>[1],
    memberId?: string,
    okMsg?: string,
  ) => {
    try {
      await ccConferenceAction(name, action, memberId)
      if (okMsg) message.success(okMsg)
      setTimeout(() => { void reload({ silent: true }) }, 400)
    } catch (e) {
      message.error((e as Error)?.message || t('操作失败'))
    }
  }

  const memberColumns = (room: CcConference): ColumnsType<CcConferenceMember> => [
    {
      title: t('成员 ID'),
      dataIndex: 'member_id',
      width: 100,
      responsive: ['lg'],
      render: (v: string) => <span className="cell-mono">{v}</span>,
    },
    {
      title: t('主叫'),
      key: 'caller',
      render: (_, m) => (
        <span>
          {m.caller_id_name && m.caller_id_name !== m.caller_id_number
            ? <span>{m.caller_id_name} <span className="cell-dim">({m.caller_id_number})</span></span>
            : <span className="cell-mono">{m.caller_id_number || '—'}</span>}
        </span>
      ),
    },
    {
      title: t('静音态'),
      dataIndex: 'muted',
      width: 100,
      responsive: ['sm'],
      render: (v: boolean) =>
        v ? <Tag color="orange" variant="filled">{t('已静音')}</Tag> : <Tag color="success" variant="filled">{t('正常')}</Tag>,
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      render: (_, m) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            {
              key: 'mute',
              label: m.muted ? t('取消静音') : t('静音'),
              icon: m.muted ? <AudioOutlined /> : <AudioMutedOutlined />,
              onClick: () => runAction(room.name, m.muted ? 'unmute' : 'mute', m.member_id, m.muted ? t('已取消静音') : t('已静音')),
            },
            {
              key: 'kick',
              label: t('踢出'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('把成员 {{id}} 踢出会议？', { id: m.member_id }),
              onClick: () => runAction(room.name, 'kick', m.member_id, t('已踢出')),
            },
          ]}
        />
      ),
    },
  ]

  const columns: ColumnsType<CcConference> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 160,
      render: (v: string, r) => (
        <Space size={6}>
          <VideoCameraOutlined />
          <span className="cell-mono">{v}</span>
          {r.dynamic && <Tag color="purple" variant="filled">{t('动态')}</Tag>}
        </Space>
      ),
    },
    {
      title: t('接入号'),
      dataIndex: 'number',
      width: 100,
      responsive: ['sm'],
      render: (v: string) =>
        v ? <Tag icon={<PhoneOutlined />} color="geekblue" variant="filled">{v}</Tag> : <span className="cell-dim">—</span>,
    },
    {
      title: 'PIN',
      dataIndex: 'pin',
      width: 80,
      responsive: ['lg'],
      render: (v: string) => (v ? <Tooltip title={t('入会需密码')}><LockOutlined style={{ color: 'var(--c-warning)' }} /></Tooltip> : <span className="cell-dim">{t('无')}</span>),
    },
    {
      title: t('上限'),
      dataIndex: 'max_members',
      width: 80,
      responsive: ['md'],
      render: (v: number) => (v > 0 ? v : <span className="cell-dim">{t('不限')}</span>),
    },
    {
      title: t('自动录音'),
      dataIndex: 'auto_record',
      width: 90,
      responsive: ['lg'],
      render: (v: boolean) => (v ? <Tag color="cyan" variant="filled">{t('开')}</Tag> : <span className="cell-dim">{t('关')}</span>),
    },
    {
      title: t('实时状态'),
      key: 'live',
      width: 180,
      responsive: ['sm'],
      render: (_, r) =>
        r.active
          ? (
            <Space size={8}>
              <Badge status="processing" text={t('进行中 · {{n}} 人', { n: r.member_count })} />
              {r.recording && <Badge status="error" text={t('录音中')} />}
              {r.locked && <Tag color="red" variant="filled">{t('已锁定')}</Tag>}
            </Space>
          )
          : <span className="cell-dim">{t('空闲')}</span>,
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 148,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, r) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: r.name })}
          actions={[
            {
              key: 'lock',
              label: r.locked ? t('解锁') : t('锁定'),
              icon: r.locked ? <UnlockOutlined /> : <LockOutlined />,
              disabled: !r.locked && !r.active,
              onClick: () => runAction(r.name, r.locked ? 'unlock' : 'lock', undefined, r.locked ? t('已解锁') : t('已锁定')),
            },
            {
              key: 'record',
              label: r.recording ? t('停录') : t('开录'),
              icon: r.recording ? <AudioMutedOutlined /> : <AudioOutlined />,
              disabled: !r.recording && !r.active,
              onClick: () => runAction(r.name, r.recording ? 'record_stop' : 'record_start', undefined, r.recording ? t('已停止录音') : t('已开始录音')),
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: !r.dynamic,
              onClick: () => openEdit(r),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: !r.dynamic,
              confirm: t('确认删除会议室 {{name}}？进行中的会议会被解散', { name: r.name }),
              onClick: () => { void handleDelete(r.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-conference-page"
      toolbar={
        <TableToolbar
          title="会议室"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建会议室')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
            </Space>
          }
        />}
    >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message={t('拨接入号进会，39xx 随拨随建')}
          description={t('固定会议室拨其 3xxx 接入号入会（39xx 段保留给动态会议）；直接拨 39xx（如 3901）即可随拨随建临时会议室，无需预先配置。')}
        />
        <Table
          rowKey="name"
          scroll={{ x: 800 }}
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={false}
          expandable={{
            expandedRowRender: (r) => (
              <Table
                rowKey="member_id"
                scroll={{ x: 800 }}
                size="small"
                columns={memberColumns(r)}
                dataSource={r.members}
                pagination={false}
                locale={{ emptyText: <GlassEmpty text="会议室内暂无成员" compact /> }}
              />
            ),
            rowExpandable: (r) => r.active,
          }}
          locale={{ emptyText: <GlassEmpty text="暂无会议室（新建后拨接入号进会，或直接拨 39xx 随拨随建）" compact /> }}
        />

      <Modal
        title={editing ? t('编辑会议室：{{name}}', { name: editing.name }) : t('新建会议室')}
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
              label={t('会议室名')}
              style={{ flex: 1 }}
              rules={[
                { required: true, message: t('请输入会议室名') },
                { pattern: /^[a-z0-9][a-z0-9_-]{1,31}$/, message: t('2-32 位小写字母/数字/_-') },
              ]}
            >
              <Input placeholder={t('如 board')} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="number"
              label={t('接入号')}
              style={{ flex: 1 }}
              tooltip={t('3xxx 四位数字；39xx 段保留给动态会议，不能占用')}
              rules={[
                { required: true, message: t('请输入接入号') },
                { pattern: /^3[0-9]{3}$/, message: t('需 3xxx 四位数字') },
                {
                  validator: (_, v) =>
                    v && /^39/.test(v)
                      ? Promise.reject(new Error(t('39xx 是动态会议段，请换个号')))
                      : Promise.resolve(),
                },
              ]}
            >
              <Input placeholder={t('如 3001')} prefix={<PhoneOutlined />} />
            </Form.Item>
          </Space>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="pin"
              label={t('入会密码 PIN')}
              style={{ flex: 1 }}
              tooltip={t('留空则拨号即入；填 4-8 位数字则入会需输密码')}
              rules={[{ pattern: /^[0-9]{4,8}$/, message: t('4-8 位数字') }]}
            >
              <Input placeholder={t('可空，如 1234')} />
            </Form.Item>
            <Form.Item name="max_members" label={t('人数上限')} tooltip={t('0=不限')} style={{ width: 140 }}>
              <InputNumber min={0} max={1000} style={{ width: '100%' }} />
            </Form.Item>
          </Space>
          <Form.Item name="auto_record" label={t('进会自动录音')} valuePropName="checked" tooltip={t('开启后有人进会即自动录音，落录音目录')}>
            <Switch />
          </Form.Item>
          <Form.Item
            name="description"
            label={t('备注')}
            rules={[{ max: 256, message: t('备注过长') }]}
          >
            <Input.TextArea rows={2} placeholder={t('选填，如「周会固定会议室」')} />
          </Form.Item>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
