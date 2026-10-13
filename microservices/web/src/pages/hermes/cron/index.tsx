// Hermes M5 定时任务：控制面 cron job 管理（表达式 + 提示词 + 开关）+ 触发记录。
// manager 在设备端评估调度、到点推提示词给本地 Hermes，结果回传落 runs 审计。
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Button,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tooltip,
} from 'antd'
import { DeleteOutlined, EditOutlined, HistoryOutlined, PlusOutlined, ReloadOutlined, ScheduleOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  createHermesCronJob,
  deleteHermesCronJob,
  listHermesCronJobs,
  listHermesCronRuns,
  toggleHermesCronJob,
  updateHermesCronJob,
  type HermesCronJob,
  type HermesCronJobPayload,
  type HermesCronJobView,
  type HermesCronRun,
} from '@/api/ai/hermes'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import { message } from '@/utils/feedback'
import { stateTone } from '../_shared'

const RUN_STATUS_LABELS: Record<string, string> = {
  triggered: '已触发',
  completed: '已完成',
  failed: '失败',
}

const RUNS_PAGE_SIZE = 20

// 服务端按字节校验（varchar/len()），前端同口径提示，避免中文写到一半才被 400。
const utf8Length = (value?: string) => (value ? new TextEncoder().encode(value).length : 0)

const byteLimitRule = (limit: number, label: string, t: TFunction) => ({
  validator: (_: unknown, value?: string) =>
    utf8Length(value) <= limit
      ? Promise.resolve()
      : Promise.reject(new Error(t('{{label}}过长：按 UTF-8 字节计最多 {{n}}（中文约 3 字节/字）', { label, n: limit }))),
})

export default function HermesCronPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('ai:hermes:cron:write')
  const [params, setParams] = useState({ page: 1, page_size: 20 })
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editing, setEditing] = useState<HermesCronJob | null>(null)
  const [togglingID, setTogglingID] = useState<number | null>(null)
  const [form] = Form.useForm()
  // runs 抽屉
  const [runsOpen, setRunsOpen] = useState(false)
  const [runsJob, setRunsJob] = useState<HermesCronJobView | null>(null)
  const [runsLoading, setRunsLoading] = useState(false)
  const [runs, setRuns] = useState<HermesCronRun[]>([])
  const [runsTotal, setRunsTotal] = useState(0)
  const [runsPage, setRunsPage] = useState(1)

  const fetchList = useCallback((p: { page: number; page_size: number }) => listHermesCronJobs(p), [])
  const { list: jobs, total, loading, reload, patchList } = useTableQuery({ params, fetcher: fetchList })

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue({
      name: '',
      description: '',
      cron_expression: '0 9 * * 1-5',
      action_type: 'prompt',
      prompt: '',
      enabled: true,
    })
    setOpen(true)
  }

  const openEdit = (job: HermesCronJob) => {
    setEditing(job)
    form.setFieldsValue({
      name: job.name,
      description: job.description,
      cron_expression: job.cron_expression,
      action_type: job.action_type,
      prompt: job.prompt,
      enabled: job.enabled,
    })
    setOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    const payload: HermesCronJobPayload = {
      name: values.name,
      description: values.description ?? '',
      cron_expression: values.cron_expression,
      action_type: values.action_type ?? 'prompt',
      prompt: values.prompt,
      enabled: values.enabled,
    }
    setSaving(true)
    try {
      if (editing) {
        await updateHermesCronJob(editing.id, payload)
        message.success(t('定时任务已更新'))
      } else {
        await createHermesCronJob(payload)
        message.success(t('定时任务已创建'))
      }
      setOpen(false)
      await reload()
    } catch {
      // Request layer already reports the server message.
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (job: HermesCronJob) => {
    if (togglingID !== null) return
    setTogglingID(job.id)
    try {
      const updated = await toggleHermesCronJob(job.id)
      patchList((current) =>
        current.map((item) => (item.id === job.id ? { ...item, enabled: updated?.enabled ?? !item.enabled } : item)),
      )
    } catch {
      // Request layer already reports the server message.
    } finally {
      setTogglingID(null)
    }
  }

  const remove = async (job: HermesCronJob) => {
    try {
      await deleteHermesCronJob(job.id)
      message.success(t('定时任务已删除'))
      if (jobs.length === 1 && params.page > 1) {
        setParams((current) => ({ ...current, page: current.page - 1 }))
      } else {
        await reload()
      }
    } catch {
      // Request layer already reports the server message.
    }
  }

  const loadRuns = useCallback(async (jobID: number, nextPage: number) => {
    setRunsLoading(true)
    try {
      const result = await listHermesCronRuns(jobID, { page: nextPage, page_size: RUNS_PAGE_SIZE })
      setRuns(result.list ?? [])
      setRunsTotal(result.total ?? 0)
      setRunsPage(nextPage)
    } catch {
      // Request layer already reports the server message.
    } finally {
      setRunsLoading(false)
    }
  }, [])

  const openRuns = (job: HermesCronJobView) => {
    setRunsJob(job)
    setRunsOpen(true)
    // 换任务先清上一个任务的记录，避免加载失败/加载中展示旧数据
    setRuns([])
    setRunsTotal(0)
    setRunsPage(1)
    void loadRuns(job.id, 1)
  }

  const columns: ColumnsType<HermesCronJobView> = [
    { title: t('名称'), dataIndex: 'name', width: 180 },
    {
      title: t('Cron 表达式'),
      dataIndex: 'cron_expression',
      width: 160,
      render: (value: string) => <span className="cell-mono">{value}</span>,
    },
    {
      title: t('提示词'),
      dataIndex: 'prompt',
      ellipsis: true,
      render: (value: string) => <span className="cell-muted">{value || '—'}</span>,
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 80,
      render: (value: boolean, record) => (
        <Switch
          size="small"
          checked={value}
          loading={togglingID === record.id}
          disabled={!canWrite || togglingID !== null}
          onChange={() => void toggle(record)}
        />
      ),
    },
    { title: t('触发次数'), dataIndex: 'run_count', width: 90, align: 'right', render: (value: number) => value ?? 0 },
    {
      title: t('最近触发'),
      dataIndex: 'last_run_at',
      width: 170,
      render: (value?: string) => (value ? formatDateTime(value) : <span className="cell-muted">—</span>),
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 160, render: (value: string) => formatDateTime(value) },
    {
      title: t('操作'),
      width: 160,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4}>
          <Tooltip title={t('触发记录')}>
            <Button type="text" size="small" icon={<HistoryOutlined />} onClick={() => void openRuns(record)} />
          </Tooltip>
          {canWrite && (
            <>
              <Button type="text" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)} />
              <Popconfirm title={t('删除此定时任务？触发记录会保留。')} onConfirm={() => void remove(record)}>
                <Button type="text" size="small" danger icon={<DeleteOutlined />} />
              </Popconfirm>
            </>
          )}
        </Space>
      ),
    },
  ]

  return (
    <ListPageShell
      className="hermes-cron-page"
      toolbar={
      <TableToolbar
        title="定时任务"
        total={total}
        icon={<ScheduleOutlined />}
        gradient="linear-gradient(135deg, #0ea5e9, #6366f1)"
        glow="rgba(99, 102, 241, 0.25)"
        description={t('控制面定义 cron 任务，manager 在设备端到点推提示词给本地 Hermes')}
        extra={
          <Space>
            <Tooltip title={t('刷新')}>
              <Button icon={<ReloadOutlined />} onClick={() => { void reload() }} loading={loading} />
            </Tooltip>
            {canWrite && (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('新建定时任务')}
              </Button>
            )}
          </Space>
        }
      />}
    >

        <Table
          rowKey="id"
          scroll={{ x: 800 }}
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={jobs}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (value) => t('共 {{n}} 条', { n: value }),
            onChange: (nextPage, nextSize) => {
              setParams({ page: nextPage, page_size: nextSize })
            },
          }}
          locale={{ emptyText: <GlassEmpty text="暂无定时任务" compact /> }}
        />

      <Modal
        title={editing ? t('编辑定时任务') : t('新建定时任务')}
        open={open}
        onOk={() => void submit()}
        onCancel={() => setOpen(false)}
        confirmLoading={saving}
        width={600}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="name"
            label={t('任务名')}
            rules={[{ required: true, message: t('请输入任务名') }, byteLimitRule(80, '任务名', t)]}
          >
            <Input placeholder={t('如：工作日早报')} />
          </Form.Item>
          <Form.Item
            name="cron_expression"
            label={t('Cron 表达式')}
            extra={t('5 段：分 时 日 月 周。示例：0 9 * * 1-5 = 工作日 9 点；*/15 * * * * = 每 15 分钟')}
            rules={[{ required: true }]}
          >
            <Input placeholder="0 9 * * 1-5" className="cell-mono" />
          </Form.Item>
          <Form.Item name="description" label={t('描述（可选）')}>
            <Input placeholder={t('这个任务做什么')} />
          </Form.Item>
          <Form.Item
            name="prompt"
            label={t('提示词')}
            rules={[{ required: true, message: t('请输入提示词') }, byteLimitRule(10000, '提示词', t)]}
            extra={t('到点时推给员工 Hermes 的内容')}
          >
            <Input.TextArea
              rows={3}
              placeholder={t('如：生成昨日工作日报')}
              showCount={{ formatter: ({ value }) => t('{{n}} / 10000 字节', { n: utf8Length(value) }) }}
            />
          </Form.Item>
          <Form.Item name="enabled" label={t('启用')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={t('触发记录 · {{name}}', { name: runsJob?.name ?? '' })}
        open={runsOpen}
        onClose={() => setRunsOpen(false)}
        width={640}
      >
        <Table
          rowKey="id"
          scroll={{ x: 800 }}
          size="small"
          loading={runsLoading}
          dataSource={runs}
          columns={[
            { title: t('设备'), dataIndex: 'device_id', ellipsis: true, render: (v: string) => <span className="cell-mono">{v}</span> },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 90,
              render: (v: string) => <StatusPill tone={stateTone(v)} label={RUN_STATUS_LABELS[v] ?? v} pulse={false} />,
            },
            { title: t('结果'), dataIndex: 'result', ellipsis: true, render: (v: string) => <span className="cell-muted">{v || '—'}</span> },
            { title: t('触发时间'), dataIndex: 'created_at', width: 170, render: (v: string) => formatDateTime(v) },
          ]}
          pagination={{
            total: runsTotal,
            current: runsPage,
            pageSize: RUNS_PAGE_SIZE,
            showTotal: (value) => t('共 {{n}} 条', { n: value }),
            onChange: (nextPage) => {
              if (runsJob) void loadRuns(runsJob.id, nextPage)
            },
          }}
          locale={{ emptyText: <GlassEmpty text="暂无触发记录（设备端尚未执行）" compact /> }}
        />
      </Drawer>
    </ListPageShell>
  )
}
