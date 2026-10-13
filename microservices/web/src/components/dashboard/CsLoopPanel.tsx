import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Row, Skeleton, Tag, Tooltip } from 'antd'
import {
  ArrowRightOutlined,
  BellOutlined,
  CustomerServiceOutlined,
  FileTextOutlined,
  ReloadOutlined,
  StarOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { getAgentQueue, getImStats, listLowRatings } from '@/api/im'
import { getTicketStats, type TicketStats } from '@/api/ticket'
import { getCrmStats } from '@/api/crm'
import { getNotifyUnreadCount } from '@/api/notify'
import CountUpValue from '@/components/common/CountUpValue'

type LoopMetric = {
  key: string
  title: string
  value: number | null
  hint?: string
  danger?: boolean
  path: string
  icon: React.ReactNode
  gradient: string
  tint: string
}

/**
 * 客服闭环一览：排队 / 工单 / 超时 / 低分 / 回访 / 站内信。
 * 各域 API 独立拉取，失败静默，不挡仪表盘主链路。
 */
export default function CsLoopPanel() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [queueSize, setQueueSize] = useState<number | null>(null)
  const [onlineAgents, setOnlineAgents] = useState<number | null>(null)
  const [ticket, setTicket] = useState<TicketStats | null>(null)
  const [followupsOverdue, setFollowupsOverdue] = useState<number | null>(null)
  const [inboxUnread, setInboxUnread] = useState<number | null>(null)
  const [lowRatings, setLowRatings] = useState<number | null>(null)
  const [avgRating, setAvgRating] = useState<number | null>(null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    const tasks = [
      getAgentQueue()
        .then((r) => {
          if (cancelled) return
          setQueueSize(Number(r?.queue_size ?? r?.list?.length ?? 0))
          setOnlineAgents(Array.isArray(r?.online_agents) ? r.online_agents.length : null)
        })
        .catch(() => {
          if (!cancelled) {
            setQueueSize(null)
            setOnlineAgents(null)
          }
        }),
      getTicketStats()
        .then((r) => {
          if (!cancelled) setTicket(r)
        })
        .catch(() => {
          if (!cancelled) setTicket(null)
        }),
      getCrmStats(30)
        .then((r) => {
          if (!cancelled) setFollowupsOverdue(Number(r?.stats?.followups_overdue ?? 0))
        })
        .catch(() => {
          if (!cancelled) setFollowupsOverdue(null)
        }),
      getNotifyUnreadCount()
        .then((r) => {
          if (!cancelled) setInboxUnread(Number(r?.count ?? 0))
        })
        .catch(() => {
          if (!cancelled) setInboxUnread(null)
        }),
      listLowRatings({ max_score: 3, days: 30, page: 1, page_size: 1 })
        .then((r) => {
          if (!cancelled) setLowRatings(Number(r?.total ?? 0))
        })
        .catch(() => {
          if (!cancelled) setLowRatings(null)
        }),
      getImStats(30)
        .then((r) => {
          if (cancelled) return
          const avg = r?.stats?.avg_rating
          setAvgRating(typeof avg === 'number' ? avg : null)
        })
        .catch(() => {
          if (!cancelled) setAvgRating(null)
        }),
    ]
    Promise.allSettled(tasks).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => {
      cancelled = true
    }
  }, [tick])

  const metrics: LoopMetric[] = [
    {
      key: 'queue',
      title: t('IM 排队'),
      value: queueSize,
      hint: onlineAgents != null ? t('在线坐席 {{n}}', { n: onlineAgents }) : t('待接入会话'),
      danger: (queueSize ?? 0) > 0,
      path: '/im/desk',
      icon: <CustomerServiceOutlined />,
      gradient: 'linear-gradient(135deg, #34d399, #059669)',
      tint: 'rgba(16, 185, 129, 0.12)',
    },
    {
      key: 'mine_open',
      title: t('我的工单'),
      value: ticket?.mine_open ?? null,
      hint: t('待办未结'),
      path: '/ticket?scope=mine',
      icon: <FileTextOutlined />,
      gradient: 'linear-gradient(135deg, #60a5fa, #2563eb)',
      tint: 'rgba(37, 99, 235, 0.12)',
    },
    {
      key: 'mine_overdue',
      title: t('我的超时'),
      value: ticket?.mine_overdue ?? null,
      hint: t('全局超时 {{n}}', { n: ticket?.overdue ?? '—' }),
      danger: (ticket?.mine_overdue ?? 0) > 0,
      path: '/ticket?scope=mine&overdue=1',
      icon: <FileTextOutlined />,
      gradient: 'linear-gradient(135deg, #f87171, #dc2626)',
      tint: 'rgba(220, 38, 38, 0.12)',
    },
    {
      key: 'low_rating',
      title: t('低分会话'),
      value: lowRatings,
      hint: avgRating != null ? t('近30天均分 {{n}}', { n: avgRating.toFixed(1) }) : t('CSAT ≤3 星'),
      danger: (lowRatings ?? 0) > 0,
      path: '/im/low-ratings',
      icon: <StarOutlined />,
      gradient: 'linear-gradient(135deg, #fbbf24, #d97706)',
      tint: 'rgba(217, 119, 6, 0.12)',
    },
    {
      key: 'followup',
      title: t('回访到期'),
      value: followupsOverdue,
      hint: t('CRM next_at 已过'),
      danger: (followupsOverdue ?? 0) > 0,
      path: '/crm/customers?scope=revisit',
      icon: <TeamOutlined />,
      gradient: 'linear-gradient(135deg, #fb923c, #ea580c)',
      tint: 'rgba(234, 88, 12, 0.12)',
    },
    {
      key: 'inbox',
      title: t('站内信未读'),
      value: inboxUnread,
      hint: t('工单 / IM / 回访提醒'),
      danger: (inboxUnread ?? 0) > 0,
      path: '/system/inbox',
      icon: <BellOutlined />,
      gradient: 'linear-gradient(135deg, #a78bfa, #7c3aed)',
      tint: 'rgba(124, 58, 237, 0.12)',
    },
  ]

  const alertCount = metrics.filter((m) => m.danger && (m.value ?? 0) > 0).length

  return (
    <Card
      className="liquid-dash-panel is-alive cs-loop-panel"
      style={{ marginTop: 20 }}
      title={
        <span>
          <CustomerServiceOutlined className="card-title-icon" />
          {t('客服闭环')}
          {alertCount > 0 ? (
            <Tag color="red" style={{ marginInlineStart: 8 }}>
              {t('{{n}} 项待处理', { n: alertCount })}
            </Tag>
          ) : (
            <Tag color="green" style={{ marginInlineStart: 8 }}>
              {t('正常')}
            </Tag>
          )}
        </span>
      }
      extra={
        <Button
          type="text"
          size="small"
          icon={<ReloadOutlined />}
          onClick={() => setTick((n) => n + 1)}
        >
          {t('刷新')}
        </Button>
      }
    >
      <div className="cs-loop-desc">
        {t('排队 → 工单 SLA → 低分质检 → CRM 回访 → 站内信，一点进入对应工作台。')}
      </div>
      <Row gutter={[16, 16]}>
        {metrics.map((m, i) => (
          <Col xs={12} sm={12} md={8} lg={4} xl={4} key={m.key}>
            <Tooltip title={m.hint || m.title}>
              <Card
                size="small"
                className={`cs-loop-metric${m.danger ? ' is-alert' : ''}`}
                hoverable
                styles={{ body: { padding: 14 } }}
                style={{ '--tint': m.tint, '--i': i } as React.CSSProperties}
                onClick={() => navigate(m.path)}
              >
                <div className="cs-loop-metric-head">
                  <span
                    className="cs-loop-metric-icon"
                    style={{ background: m.gradient }}
                  >
                    {m.icon}
                  </span>
                  <span className="cs-loop-metric-title">{m.title}</span>
                </div>
                <div className="cs-loop-metric-value">
                  {loading || m.value == null ? (
                    <Skeleton.Button active size="small" style={{ width: 48, height: 28 }} />
                  ) : (
                    <CountUpValue value={m.value} />
                  )}
                </div>
                <div className="cs-loop-metric-foot">
                  {m.hint || t('查看')} <ArrowRightOutlined />
                </div>
              </Card>
            </Tooltip>
          </Col>
        ))}
      </Row>
    </Card>
  )
}
