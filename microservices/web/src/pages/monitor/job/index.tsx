import { useEffect, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Modal, Form, Input, Select,
  Card, InputNumber, Drawer, Grid, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, ReloadOutlined, ClearOutlined, SearchOutlined,
  EditOutlined, DeleteOutlined, PlayCircleOutlined, PauseCircleOutlined, ThunderboltOutlined,
  HistoryOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { ScheduledJob } from '@/types'
import {
  getJobList, createJob, updateJob, deleteJob,
  startJob, stopJob, runJob, cleanupJobLogs,
  getJobHealth, type JobHealth,
  getJobHeartbeats, type JobHeartbeat,
  getJobTargets, type JobTarget, JOB_TARGET_LABELS,
  getJobLogList, type ScheduledJobLog,
} from '@/api/monitor'
import TableToolbar from '@/components/TableToolbar'
import TableRowActions from '@/components/TableRowActions'
import StatusPill from '@/components/StatusPill'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import TaskRunsPanel from './TaskRunsPanel'
import './styles.css'

interface SearchParams {
  page: number
  page_size: number
  name?: string
  status?: number
}

export default function JobPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<ScheduledJob[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<ScheduledJob | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [cleanupModalOpen, setCleanupModalOpen] = useState(false)
  const [cleanupSubmitting, setCleanupSubmitting] = useState(false)
  const [health, setHealth] = useState<JobHealth | null>(null)
  const [targets, setTargets] = useState<JobTarget[]>([])
  // 执行日志抽屉：logJob=null 表示关闭；job_id 为 0 时看全部任务的日志
  const [logJob, setLogJob] = useState<ScheduledJob | null>(null)
  const [logs, setLogs] = useState<ScheduledJobLog[]>([])
  const [logTotal, setLogTotal] = useState(0)
  const [logLoading, setLogLoading] = useState(false)
  const [logPage, setLogPage] = useState(1)
  const [logStatus, setLogStatus] = useState<number | undefined>(undefined)
  const [form] = Form.useForm()
  const [cleanupForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getJobList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取任务列表失败'))
    } finally {
      setLoading(false)
    }
    getJobHealth()
      .then(setHealth)
      .catch(() => {
        // ignore
      })
  }, [t])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  // 目标清单只在挂载时取一次：它由后端的调度分发表决定，运行期不会变。
  useEffect(() => {
    getJobTargets()
      .then((res) => setTargets(res.list ?? []))
      .catch(() => {
        // 取不到就让表单退回可输入，不挡住用户建任务
        setTargets([])
      })
  }, [])

  const fetchLogs = useCallback(async (jobID: number, page: number, status?: number) => {
    setLogLoading(true)
    try {
      const res = await getJobLogList({
        page,
        page_size: 10,
        ...(jobID ? { job_id: jobID } : {}),
        ...(status === undefined ? {} : { status }),
      })
      setLogs(res.list ?? [])
      setLogTotal(res.total ?? 0)
    } catch {
      message.error(t('获取执行日志失败'))
    } finally {
      setLogLoading(false)
    }
  }, [t])

  const openLogs = (record: ScheduledJob | null) => {
    setLogJob(record ?? ({ id: 0, name: '全部任务' } as ScheduledJob))
    setLogPage(1)
    setLogStatus(undefined)
    fetchLogs(record?.id ?? 0, 1, undefined)
  }

  const handleSearch = (values: { name?: string; status?: number }) => {
    setParams({ ...params, page: 1, name: values.name, status: values.status })
  }

  const handleSearchReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: ScheduledJob) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      group_name: record.group_name,
      cron_expression: record.cron_expression,
      invoke_target: record.invoke_target,
      description: record.description,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await updateJob(editRecord.id, values)
        message.success(t('更新成功'))
      } else {
        await createJob(values)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteJob(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleStart = async (id: number) => {
    try {
      await startJob(id)
      message.success(t('启动成功'))
      fetchList(params)
    } catch {
      message.error(t('启动失败'))
    }
  }

  const handleStop = async (id: number) => {
    try {
      await stopJob(id)
      message.success(t('停止成功'))
      fetchList(params)
    } catch {
      message.error(t('停止失败'))
    }
  }

  const handleRun = async (id: number) => {
    try {
      await runJob(id)
      message.success(t('执行成功'))
    } catch {
      message.error(t('执行失败'))
    }
  }

  const handleCleanup = async () => {
    const values = await cleanupForm.validateFields().catch(() => null)
    if (!values) return
    setCleanupSubmitting(true)
    try {
      const res = await cleanupJobLogs(values.retention_days)
      message.success(t('清理成功，共删除 {{n}} 条日志', { n: res.deleted_rows }))
      setCleanupModalOpen(false)
      cleanupForm.resetFields()
    } catch {
      message.error(t('清理失败'))
    } finally {
      setCleanupSubmitting(false)
    }
  }

  const columns: ColumnsType<ScheduledJob> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 180,
      ellipsis: { showTitle: false },
      render: (v: string) => (
        <Tooltip title={v}>
          <span className="job-cell-ellipsis list-primary-cell">{v}</span>
        </Tooltip>
      ),
    },
    {
      title: t('分组'),
      dataIndex: 'group_name',
      width: 120,
      ellipsis: { showTitle: false },
      render: (v: string) => v ? (
        <Tooltip title={v}>
          <Tag variant="filled" className="job-table-tag">{v}</Tag>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('Cron表达式'),
      dataIndex: 'cron_expression',
      width: 150,
      ellipsis: { showTitle: false },
      render: (v: string) => (
        <Tooltip title={v}>
          <Tag variant="filled" color="geekblue" className="cell-mono job-table-tag">{v}</Tag>
        </Tooltip>
      ),
    },
    {
      title: t('调用目标'),
      dataIndex: 'invoke_target',
      width: 200,
      ellipsis: { showTitle: false },
      render: (v: string) => {
        const known = JOB_TARGET_LABELS[v]
        // 历史数据可能存着白名单外的目标（写入校验是后加的），标出来而不是
        // 让它看起来正常——它一触发就会失败。
        const listed = targets.some((tg) => tg.target === v)
        if (!known && !listed) {
          return (
            <Tooltip title={t('该目标不在调度器的内置清单里，任务触发时会直接失败；请改选一个有效目标')}>
              <Tag color="error" className="cell-mono job-table-tag">{v} · {t('无效')}</Tag>
            </Tooltip>
          )
        }
        return (
          <Tooltip title={t(known?.hint ?? targets.find((tg) => tg.target === v)?.description ?? v)}>
            <span className="cell-mono cell-dim job-cell-ellipsis">{t(known?.label ?? v)}</span>
          </Tooltip>
        )
      },
    },
    {
      title: t('说明'),
      dataIndex: 'description',
      width: 220,
      ellipsis: { showTitle: false },
      render: (v: string) => v ? (
        <Tooltip title={v}>
          <span className="job-cell-ellipsis">{v}</span>
        </Tooltip>
      ) : <span className="cell-muted">—</span>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (v: number) => (
        v === 1 ? <StatusPill tone="success" label="运行中" /> : <StatusPill tone="muted" label="已暂停" />
      ),
    },
    {
      title: t('下次执行'),
      dataIndex: 'next_run_time',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 148,
      fixed: 'right' as const,
      align: 'center' as const,
      render: (_, record) => (
        <TableRowActions
          className="job-row-actions"
          menuOnly={compactActions}
          maxInline={3}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:job:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'toggle',
              label: record.status === 0 ? t('启动') : t('暂停'),
              icon: record.status === 0 ? <PlayCircleOutlined /> : <PauseCircleOutlined />,
              show: hasPerm('system:job:run'),
              onClick: () => {
                if (record.status === 0) void handleStart(record.id)
                else void handleStop(record.id)
              },
            },
            {
              key: 'run',
              label: t('立即执行'),
              icon: <ThunderboltOutlined />,
              show: hasPerm('system:job:run'),
              confirm: t('确认立即执行该任务?'),
              onClick: () => { void handleRun(record.id) },
            },
            {
              key: 'logs',
              label: t('执行日志'),
              icon: <HistoryOutlined />,
              onClick: () => openLogs(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:job:delete'),
              confirm: t('确认删除该任务?'),
              onClick: () => { void handleDelete(record.id) },
            },
          ]}
        />
      ),
    },
  ]

  return (
    <div className="page-list job-page">
      <TaskRunsPanel />

      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="name">
            <Input placeholder={t('搜索任务名称')} prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder={t('状态')} style={{ width: 110 }} allowClear>
              <Select.Option value={1}>{t('运行中')}</Select.Option>
              <Select.Option value={0}>{t('已暂停')}</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleSearchReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
          {health && (
            <Form.Item className="job-health-summary">
              <Space size={8} wrap>
                <span className="health-pill">{t('共 {{n}}', { n: health.total })}</span>
                <span className="health-pill health-pill-success">
                  <span className="live-dot" />{t('运行 {{n}}', { n: health.enabled })}
                </span>
                <span className="health-pill">{t('暂停 {{n}}', { n: health.paused })}</span>
                <span className={`health-pill ${health.recent_failed > 0 ? 'health-pill-danger' : ''}`}>
                  {t('近 {{n}}h 失败 {{m}}', { n: health.window_hours, m: health.recent_failed })}
                </span>
              </Space>
            </Form.Item>
          )}
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="定时任务"
          total={total}
          extra={
            <Space wrap className="job-toolbar-actions">
              <Button icon={<HistoryOutlined />} onClick={() => openLogs(null)}>{t('执行日志')}</Button>
              {hasPerm('system:job:run') && (
                <Button
                  icon={<ClearOutlined />}
                  onClick={() => { cleanupForm.resetFields(); setCleanupModalOpen(true) }}
                >
                  {t('清理日志')}
                </Button>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>{t('刷新')}</Button>
              {hasPerm('system:job:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增任务')}</Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          rowClassName={(record) => (record.status === 0 ? 'job-row-paused' : '')}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无定时任务" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Modal
        title={editRecord ? t('编辑任务') : t('新增任务')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入任务名称') }]}>
            <Input />
          </Form.Item>
          <Form.Item name="group_name" label={t('分组')}>
            <Input />
          </Form.Item>
          <Form.Item name="cron_expression" label={t('Cron表达式')} rules={[{ required: true, message: t('请输入Cron表达式') }]}>
            <Input placeholder={t('如: 0 * * * * *（秒 分 时 日 月 周）')} />
          </Form.Item>
          {/* 目标必须是调度器内置的，自由文本会存进库、等触发时才失败。
              清单接口取不到时退回输入框，至少不挡住建任务（后端仍会校验）。 */}
          <Form.Item
            name="invoke_target"
            label={t('调用目标')}
            rules={[{ required: true, message: t('请选择调用目标') }]}
            tooltip={t('仅可选择调度器内置的目标；后端会在保存时校验')}
          >
            {targets.length > 0 ? (
              <Select
                placeholder={t('请选择调用目标')}
                optionLabelProp="label"
                options={targets.map((tg) => ({
                  value: tg.target,
                  label: JOB_TARGET_LABELS[tg.target]?.label ?? tg.target,
                  title: tg.description,
                }))}
                optionRender={(option) => (
                  <div>
                    <div>{t(option.data.label)}</div>
                    <div className="cell-dim" style={{ fontSize: 12 }}>
                      {t(JOB_TARGET_LABELS[option.data.value as string]?.hint ?? option.data.title)}
                    </div>
                  </div>
                )}
              />
            ) : (
              <Input placeholder={t('目标清单加载失败，可手动输入（保存时后端仍会校验）')} />
            )}
          </Form.Item>
          <Form.Item name="description" label={t('说明')}>
            <Input />
          </Form.Item>
          <Form.Item name="status" label={t('状态')} initialValue={0}>
            <Select>
              <Select.Option value={1}>{t('运行中')}</Select.Option>
              <Select.Option value={0}>{t('已暂停')}</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('清理任务日志')}
        open={cleanupModalOpen}
        onOk={handleCleanup}
        onCancel={() => setCleanupModalOpen(false)}
        confirmLoading={cleanupSubmitting}
        destroyOnHidden
      >
        <Form form={cleanupForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="retention_days"
            label={t('保留天数')}
            rules={[{ required: true, message: t('请输入保留天数') }]}
            initialValue={30}
          >
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={logJob?.id ? t('执行日志 · {{name}}', { name: logJob.name }) : t('执行日志 · 全部任务')}
        open={!!logJob}
        onClose={() => setLogJob(null)}
        size="min(760px, 100vw)"
        rootClassName="job-log-drawer"
        destroyOnHidden
      >
        <Space className="job-log-toolbar" style={{ marginBottom: 12 }} wrap>
          <Select
            allowClear
            placeholder={t('执行结果')}
            style={{ width: 160 }}
            value={logStatus}
            onChange={(value) => {
              setLogStatus(value)
              setLogPage(1)
              fetchLogs(logJob?.id ?? 0, 1, value)
            }}
            options={[
              { value: 1, label: t('成功') },
              { value: 0, label: t('失败') },
            ]}
          />
          <Button
            icon={<ReloadOutlined />}
            onClick={() => fetchLogs(logJob?.id ?? 0, logPage, logStatus)}
          >
            {t('刷新')}
          </Button>
        </Space>
        <Table
          rowKey="id"
          className="list-table job-log-table"
          size="small"
          loading={logLoading}
          dataSource={logs}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无执行日志（任务触发后自动记录）" compact /> }}
          pagination={{
            current: logPage,
            pageSize: 10,
            total: logTotal,
            showSizeChanger: false,
            onChange: (page) => {
              setLogPage(page)
              fetchLogs(logJob?.id ?? 0, page, logStatus)
            },
          }}
          columns={[
            ...(logJob?.id
              ? []
              : [{
                title: t('任务'),
                dataIndex: 'job_name',
                width: 170,
                ellipsis: { showTitle: false },
                render: (v: string) => (
                  <Tooltip title={v}>
                    <span className="job-cell-ellipsis">{v}</span>
                  </Tooltip>
                ),
              }]),
            {
              title: t('结果'),
              dataIndex: 'status',
              width: 90,
              render: (v: number) =>
                v === 1 ? <Tag color="success">{t('成功')}</Tag> : <Tag color="error">{t('失败')}</Tag>,
            },
            {
              title: t('耗时'),
              dataIndex: 'duration',
              width: 100,
              render: (v: number) => <span className="cell-mono">{v} ms</span>,
            },
            {
              title: t('输出'),
              dataIndex: 'message',
              width: 260,
              ellipsis: { showTitle: false },
              render: (v: string) => (
                <Tooltip title={v}>
                  <span className="cell-mono cell-dim job-cell-ellipsis">{v || '—'}</span>
                </Tooltip>
              ),
            },
            {
              title: t('时间'),
              dataIndex: 'created_at',
              width: 170,
              className: 'cell-time',
              render: formatDateTime,
            },
          ]}
        />
      </Drawer>

      <HeartbeatsCard />
    </div>
  )
}

// 服务任务心跳：monitor 进程外的分布式任务（各服务内循环 + 主机 shell cron）
// 最近运行状态。stale（超期未上报）标红——静默停摆在这里现形。
function HeartbeatsCard() {
  const { t } = useTranslation()
  const [list, setList] = useState<JobHeartbeat[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getJobHeartbeats()
      setList(res.list ?? [])
    } catch {
      message.error(t('获取任务心跳失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { void load() }, [load])

  const fmtInterval = (sec: number) => {
    if (!sec) return '—'
    if (sec % 86400 === 0) return t('{{n}} 天', { n: sec / 86400 })
    if (sec % 3600 === 0) return t('{{n}} 小时', { n: sec / 3600 })
    if (sec % 60 === 0) return t('{{n}} 分钟', { n: sec / 60 })
    return t('{{n}} 秒', { n: sec })
  }

  const columns: ColumnsType<JobHeartbeat> = [
    {
      title: t('任务'),
      dataIndex: 'job_key',
      width: 240,
      render: (v: string, r) => (
        <div className="job-heartbeat-task">
          <Tooltip title={v}>
            <span className="cell-mono job-cell-ellipsis">{v}</span>
          </Tooltip>
          <Tooltip title={r.description}>
            <span className="cell-dim job-cell-ellipsis job-heartbeat-description">{r.description || '—'}</span>
          </Tooltip>
        </div>
      ),
    },
    {
      title: t('来源'),
      dataIndex: 'service',
      width: 130,
      render: (v: string) => (
        <Tooltip title={v}>
          <Tag variant="filled" className="job-table-tag">{v}</Tag>
        </Tooltip>
      ),
    },
    { title: t('期望间隔'), dataIndex: 'interval_sec', width: 110, render: fmtInterval },
    {
      title: t('上次运行'),
      dataIndex: 'last_run_at',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: t('状态'),
      key: 'beat_status',
      width: 130,
      render: (_, r) =>
        r.stale ? (
          <StatusPill tone="danger" label="超期未上报" />
        ) : r.last_status === 'error' ? (
          <StatusPill tone="danger" label="上次失败" />
        ) : (
          <StatusPill tone="success" label="正常" />
        ),
    },
    {
      title: t('累计（失败/总数）'),
      key: 'beat_runs',
      width: 150,
      render: (_, r) => (
        <span className="cell-mono">
          {r.fails > 0 ? <span style={{ color: 'var(--c-error-strong)' }}>{r.fails}</span> : 0} / {r.runs}
        </span>
      ),
    },
    {
      title: t('最近错误'),
      dataIndex: 'last_error',
      width: 260,
      ellipsis: { showTitle: false },
      render: (v: string) => v ? (
        <Tooltip title={v}>
          <span className="cell-mono job-cell-ellipsis">{v}</span>
        </Tooltip>
      ) : <span className="cell-dim">—</span>,
    },
  ]

  return (
    <Card className="list-main-card" bordered={false}>
      <TableToolbar
        title="服务任务心跳"
        total={list.length}
        extra={<Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>}
      />
      <Table
        rowKey="job_key"
        className="list-table job-heartbeat-table"
        columns={columns}
        dataSource={list}
        loading={loading}
        rowClassName={(record) => (record.stale || record.last_status === 'error' ? 'job-heartbeat-alert' : '')}
        scroll={{ x: 'max-content' }}
        pagination={false}
        locale={{ emptyText: <GlassEmpty text="暂无任务心跳（各服务后台循环运行后自动上报）" compact /> }}
      />
    </Card>
  )
}
