import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Card, Input, InputNumber, Form, Modal, Select, Tree, Empty, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined, ForkOutlined,
  PhoneOutlined, ApartmentOutlined, RollbackOutlined, StopOutlined, RetweetOutlined, MenuOutlined,
  NumberOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DataNode } from 'antd/es/tree'
import {
  listFsIVRs, createFsIVR, updateFsIVR, deleteFsIVR, listPrompts,
  type FsIVR, type FsIVREntry, type FsIVRPayload, type PromptFile,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#'] as const

type ActionKind = FsIVREntry['action']

const ACTION_META: Record<ActionKind, { label: string; color: string; icon: React.ReactNode }> = {
  transfer: { label: '转接', color: 'var(--c-primary)', icon: <PhoneOutlined /> },
  submenu: { label: '子菜单', color: 'var(--c-purple)', icon: <ApartmentOutlined /> },
  collect: { label: '收号', color: 'var(--c-cyan)', icon: <NumberOutlined /> },
  replay: { label: '重播', color: 'var(--c-cyan)', icon: <RetweetOutlined /> },
  back: { label: '返回上级', color: 'var(--c-orange)', icon: <RollbackOutlined /> },
  hangup: { label: '挂断', color: 'var(--c-error-strong)', icon: <StopOutlined /> },
}

// 收号参数：target 编码为 "min:max:timeout:prompt:next"
type CollectParams = { min: number; max: number; timeout: number; prompt: string; next: string }

const DEFAULT_COLLECT: CollectParams = { min: 1, max: 6, timeout: 8000, prompt: '', next: '' }

function parseCollect(target?: string): CollectParams {
  if (!target) return { ...DEFAULT_COLLECT }
  const parts = target.split(':')
  if (parts.length < 5) return { ...DEFAULT_COLLECT }
  // next 是最后一段；prompt 是中间（理论不含冒号），其余按序取
  return {
    min: Number(parts[0]) || DEFAULT_COLLECT.min,
    max: Number(parts[1]) || DEFAULT_COLLECT.max,
    timeout: Number(parts[2]) || DEFAULT_COLLECT.timeout,
    prompt: parts[3] || '',
    next: parts[4] || '',
  }
}

function formatCollect(p: CollectParams): string {
  return `${p.min}:${p.max}:${p.timeout}:${p.prompt}:${p.next}`
}

function entrySummary(e: FsIVREntry, t: TFunction) {
  const meta = ACTION_META[e.action]
  if (e.action === 'transfer') return t('转 {{target}}', { target: e.target })
  if (e.action === 'submenu') return t('→ {{target}}', { target: e.target })
  if (e.action === 'collect') {
    const p = parseCollect(e.target)
    return t('收号转 {{next}}', { next: p.next || '?' })
  }
  return t(meta.label)
}

type KeyMap = Partial<Record<string, FsIVREntry>>

export default function CcIVRPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [modalOpen, setModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<FsIVR | null>(null)
  const [form] = Form.useForm<{ name: string; access_number: string; greeting?: string; timeout_ms?: number }>()
  // 拨号盘键位配置（可视化编辑核心状态）
  const [keymap, setKeymap] = useState<KeyMap>({})
  const [selectedKey, setSelectedKey] = useState<string>('1')
  // 收号动作的提示音下拉数据源
  const [prompts, setPrompts] = useState<PromptFile[]>([])

  const fetcher = useCallback(async () => {
    const res = await listFsIVRs()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取 IVR 列表失败')), [t]),
  })
  // 提示音库拉一次（收号动作选提示音用），失败不阻塞
  useEffect(() => {
    listPrompts().then((res) => setPrompts(res.list ?? [])).catch(() => setPrompts([]))
  }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ timeout_ms: 8000 })
    setKeymap({ '1': { digits: '1', action: 'transfer', target: '1000' } })
    setSelectedKey('1')
    setModalOpen(true)
  }

  const openEdit = (row: FsIVR) => {
    setEditing(row)
    form.setFieldsValue({
      name: row.name,
      access_number: row.access_number,
      greeting: row.greeting || undefined,
      timeout_ms: row.timeout_ms,
    })
    const km: KeyMap = {}
    for (const e of row.entry_list ?? []) km[e.digits] = e
    setKeymap(km)
    setSelectedKey(row.entry_list?.[0]?.digits ?? '1')
    setModalOpen(true)
  }

  const setEntry = (digits: string, patch: Partial<FsIVREntry> | null) => {
    setKeymap((prev) => {
      const next = { ...prev }
      if (patch === null) {
        delete next[digits]
      } else {
        const cur = next[digits] ?? { digits, action: 'transfer' as ActionKind }
        next[digits] = { ...cur, ...patch, digits }
      }
      return next
    })
  }

  const handleSave = async () => {
    let base: { name: string; access_number: string; greeting?: string; timeout_ms?: number }
    try {
      base = await form.validateFields()
    } catch {
      return
    }
    const entries = KEYS.filter((k) => keymap[k]).map((k) => {
      const e = keymap[k]!
      let target: string | undefined
      if (e.action === 'transfer' || e.action === 'submenu') target = e.target?.trim()
      else if (e.action === 'collect') target = e.target // 已是编码串
      return { digits: k, action: e.action, target }
    })
    if (!entries.length) {
      message.error(t('至少给一个按键配置动作'))
      return
    }
    for (const e of entries) {
      if ((e.action === 'transfer' || e.action === 'submenu') && !e.target) {
        message.error(t('按键 {{digits}} 缺少{{kind}}', { digits: e.digits, kind: e.action === 'transfer' ? t('目标号码') : t('子菜单') }))
        return
      }
      if (e.action === 'collect') {
        const p = parseCollect(e.target)
        if (!p.next) {
          message.error(t('按键 {{digits}} 收号缺少完成后转接号码', { digits: e.digits }))
          return
        }
        if (p.min < 1 || p.max < 1 || p.min > p.max) {
          message.error(t('按键 {{digits}} 收号位数需 min≤max 且≥1', { digits: e.digits }))
          return
        }
      }
    }
    const payload: FsIVRPayload = {
      name: base.name.trim(),
      access_number: base.access_number.trim(),
      greeting: base.greeting?.trim(),
      timeout_ms: base.timeout_ms,
      entries,
    }
    setSaving(true)
    try {
      if (editing) {
        await updateFsIVR(editing.name, payload)
        message.success(t('IVR {{name}} 已更新并热加载', { name: editing.name }))
      } else {
        await createFsIVR(payload)
        message.success(t('IVR 已创建，拨 {{number}} 即可进入', { number: payload.access_number }))
      }
      setModalOpen(false)
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('保存 IVR 失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteFsIVR(name)
      message.success(t('IVR {{name}} 已删除', { name }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除 IVR 失败'))
    }
  }

  // ---- 层级树总览：根 = 未被任何菜单引用为子菜单的菜单 ----
  const treeData = useMemo<DataNode[]>(() => {
    const byName = new Map(list.map((m) => [m.name, m]))
    const referenced = new Set<string>()
    for (const m of list) {
      for (const e of m.entry_list ?? []) {
        if (e.action === 'submenu' && e.target) referenced.add(e.target)
      }
    }
    const menuNode = (m: FsIVR, path: Set<string>): DataNode => ({
      key: [...path, m.name].join('/'),
      title: (
        <Space size={6}>
          <ForkOutlined />
          <span className="cell-mono">{m.name}</span>
          <Tag color="geekblue" variant="filled" style={{ marginInlineEnd: 0 }}>{t('拨 {{num}}', { num: m.access_number })}</Tag>
        </Space>
      ),
      children: (m.entry_list ?? []).map((e) => {
        const meta = ACTION_META[e.action]
        const keyBase = [...path, m.name, e.digits].join('/')
        if (e.action === 'submenu' && e.target && byName.has(e.target) && !path.has(e.target)) {
          const sub = byName.get(e.target)!
          return {
            key: keyBase,
            title: <span>{t('按')} <b>{e.digits}</b> {t('→ 子菜单')}</span>,
            children: [menuNode(sub, new Set([...path, m.name]))],
          }
        }
        return {
          key: keyBase,
          title: (
            <span>
              {t('按')} <b>{e.digits}</b> {t('→')} <span style={{ color: meta.color }}>{entrySummary(e, t)}</span>
            </span>
          ),
        }
      }),
    })
    const roots = list.filter((m) => !referenced.has(m.name))
    return (roots.length ? roots : list).map((m) => menuNode(m, new Set()))
  }, [list, t])

  const columns: ColumnsType<FsIVR> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 150,
      render: (v: string) => (
        <Space size={6}><ForkOutlined /><span className="cell-mono">{v}</span></Space>
      ),
    },
    {
      title: t('接入号'),
      dataIndex: 'access_number',
      width: 100,
      render: (v: string) => <Tag icon={<PhoneOutlined />} color="geekblue" variant="filled">{v}</Tag>,
    },
    {
      title: t('按键映射'),
      key: 'entries',
      render: (_, row) => (
        <Space size={4} wrap>
          {(row.entry_list ?? []).map((e) => (
            <Tag key={e.digits} variant="filled" style={{ borderColor: ACTION_META[e.action].color }}>
              {e.digits} {entrySummary(e, t)}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: compactActions ? 48 : 88,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, row) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: row.name })}
          actions={[
            { key: 'edit', label: t('编辑'), icon: <EditOutlined />, onClick: () => openEdit(row) },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('确认删除 IVR {{name}}？', { name: row.name }),
              onClick: () => { void handleDelete(row.name) },
            },
          ]}
        />
      ),
    },
  ]

  const selected = keymap[selectedKey]
  const otherMenus = list.filter((m) => m.name !== editing?.name)

  return (
    <ListPageShell
      className="cc-ivr-page"
      toolbar={
        <TableToolbar
          title="IVR 语音导航"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建 IVR')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }}>{t('刷新')}</Button>
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
          locale={{ emptyText: <GlassEmpty text="暂无 IVR（新建后拨接入号即可体验按键导航）" compact /> }}
        />

      <Card className="list-main-card" bordered={false}>
        <TableToolbar title="导航层级总览" total={list.length} />
        {list.length ? (
          <Tree treeData={treeData} defaultExpandAll selectable={false} showLine style={{ padding: '8px 4px' }} />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('暂无导航')} />
        )}
      </Card>

      <Modal
        title={editing ? t('编辑 IVR：{{name}}', { name: editing.name }) : t('新建 IVR')}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={handleSave}
        confirmLoading={saving}
        okText={editing ? t('保存并热加载') : t('创建')}
        destroyOnHidden
        width={680}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 8 }}>
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
              <Input placeholder={t('如 main-menu')} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="access_number"
              label={t('接入号')}
              style={{ flex: 1 }}
              tooltip={t('分机直拨可进；SIP 线路的「入方向落点」填它即外线进导航；子菜单的接入号仅用于直拨测试')}
              rules={[
                { required: true, message: t('请输入接入号') },
                { pattern: /^[0-9]{4,6}$/, message: t('4-6 位数字') },
              ]}
            >
              <Input placeholder={t('如 6000')} prefix={<PhoneOutlined />} />
            </Form.Item>
            <Form.Item name="timeout_ms" label={t('按键等待(ms)')} style={{ width: 130 }}>
              <InputNumber min={2000} max={30000} step={1000} style={{ width: '100%' }} />
            </Form.Item>
          </Space>
          <Form.Item
            name="greeting"
            label={t('欢迎音（可选）')}
            tooltip={t('放音文件路径或 tone_stream://...；留空用默认提示音')}
            rules={[{ pattern: /^[A-Za-z0-9_\-./:%(),]{0,200}$/, message: t('路径含非法字符') }]}
          >
            <Input placeholder={t('留空 = 默认提示音；可填 ivr/ivr-welcome_to_freeswitch.wav')} />
          </Form.Item>
        </Form>

        <div style={{ display: 'flex', gap: 20 }}>
          {/* 拨号盘 */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 84px)', gap: 8 }}>
            {KEYS.map((k) => {
              const e = keymap[k]
              const meta = e ? ACTION_META[e.action] : null
              const active = selectedKey === k
              return (
                <button
                  key={k}
                  type="button"
                  onClick={() => setSelectedKey(k)}
                  style={{
                    height: 64,
                    borderRadius: 10,
                    cursor: 'pointer',
                    border: active ? `2px solid ${meta?.color ?? 'var(--c-primary)'}` : '1px solid rgba(128,128,160,0.35)',
                    background: meta ? `${meta.color}22` : 'transparent',
                    color: 'inherit',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 2,
                  }}
                >
                  <span style={{ fontSize: 18, fontWeight: 600 }} className="cell-mono">{k}</span>
                  <span style={{ fontSize: 11, color: meta?.color ?? 'rgba(128,128,160,0.7)' }}>
                    {e ? entrySummary(e, t) : t('未配置')}
                  </span>
                </button>
              )
            })}
          </div>

          {/* 选中键配置 */}
          <div style={{ flex: 1, minWidth: 220 }}>
            <div style={{ marginBottom: 10, fontWeight: 600 }}>
              <MenuOutlined /> {t('按键')} <span className="cell-mono">{selectedKey}</span> {t('的动作')}
            </div>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <Select
                style={{ width: '100%' }}
                placeholder={t('选择动作')}
                value={selected?.action}
                onChange={(v: ActionKind) => setEntry(selectedKey, {
                  action: v,
                  target: v === 'collect' ? formatCollect(DEFAULT_COLLECT) : undefined,
                })}
                options={(Object.keys(ACTION_META) as ActionKind[]).map((a) => ({
                  value: a,
                  label: (
                    <Space size={6}>
                      <span style={{ color: ACTION_META[a].color }}>{ACTION_META[a].icon}</span>
                      {t(ACTION_META[a].label)}
                    </Space>
                  ),
                }))}
              />
              {selected?.action === 'transfer' && (
                <Input
                  placeholder={t('目标号码（分机/5000 队列/接入号）')}
                  value={selected.target}
                  onChange={(ev) => setEntry(selectedKey, { target: ev.target.value })}
                  prefix={<PhoneOutlined />}
                />
              )}
              {selected?.action === 'submenu' && (
                <Select
                  style={{ width: '100%' }}
                  placeholder={t('选择子菜单')}
                  value={selected.target || undefined}
                  onChange={(v: string) => setEntry(selectedKey, { target: v })}
                  options={otherMenus.map((m) => ({ value: m.name, label: t('{{name}}（拨 {{num}}）', { name: m.name, num: m.access_number }) }))}
                  notFoundContent={t('先创建另一个 IVR 作为子菜单')}
                />
              )}
              {selected?.action === 'collect' && (() => {
                const p = parseCollect(selected.target)
                const patch = (np: Partial<CollectParams>) =>
                  setEntry(selectedKey, { target: formatCollect({ ...p, ...np }) })
                return (
                  <Space direction="vertical" size={8} style={{ width: '100%' }}>
                    <Select
                      style={{ width: '100%' }}
                      placeholder={t('收号提示音（可选，来自提示音库）')}
                      allowClear
                      value={p.prompt || undefined}
                      onChange={(v?: string) => patch({ prompt: v || '' })}
                      options={prompts.map((f) => ({ value: f.name, label: f.name }))}
                      notFoundContent={t('提示音库为空，可在「提示音库」上传/生成')}
                    />
                    <Space size={8} style={{ display: 'flex' }}>
                      <InputNumber
                        style={{ flex: 1 }}
                        addonBefore={t('最小位')}
                        min={1}
                        max={32}
                        value={p.min}
                        onChange={(v) => patch({ min: Number(v) || 1 })}
                      />
                      <InputNumber
                        style={{ flex: 1 }}
                        addonBefore={t('最大位')}
                        min={1}
                        max={32}
                        value={p.max}
                        onChange={(v) => patch({ max: Number(v) || 1 })}
                      />
                    </Space>
                    <InputNumber
                      style={{ width: '100%' }}
                      addonBefore={t('超时(ms)')}
                      min={1000}
                      max={60000}
                      step={1000}
                      value={p.timeout}
                      onChange={(v) => patch({ timeout: Number(v) || 8000 })}
                    />
                    <Input
                      placeholder={t('收号完成后转接号码（分机/队列/接入号）')}
                      value={p.next}
                      onChange={(ev) => patch({ next: ev.target.value.trim() })}
                      prefix={<PhoneOutlined />}
                    />
                    <div className="cell-dim" style={{ fontSize: 12 }}>
                      {t('收号存入变量')} <span className="cell-mono">cc_collected</span>{t('，随话单带出可在话单页查看。')}
                    </div>
                  </Space>
                )
              })()}
              {selected && (
                <Button size="small" danger onClick={() => setEntry(selectedKey, null)}>
                  {t('清除此键')}
                </Button>
              )}
              <div className="cell-dim" style={{ fontSize: 12, lineHeight: 1.7 }}>
                {t('提示：子菜单里配「返回上级」可回到主菜单;「重播」重新播放当前菜单;未配置的按键会触发无效提示音并重试。')}
              </div>
            </Space>
          </div>
        </div>
      </Modal>
    </ListPageShell>
  )
}
