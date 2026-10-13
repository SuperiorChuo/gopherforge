import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Table, Button, Space, Tag, Input, Form, Modal, Select, Checkbox, TimePicker, Grid } from 'antd'
import { message } from '@/utils/feedback'
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, EditOutlined,
  ClockCircleOutlined, PhoneOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { type Dayjs } from 'dayjs'
import {
  listCcTimeRoutes, createCcTimeRoute, updateCcTimeRoute, deleteCcTimeRoute,
  listPrompts,
  type CcTimeRoute, type CcTimeRoutePayload,
} from '@/api/cc'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { useTableQuery } from '@/hooks/useTableQuery'

// FS wday 映射：周日=1 … 周六=7。展示顺序按周一起排，值对应 FS 数字。
const WEEKDAYS: { value: number; label: string }[] = [
  { value: 2, label: '周一' },
  { value: 3, label: '周二' },
  { value: 4, label: '周三' },
  { value: 5, label: '周四' },
  { value: 6, label: '周五' },
  { value: 7, label: '周六' },
  { value: 1, label: '周日' },
]

const WDAY_LABEL: Record<number, string> = Object.fromEntries(WEEKDAYS.map((d) => [d.value, d.label]))

// 选中的 wday 数字数组 → FS wday 字符串：连续段压成 "a-b"，离散用逗号。
// 连续判断按 FS 数字自然序（1..7）。
function daysToWday(days: number[]): string {
  const sorted = [...new Set(days)].sort((a, b) => a - b)
  if (sorted.length === 0) return ''
  const parts: string[] = []
  let start = sorted[0]
  let prev = sorted[0]
  for (let i = 1; i <= sorted.length; i++) {
    const cur = sorted[i]
    if (cur === prev + 1) {
      prev = cur
      continue
    }
    parts.push(start === prev ? `${start}` : `${start}-${prev}`)
    start = cur
    prev = cur
  }
  return parts.join(',')
}

// FS wday 字符串 → 数字数组（编辑回填）。支持 "2-6" 段与 "2,4,6" 离散混合。
function wdayToDays(wday: string): number[] {
  const out: number[] = []
  for (const seg of wday.split(',')) {
    const s = seg.trim()
    if (!s) continue
    if (s.includes('-')) {
      const [a, b] = s.split('-').map((n) => parseInt(n, 10))
      if (!isNaN(a) && !isNaN(b)) {
        for (let i = a; i <= b; i++) out.push(i)
      }
    } else {
      const n = parseInt(s, 10)
      if (!isNaN(n)) out.push(n)
    }
  }
  return out
}

// wday 字符串人话展示（表格用），如 "2-6" → "周一~周五"
function wdayHuman(wday: string, t: TFunction): string {
  const days = wdayToDays(wday)
  if (days.length === 0) return '—'
  return days.map((d) => WDAY_LABEL[d] ? t(WDAY_LABEL[d]) : d).join(t('、'))
}

// 时段行：TimePicker.RangePicker 的 [起, 止]
type RangeRow = [Dayjs, Dayjs] | null

// 表单值形态
type FormValues = {
  name: string
  access_number: string
  days: number[]
  ranges: RangeRow[]
  open_target: string
  closed_target?: string
  closed_prompt?: string
}

type SoundOption = { path: string; name: string }

export default function CcTimeRoutesPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const [modal, setModal] = useState(false)
  const [editing, setEditing] = useState<CcTimeRoute | null>(null)
  const [saving, setSaving] = useState(false)
  const [prompts, setPrompts] = useState<SoundOption[]>([])
  const [form] = Form.useForm<FormValues>()

  const fetcher = useCallback(async () => {
    const res = await listCcTimeRoutes()
    const list = res.list ?? []
    return { list, total: list.length }
  }, [])
  const { list, loading, reload } = useTableQuery({
    params: {},
    fetcher,
    onError: useCallback(() => message.error(t('获取时间路由失败')), [t]),
  })

  useEffect(() => {
    listPrompts()
      .then((r) => setPrompts((r.list ?? []).map((p) => ({ path: p.path, name: p.name }))))
      .catch(() => setPrompts([]))
  }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ days: [2, 3, 4, 5, 6], ranges: [null] })
    setModal(true)
  }

  const openEdit = (row: CcTimeRoute) => {
    setEditing(row)
    const ranges: RangeRow[] = row.time_ranges
      .split(',')
      .map((seg) => seg.trim())
      .filter(Boolean)
      .map((seg) => {
        const [a, b] = seg.split('-')
        return [dayjs(a, 'HH:mm'), dayjs(b, 'HH:mm')] as [Dayjs, Dayjs]
      })
    form.setFieldsValue({
      name: row.name,
      access_number: row.access_number,
      days: wdayToDays(row.wdays),
      ranges: ranges.length ? ranges : [null],
      open_target: row.open_target,
      closed_target: row.closed_target || undefined,
      closed_prompt: row.closed_prompt || undefined,
    })
    setModal(true)
  }

  const handleSave = async (values: FormValues) => {
    const wdays = daysToWday(values.days)
    if (!wdays) {
      message.error(t('请选择工作日'))
      return
    }
    const validRanges = (values.ranges || []).filter((r): r is [Dayjs, Dayjs] => !!r && !!r[0] && !!r[1])
    if (validRanges.length === 0) {
      message.error(t('请至少配置一个营业时段'))
      return
    }
    const time_ranges = validRanges
      .map(([a, b]) => `${a.format('HH:mm')}-${b.format('HH:mm')}`)
      .join(',')
    const payload: CcTimeRoutePayload = {
      name: values.name.trim(),
      access_number: values.access_number.trim(),
      wdays,
      time_ranges,
      open_target: values.open_target.trim(),
      closed_target: values.closed_target?.trim() || undefined,
      closed_prompt: values.closed_prompt || undefined,
    }
    setSaving(true)
    try {
      if (editing) {
        await updateCcTimeRoute(editing.name, payload)
        message.success(t('时间路由 {{name}} 已更新', { name: editing.name }))
      } else {
        await createCcTimeRoute(payload)
        message.success(t('时间路由已创建，拨 {{number}} 按营业时间分流', { number: payload.access_number }))
      }
      setModal(false)
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteCcTimeRoute(name)
      message.success(t('时间路由 {{name}} 已删除', { name }))
      void reload()
    } catch (e) {
      message.error((e as Error)?.message || t('删除失败'))
    }
  }

  const columns: ColumnsType<CcTimeRoute> = [
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 140,
      render: (v: string) => (
        <Space size={6}><ClockCircleOutlined /><span className="cell-mono">{v}</span></Space>
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
      title: t('工作日'),
      dataIndex: 'wdays',
      width: 200,
      responsive: ['md'],
      render: (v: string) => <span className="cell-dim">{wdayHuman(v, t)}</span>,
    },
    {
      title: t('营业时段'),
      dataIndex: 'time_ranges',
      responsive: ['lg'],
      render: (v: string) => (
        <Space size={4} wrap>
          {v.split(',').filter(Boolean).map((seg) => (
            <Tag key={seg} variant="filled">{seg}</Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('营业转'),
      dataIndex: 'open_target',
      width: 100,
      responsive: ['sm'],
      render: (v: string) => <span className="cell-mono">{v}</span>,
    },
    {
      title: t('非营业转'),
      dataIndex: 'closed_target',
      width: 130,
      responsive: ['md'],
      render: (v: string, r) =>
        v
          ? <span className="cell-mono">{v}</span>
          : <span className="cell-dim">{r.closed_prompt ? t('提示后挂断') : t('默认提示后挂断')}</span>,
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
              confirm: t('确认删除时间路由 {{name}}？', { name: row.name }),
              onClick: () => { void handleDelete(row.name) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <ListPageShell
      className="cc-timeroutes-page"
      toolbar={
        <TableToolbar
          title="营业时间路由"
          total={list.length}
          extra={
            <Space size={16}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新建时间路由')}</Button>
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
          locale={{ emptyText: <GlassEmpty text="暂无时间路由（拨接入号先判营业时间，营业转队列/IVR，非营业转提示/兜底）" compact /> }}
        />

      <Modal
        title={editing ? t('编辑时间路由：{{name}}', { name: editing.name }) : t('新建时间路由')}
        open={modal}
        onCancel={() => setModal(false)}
        onOk={() => form.submit()}
        confirmLoading={saving}
        okText={editing ? t('保存并重载') : t('创建')}
        destroyOnHidden
        width={600}
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
              <Input placeholder={t('如 biz-hours')} disabled={!!editing} />
            </Form.Item>
            <Form.Item
              name="access_number"
              label={t('接入号')}
              style={{ flex: 1 }}
              rules={[{ required: true, message: t('请输入接入号') }, { pattern: /^[0-9]{3,6}$/, message: t('3-6 位数字') }]}
            >
              <Input placeholder={t('如 6600')} prefix={<PhoneOutlined />} />
            </Form.Item>
          </Space>

          <Form.Item name="days" label={t('工作日')} rules={[{ required: true, message: t('请选择工作日') }]}>
            <Checkbox.Group
              options={WEEKDAYS.map((d) => ({ label: t(d.label), value: d.value }))}
            />
          </Form.Item>

          <Form.Item label={t('营业时段')} required tooltip={t('可加多段，如 09:00-12:00 与 13:30-18:00')}>
            <Form.List name="ranges">
              {(fields, { add, remove }) => (
                <Space direction="vertical" size={8} style={{ display: 'flex' }}>
                  {fields.map((field) => (
                    <Space key={field.key} size={8}>
                      <Form.Item {...field} noStyle>
                        <TimePicker.RangePicker format="HH:mm" minuteStep={5} />
                      </Form.Item>
                      {fields.length > 1 && (
                        <Button
                          danger
                          type="text"
                          icon={<DeleteOutlined />}
                          onClick={() => remove(field.name)}
                        />
                      )}
                    </Space>
                  ))}
                  <Button type="dashed" onClick={() => add(null)} icon={<PlusOutlined />} style={{ width: 220 }}>
                    {t('加一段时间')}
                  </Button>
                </Space>
              )}
            </Form.List>
          </Form.Item>

          <Form.Item
            name="open_target"
            label={t('营业中转')}
            tooltip={t('营业时间内转到的目标号码：队列（如 7000）/IVR（如 6000）/分机/响铃组接入号')}
            rules={[{ required: true, message: t('请输入营业转目标号码') }, { pattern: /^[0-9*#+]{2,32}$/, message: t('号码格式不对') }]}
          >
            <Input placeholder={t('如 7000（转队列）')} />
          </Form.Item>

          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="closed_target"
              label={t('非营业转')}
              style={{ flex: 1 }}
              tooltip={t('非营业时间转到的目标号码；留空=播放下方提示音后挂断')}
              rules={[{ pattern: /^[0-9*#+]{0,32}$/, message: t('号码格式不对') }]}
            >
              <Input placeholder={t('留空=提示后挂断')} />
            </Form.Item>
            <Form.Item
              name="closed_prompt"
              label={t('非营业提示音')}
              style={{ flex: 1 }}
              tooltip={t('仅在「非营业转」留空时生效；不选用默认抱歉提示')}
            >
              <Select allowClear placeholder={t('默认（misc/sorry）')}>
                {prompts.map((p) => (
                  <Select.Option key={p.path} value={p.path}>{p.name}</Select.Option>
                ))}
              </Select>
            </Form.Item>
          </Space>
        </Form>
      </Modal>
    </ListPageShell>
  )
}
