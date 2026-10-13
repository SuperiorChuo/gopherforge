import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import axios from 'axios'
import { Alert, Avatar, Button, Form, Input, Progress, Select, Space, Table, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined, SearchOutlined, SyncOutlined, UserOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  getMpFanSync,
  getMpAccount,
  listMpAccounts,
  listMpFans,
  startMpFanSync,
  type MpAccount,
  type MpFan,
  type MpFanSyncJob,
  type MpFanSyncStatus,
} from '@/api/mp'
import ListFilterForm from '@/components/common/ListFilterForm'
import ListPageShell from '@/components/common/ListPageShell'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import StatusPill from '@/components/common/StatusPill'
import { usePermission } from '@/hooks/usePermission'
import { useTableQuery } from '@/hooks/useTableQuery'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import './index.css'

const { Text } = Typography

interface SearchParams {
  account_id?: number
  keyword?: string
  page: number
  page_size: number
}

const syncStatusMeta: Record<MpFanSyncStatus, {
  label: string
  tone: 'warning' | 'info' | 'success' | 'danger'
  alert: 'warning' | 'info' | 'success' | 'error'
  progress: 'normal' | 'active' | 'success' | 'exception'
}> = {
  pending: { label: '等待执行', tone: 'warning', alert: 'warning', progress: 'normal' },
  running: { label: '同步中', tone: 'info', alert: 'info', progress: 'active' },
  succeeded: { label: '同步完成', tone: 'success', alert: 'success', progress: 'success' },
  failed: { label: '同步失败', tone: 'danger', alert: 'error', progress: 'exception' },
}

function splitTags(raw?: string): string[] {
  if (!raw) return []
  return raw
    .split(/[,，|]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function isActiveSync(job?: MpFanSyncJob | null): boolean {
  return job?.status === 'pending' || job?.status === 'running'
}

function isTerminalSync(job?: MpFanSyncJob | null): boolean {
  return job?.status === 'succeeded' || job?.status === 'failed'
}

function isRetryableSyncStatusError(error: unknown): boolean {
  if (!axios.isAxiosError(error) || !error.response) return true
  const { status } = error.response
  return status === 408 || status === 425 || status === 429 || status >= 500
}

function syncPercent(job: MpFanSyncJob): number {
  if (job.status === 'succeeded') return 100
  if (job.total_count <= 0) return 0
  return Math.min(100, Math.round((job.synced_count / job.total_count) * 100))
}

function SyncStatus({
  accountName,
  job,
  loading,
  refreshError,
  refreshRetryable,
}: {
  accountName?: string
  job: MpFanSyncJob | null
  loading: boolean
  refreshError: string
  refreshRetryable: boolean
}) {
  const { t } = useTranslation()
  if (loading && !job) {
    return <Alert className="mp-fans-sync-alert" type="info" showIcon title={t('正在读取粉丝同步状态')} />
  }
  if (!job) {
    return (
      <Alert
        className="mp-fans-sync-alert"
        type={refreshError ? 'error' : 'info'}
        showIcon
        title={refreshError || t('尚未同步粉丝资料')}
        description={refreshError ? t('请稍后刷新状态，启动同步前需先确认当前账号没有在途任务。') : t('点击“同步粉丝”后，任务会在后台分页拉取并更新本地粉丝资料。')}
      />
    )
  }

  const meta = syncStatusMeta[job.status]
  const progress = syncPercent(job)
  const updatedAt = job.finished_at || job.updated_at
  return (
    <Alert
      className="mp-fans-sync-alert"
      type={meta.alert}
      showIcon
      title={
        <div className="mp-fans-sync-title">
          <span>{accountName || t('公众号 #{{id}}', { id: job.account_id })}</span>
          <StatusPill tone={meta.tone} label={meta.label} pulse={job.status === 'running'} />
        </div>
      }
      description={
        <div className="mp-fans-sync-description">
          <div className="mp-fans-sync-summary">
            <div><span>{t('微信总数')}</span><strong>{job.total_count}</strong></div>
            <div><span>{t('已读取')}</span><strong>{job.listed_count}</strong></div>
            <div><span>{t('已写入')}</span><strong>{job.synced_count}</strong></div>
          </div>
          <Progress
            className="mp-fans-sync-progress"
            percent={progress}
            status={meta.progress}
            size="small"
          />
          <div className="mp-fans-sync-meta">
            <Text type="secondary">{t('最近更新 {{time}}', { time: formatDateTime(updatedAt) })}</Text>
            {job.attempts > 0 && <Text type="secondary">{t('已尝试 {{n}} 次', { n: job.attempts })}</Text>}
            {job.status === 'pending' && job.next_at && (
              <Text type="secondary">{t('下次执行 {{time}}', { time: formatDateTime(job.next_at) })}</Text>
            )}
          </div>
          {job.last_error && <Text type="danger" className="mp-fans-sync-error">{job.last_error}</Text>}
          {refreshError && (
            <Text type="danger" className="mp-fans-sync-error">
              {refreshError}{refreshRetryable ? t('，将自动重试') : t('，请手动刷新重试')}
            </Text>
          )}
        </div>
      }
    />
  )
}

export default function MpFansPage() {
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const canWrite = hasPerm('social:write')
  const [accounts, setAccounts] = useState<MpAccount[]>([])
  const [accountsLoading, setAccountsLoading] = useState(false)
  const [accountDetail, setAccountDetail] = useState<MpAccount | null>(null)
  const [accountDetailLoading, setAccountDetailLoading] = useState(false)
  const [syncJob, setSyncJob] = useState<MpFanSyncJob | null>(null)
  const [syncLoading, setSyncLoading] = useState(false)
  const [syncStarting, setSyncStarting] = useState(false)
  const [syncRefreshError, setSyncRefreshError] = useState('')
  const [syncRefreshRetryable, setSyncRefreshRetryable] = useState(false)
  const [syncCheckedAccountID, setSyncCheckedAccountID] = useState<number>()
  const [params, setParams] = useUrlParams<SearchParams>(
    { page: 1, page_size: 10 },
    ['page', 'page_size', 'account_id'],
  )
  const [listAccountID, setListAccountID] = useState(params.account_id)
  const [searchForm] = Form.useForm()
  const draftAccountID = Form.useWatch<number | undefined>('account_id', searchForm)
  const syncRequestSequence = useRef(0)
  const currentAccountID = useRef(params.account_id)
  const lastObservedJob = useRef<MpFanSyncJob | null>(null)
  const refreshedTerminalJob = useRef('')

  // latest-ref 写入放 effect：读取点全在订阅回调/请求回调里（render 中不读），
  // 提交后即生效；避免 render 期写 ref 在 StrictMode 弃用渲染下残留错值。
  useEffect(() => {
    currentAccountID.current = params.account_id
  })

  useEffect(() => {
    let cancelled = false
    setAccountsLoading(true)
    listMpAccounts({ page: 1, page_size: 100 })
      .then((res) => {
        if (!cancelled) setAccounts(res.list ?? [])
      })
      .catch(() => {
        if (!cancelled) {
          setAccounts([])
          message.error(t('获取公众号账号失败'))
        }
      })
      .finally(() => {
        if (!cancelled) setAccountsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [t])

  const listedSelectedAccount = useMemo(
    () => accounts.find((account) => account.id === params.account_id),
    [accounts, params.account_id],
  )

  useEffect(() => {
    const accountID = params.account_id
    if (!accountID || listedSelectedAccount || accountsLoading) {
      setAccountDetail(null)
      setAccountDetailLoading(false)
      return
    }
    let cancelled = false
    setAccountDetail(null)
    setAccountDetailLoading(true)
    getMpAccount(accountID, { silent: true })
      .then((result) => {
        if (!cancelled && result.id === accountID) setAccountDetail(result)
      })
      .catch(() => {
        if (!cancelled) setAccountDetail(null)
      })
      .finally(() => {
        if (!cancelled) setAccountDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [accountsLoading, listedSelectedAccount, params.account_id])

  const fetchList = useCallback(async (nextParams: SearchParams) => {
    try {
      const res = await listMpFans({ ...nextParams })
      setListAccountID(nextParams.account_id)
      return { list: res.list ?? [], total: res.total ?? 0 }
    } catch (error) {
      setListAccountID(nextParams.account_id)
      throw error
    }
  }, [])
  const onLoadError = useCallback(() => message.error(t('获取粉丝列表失败')), [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  useEffect(() => () => {
    syncRequestSequence.current += 1
  }, [])

  const observeSyncJob = useCallback((nextJob: MpFanSyncJob | null) => {
    if (nextJob && currentAccountID.current !== nextJob.account_id) return
    const previous = lastObservedJob.current
    lastObservedJob.current = nextJob
    setSyncJob(nextJob)

    if (!nextJob || !isTerminalSync(nextJob) || previous?.id !== nextJob.id || !isActiveSync(previous)) return
    const refreshKey = `${nextJob.id}:${nextJob.status}:${nextJob.updated_at}`
    if (refreshedTerminalJob.current === refreshKey) return
    refreshedTerminalJob.current = refreshKey
    if (nextJob.status === 'succeeded') {
      message.success(t('粉丝同步完成'))
    } else {
      message.error(t('粉丝同步失败，请查看任务状态'))
    }
    void reload()
  }, [reload, t])

  const fetchSyncJob = useCallback(async (
    accountID: number,
    showLoading = false,
  ): Promise<{ ok: true; job: MpFanSyncJob | null } | { ok: false; retryable: boolean }> => {
    const sequence = ++syncRequestSequence.current
    if (showLoading) setSyncLoading(true)
    try {
      const result = await getMpFanSync(accountID, { silent: true })
      if (sequence !== syncRequestSequence.current || currentAccountID.current !== accountID) {
        return { ok: false, retryable: false }
      }
      if (result.job && result.job.account_id !== accountID) {
        setSyncRefreshError(t('同步状态账号不匹配'))
        setSyncRefreshRetryable(false)
        return { ok: false, retryable: false }
      }
      setSyncRefreshError('')
      setSyncRefreshRetryable(false)
      observeSyncJob(result.job)
      setSyncCheckedAccountID(accountID)
      return { ok: true, job: result.job }
    } catch (error) {
      const retryable = isRetryableSyncStatusError(error)
      if (sequence === syncRequestSequence.current && currentAccountID.current === accountID) {
        setSyncRefreshError(t('同步状态刷新失败'))
        setSyncRefreshRetryable(retryable)
      }
      return { ok: false, retryable }
    } finally {
      if (showLoading && sequence === syncRequestSequence.current) setSyncLoading(false)
    }
  }, [observeSyncJob, t])

  useEffect(() => {
    const accountID = params.account_id
    syncRequestSequence.current += 1
    lastObservedJob.current = null
    setSyncJob(null)
    setSyncCheckedAccountID(undefined)
    setSyncRefreshError('')
    setSyncRefreshRetryable(false)
    setSyncStarting(false)
    if (!accountID) {
      setSyncLoading(false)
      return
    }
    void fetchSyncJob(accountID, true)
    return () => {
      syncRequestSequence.current += 1
    }
  }, [fetchSyncJob, params.account_id])

  const currentSyncJob = syncJob?.account_id === params.account_id ? syncJob : null
  const syncStateConfirmed = Boolean(params.account_id && syncCheckedAccountID === params.account_id)
  const activeSync = isActiveSync(currentSyncJob)

  useEffect(() => {
    const accountID = params.account_id
    if (!accountID || !activeSync) return
    let stopped = false
    let timer: number | undefined
    let retryDelay = 2000

    const schedule = (delay: number) => {
      timer = window.setTimeout(() => void poll(), delay)
    }

    const poll = async () => {
      const result = await fetchSyncJob(accountID)
      if (stopped) return
      if (result.ok) {
        retryDelay = 2000
        if (isActiveSync(result.job)) schedule(retryDelay)
        return
      }
      if (result.retryable) {
        schedule(retryDelay)
        retryDelay = Math.min(retryDelay * 2, 30_000)
      }
    }
    schedule(retryDelay)
    return () => {
      stopped = true
      if (timer !== undefined) window.clearTimeout(timer)
    }
  }, [activeSync, fetchSyncJob, params.account_id, syncJob?.id])

  const selectedAccount = listedSelectedAccount
    || (accountDetail?.id === params.account_id ? accountDetail : undefined)
  const accountChoices = selectedAccount && !listedSelectedAccount
    ? [selectedAccount, ...accounts]
    : accounts
  const syncDisabledReason = (() => {
    if (!params.account_id) return t('请先选择一个公众号账号并查询')
    if (draftAccountID !== params.account_id) return t('公众号筛选已改变，请先查询后再同步')
    if (accountsLoading || accountDetailLoading) return t('正在读取公众号账号')
    if (!selectedAccount) return t('公众号账号不存在或无权访问')
    if (selectedAccount.status !== 'active') return t('公众号账号已停用')
    if (syncLoading) return t('正在确认同步任务状态')
    if (syncRefreshError) return t('无法确认当前同步任务状态，请先刷新')
    if (!syncStateConfirmed) return t('正在确认同步任务状态')
    if (activeSync) return t('已有粉丝同步任务正在执行')
    return undefined
  })()

  const startSync = async () => {
    if (!canWrite || syncDisabledReason || !selectedAccount) return
    const accountID = selectedAccount.id
    const sequence = ++syncRequestSequence.current
    setSyncStarting(true)
    setSyncRefreshError('')
    try {
      const result = await startMpFanSync(accountID)
      if (sequence !== syncRequestSequence.current || currentAccountID.current !== accountID) return
      observeSyncJob(result.job)
      message.success(result.reused ? t('已有同步任务，已继续跟踪进度') : t('粉丝同步任务已创建'))
    } catch {
      // 全局请求层展示服务端返回的具体错误。
    } finally {
      if (sequence === syncRequestSequence.current) setSyncStarting(false)
    }
  }

  const refreshPage = () => {
    void reload()
    if (params.account_id) void fetchSyncJob(params.account_id, true)
  }

  const columns: ColumnsType<MpFan> = [
    {
      title: t('头像'), dataIndex: 'avatar', width: 70,
      render: (v: string) => <Avatar src={v || undefined} icon={<UserOutlined />} size={36} />,
    },
    { title: t('昵称'), dataIndex: 'nickname', width: 160, render: (v: string) => v || <span className="cell-muted">—</span> },
    { title: 'OpenID', dataIndex: 'open_id', width: 220, render: (v: string) => <span className="cell-mono">{v || '—'}</span> },
    { title: t('城市'), dataIndex: 'city', width: 120, render: (v: string) => v || <span className="cell-muted">—</span> },
    {
      title: t('标签'), dataIndex: 'tags', width: 190, ellipsis: true,
      render: (v: string) => {
        const tags = splitTags(v)
        return tags.length
          ? <Space size={4} wrap>{tags.map((tag) => <Tag key={tag} variant="filled">{tag}</Tag>)}</Space>
          : <span className="cell-muted">—</span>
      },
    },
    {
      title: t('是否关注'), dataIndex: 'subscribed', width: 100,
      render: (v: boolean) =>
        v ? <StatusPill tone="success" label="已关注" pulse={false} /> : <StatusPill tone="muted" label="已取关" />,
    },
    { title: t('关注时间'), dataIndex: 'subscribe_time', width: 165, className: 'cell-time', render: formatDateTime },
  ]

  const syncButtonText = activeSync ? t('同步中') : currentSyncJob ? t('重新同步') : t('同步粉丝')
  const listMatchesAccount = listAccountID === params.account_id
  const visibleList = listMatchesAccount ? list : []
  const visibleTotal = listMatchesAccount ? total : 0

  return (
    <ListPageShell
      className="mp-fans-page"
      filter={
        <ListFilterForm
          form={searchForm}
          initialValues={params}
          onFinish={(values) => setParams({ ...params, page: 1, ...values })}
        >
          <Form.Item name="account_id">
            <Select
              placeholder={t('全部账号')}
              style={{ width: 200 }}
              allowClear
              showSearch
              loading={accountsLoading}
              optionFilterProp="label"
              options={accountChoices.map((a) => ({ value: a.id, label: a.name || a.app_id }))}
            />
          </Form.Item>
          <Form.Item name="keyword">
            <Input placeholder={t('昵称 / OpenID')} prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={() => { searchForm.setFieldsValue({ account_id: undefined, keyword: undefined }); setParams({ page: 1, page_size: 10 }) }}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      }
      toolbar={
        <TableToolbar
          title="粉丝管理"
          total={visibleTotal}
          extra={
            <Space wrap className="mp-fans-toolbar-actions">
              {canWrite && (
                <Tooltip title={syncDisabledReason}>
                  <span>
                    <Button
                      type="primary"
                      icon={<SyncOutlined spin={activeSync} />}
                      loading={syncStarting}
                      disabled={Boolean(syncDisabledReason) || syncStarting}
                      onClick={() => void startSync()}
                    >
                      {syncButtonText}
                    </Button>
                  </span>
                </Tooltip>
              )}
              <Button icon={<ReloadOutlined />} loading={loading || syncLoading} onClick={refreshPage}>{t('刷新')}</Button>
            </Space>
          }
        />}
      >
        {params.account_id && (
          <SyncStatus
            accountName={selectedAccount?.name || selectedAccount?.app_id}
            job={currentSyncJob}
            loading={syncLoading || (!syncStateConfirmed && !syncRefreshError)}
            refreshError={syncRefreshError}
            refreshRetryable={syncRefreshRetryable && activeSync}
          />
        )}
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={visibleList}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无粉丝资料，可选择公众号后启动后台同步" /> }}
          scroll={{ x: 1040 }}
          pagination={{
            total: visibleTotal,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
    </ListPageShell>
  )
}
