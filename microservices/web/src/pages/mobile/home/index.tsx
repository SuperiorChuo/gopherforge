import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { listTodoTasks } from '@/api/bpm'
import { getAlertSummary, getServicesHealth, getTaskRunSummary } from '@/api/monitor'
// 下游无 notify 服务面：未读站内信卡与 inbox 入口不在脚手架形态内。

import { getLastLogin } from '@/api/system/log'
import { getOnlineUserCount } from '@/api/system/online-user'
import { useAppSelector } from '@/hooks/store'
import { usePermission } from '@/hooks/usePermission'
import type { LoginLog } from '@/types'
import { openDesktopConsole } from '../open-desktop'

type Tone = 'ok' | 'warn' | 'err'
type MetricState =
  | { kind: 'hidden' }
  | { kind: 'loading' }
  | { kind: 'ready'; value: string; hint?: string; tone?: Tone }
  | { kind: 'error' }

type HomeState = {
  health: MetricState
  online: MetricState
  todos: MetricState
  alerts: MetricState
  tasks: MetricState
}

const hidden: MetricState = { kind: 'hidden' }
const loading: MetricState = { kind: 'loading' }

function greetingByHour(hour: number) {
  if (hour < 6) return '凌晨好'
  if (hour < 12) return '上午好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
}

function MetricCard({
  label,
  state,
  onClick,
}: {
  label: string
  state: MetricState
  onClick?: () => void
}) {
  const { t } = useTranslation()
  if (state.kind === 'hidden') return null
  const valueClass =
    state.kind === 'ready' && state.tone ? `m-card-value is-${state.tone}` : 'm-card-value'
  const inner = (
    <>
      <span className="m-card-label">{t(label)}</span>
      {state.kind === 'loading' ? (
        <span className="m-skel" />
      ) : (
        <span className={state.kind === 'error' ? 'm-card-value is-muted' : valueClass}>
          {state.kind === 'ready' ? state.value : '--'}
        </span>
      )}
      <span className="m-card-hint">
        {state.kind === 'ready' ? t(state.hint ?? '实时数据') : state.kind === 'loading' ? t('加载中…') : t('暂时不可用')}
      </span>
    </>
  )
  if (onClick) {
    return (
      <button type="button" className="m-card" onClick={onClick}>
        {inner}
      </button>
    )
  }
  return <article className="m-card">{inner}</article>
}

export default function MobileHomePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const userInfo = useAppSelector((s) => s.auth.userInfo)
  const { hasPerm, isSuperAdmin } = usePermission()
  const [lastLogin, setLastLogin] = useState<LoginLog | null>(null)
  const [state, setState] = useState<HomeState>({
    health: hidden,
    online: hidden,
    todos: loading,
    alerts: hidden,
    tasks: hidden,
  })

  const load = useCallback(() => {
    const canHealth = hasPerm('system:monitor:server')
    const canOnline = hasPerm('system:online-user:list')
    const canAlerts = hasPerm('system:alert:list')
    const canTasks = hasPerm('system:job:list')

    setState({
      health: canHealth ? loading : hidden,
      online: canOnline ? loading : hidden,
      todos: loading,
      alerts: canAlerts ? loading : hidden,
      tasks: canTasks ? loading : hidden,
    })

    if (canHealth) {
      getServicesHealth()
        .then((res) => {
          const total = res.total ?? res.list?.length ?? 0
          const healthy = res.healthy ?? res.list?.filter((row) => row.ok).length ?? 0
          const allOk = total > 0 && healthy === total
          setState((s) => ({
            ...s,
            health: {
              kind: 'ready',
              value: total ? `${healthy}/${total}` : '0/0',
              hint: allOk ? '全部健康' : '存在异常服务',
              tone: allOk ? 'ok' : 'err',
            },
          }))
        })
        .catch(() => setState((s) => ({ ...s, health: { kind: 'error' } })))
    }

    if (canOnline) {
      getOnlineUserCount()
        .then((count) =>
          setState((s) => ({
            ...s,
            online: { kind: 'ready', value: String(count), hint: '当前在线会话' },
          })),
        )
        .catch(() => setState((s) => ({ ...s, online: { kind: 'error' } })))
    }

    listTodoTasks({ page: 1, page_size: 1 }, true)
      .then((res) => {
        const count = Number(res?.total ?? 0)
        setState((s) => ({
          ...s,
          todos: {
            kind: 'ready',
            value: String(count),
            hint: count ? '待你处理' : '没有待办',
            tone: count ? 'warn' : undefined,
          },
        }))
      })
      .catch(() => setState((s) => ({ ...s, todos: { kind: 'error' } })))

    if (canAlerts) {
      getAlertSummary()
        .then((res) => {
          const firing = Number(res.firing ?? 0)
          setState((s) => ({
            ...s,
            alerts: {
              kind: 'ready',
              value: String(firing),
              hint: firing ? '正在告警' : '当前无告警',
              tone: firing ? 'err' : 'ok',
            },
          }))
        })
        .catch(() => setState((s) => ({ ...s, alerts: { kind: 'error' } })))
    }

    if (canTasks) {
      getTaskRunSummary(24)
        .then((res) => {
          const failed = Number(res.failed ?? 0)
          setState((s) => ({
            ...s,
            tasks: {
              kind: 'ready',
              value: String(failed),
              hint: failed ? '近 24 小时失败' : '近 24 小时无失败',
              tone: failed ? 'err' : 'ok',
            },
          }))
        })
        .catch(() => setState((s) => ({ ...s, tasks: { kind: 'error' } })))
    }

    getLastLogin()
      .then(setLastLogin)
      .catch(() => setLastLogin(null))
  }, [hasPerm])

  useEffect(() => {
    load()
  }, [load])

  const hour = new Date().getHours()
  const name = isSuperAdmin ? t('管理员') : userInfo?.nickname || userInfo?.username || ''

  return (
    <main className="m-home">
      <section>
        <h2 className="m-hello">
          {t(greetingByHour(hour))}, <em>{name}</em>
        </h2>
        <p className="m-sub">{t('手机上看系统，处理急事。复杂配置请回电脑。')}</p>
        <div className="m-chip-row">
          {lastLogin?.created_at && (
            <span className="m-chip">
              {t('上次登录 {{time}}', { time: dayjs(lastLogin.created_at).format('MM-DD HH:mm') })}
              {lastLogin.ip ? ` · ${lastLogin.ip}` : ''}
            </span>
          )}
        </div>
      </section>

      <section className="m-grid" aria-live="polite">
        <MetricCard label="服务健康" state={state.health} onClick={() => navigate('/m/ops/health')} />
        <MetricCard label="在线用户" state={state.online} onClick={() => navigate('/m/ops/online')} />
        <MetricCard label="待办审批" state={state.todos} onClick={() => navigate('/m/tasks')} />
        <MetricCard label="告警" state={state.alerts} onClick={() => navigate('/m/ops/alerts')} />
        <MetricCard label="失败任务" state={state.tasks} onClick={() => navigate('/m/ops/jobs')} />
      </section>

      <div className="m-footer-links">
        <button type="button" className="m-link-btn" onClick={load}>
          <ReloadOutlined /> {t('刷新态势')}
        </button>
        <button type="button" className="m-link-btn" onClick={() => openDesktopConsole()}>
          {t('打开完整控制台')}
        </button>
      </div>
    </main>
  )
}
