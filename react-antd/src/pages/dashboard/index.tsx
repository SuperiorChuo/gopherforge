import { useEffect, useState } from 'react'
import { Card, Row, Col, Skeleton, Tag, Tooltip, Empty, Button, Space, Segmented } from 'antd'
import {
  UserOutlined,
  TeamOutlined,
  SafetyOutlined,
  MenuOutlined,
  ArrowRightOutlined,
  WifiOutlined,
  SoundOutlined,
  LineChartOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useAppSelector } from '@/hooks/store'
import { getUserList } from '@/api/system/user'
import { getRoleList } from '@/api/system/role'
import { getPermissionList } from '@/api/system/permission'
import { getMenuList } from '@/api/system/menu'
import { getLoginTrend, getLastLogin, type LoginTrendItem } from '@/api/system/log'
import type { LoginLog } from '@/types'
import { getOnlineUserCount } from '@/api/system/online-user'
import { getActiveNotices } from '@/api/system/notice'
import type { Notice } from '@/types'
import { usePermission } from '@/hooks/usePermission'
import { useCountUp } from '@/hooks/useCountUp'
import dayjs from 'dayjs'

function CountUpValue({ value }: { value: number }) {
  const display = useCountUp(value)
  return <>{display.toLocaleString()}</>
}

interface StatCard {
  key: string
  title: string
  value: number
  icon: React.ReactNode
  gradient: string
  shadow: string
  // 强调色以极低透明度从卡片右上角"照进"玻璃(--tint)
  tint: string
  path: string
}

const noticeTypeLabels: Record<number, string> = { 1: '通知', 2: '公告' }

export default function DashboardPage() {
  const navigate = useNavigate()
  const { userInfo } = useAppSelector((s) => s.auth)
  const { hasPerm } = usePermission()
  const [stats, setStats] = useState({ users: 0, roles: 0, permissions: 0, menus: 0 })
  const [loading, setLoading] = useState(true)
  const [trend, setTrend] = useState<LoginTrendItem[] | null>(null)
  const [trendDays, setTrendDays] = useState(7)
  const [onlineCount, setOnlineCount] = useState<number | null>(null)
  const [notices, setNotices] = useState<Notice[] | null>(null)
  const [lastLogin, setLastLogin] = useState<LoginLog | null>(null)

  const canUsers = hasPerm('system:user:list')
  const canRoles = hasPerm('system:role:list')
  const canPerms = hasPerm('system:permission:list')
  const canMenus = hasPerm('system:menu:list')
  const canTrend = hasPerm('system:log:login')
  const canOnline = hasPerm('system:online-user:list')

  useEffect(() => {
    // 只请求有权限的模块，无权限的卡片直接不展示
    const tasks: Promise<unknown>[] = []
    if (canUsers) tasks.push(getUserList({ page: 1, page_size: 1 }).then((r) => setStats((s) => ({ ...s, users: r.total }))))
    if (canRoles) tasks.push(getRoleList({ page: 1, page_size: 1 }).then((r) => setStats((s) => ({ ...s, roles: r.total }))))
    if (canPerms) tasks.push(getPermissionList({ page: 1, page_size: 1 }).then((r) => setStats((s) => ({ ...s, permissions: r.total }))))
    if (canMenus) tasks.push(getMenuList({ page: 1, page_size: 1 }).then((r) => setStats((s) => ({ ...s, menus: r.total }))))
    Promise.allSettled(tasks).finally(() => setLoading(false))

    if (!canTrend) {
      setTrend([])
    }
    if (canOnline) {
      getOnlineUserCount().then(setOnlineCount).catch(() => setOnlineCount(null))
    }
    getActiveNotices().then(setNotices).catch(() => setNotices([]))
    getLastLogin().then(setLastLogin).catch(() => setLastLogin(null))
    // 权限在进入布局前已加载完成，挂载时值即最终值
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 趋势图随天数切换单独拉取
  useEffect(() => {
    if (!canTrend) return
    getLoginTrend(trendDays).then(setTrend).catch(() => setTrend([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trendDays])

  const now = new Date()
  const hour = now.getHours()
  const greeting =
    hour < 6 ? '凌晨好' : hour < 12 ? '上午好' : hour < 14 ? '中午好' : hour < 18 ? '下午好' : '晚上好'

  const allCards: Array<StatCard & { visible: boolean }> = [
    {
      key: 'users',
      visible: canUsers,
      title: '总用户数',
      value: stats.users,
      icon: <UserOutlined />,
      gradient: 'linear-gradient(135deg, #6366f1, #4f46e5)',
      shadow: 'rgba(79, 70, 229, 0.35)',
      tint: 'rgba(99, 102, 241, 0.14)',
      path: '/system/user',
    },
    {
      key: 'roles',
      visible: canRoles,
      title: '角色数量',
      value: stats.roles,
      icon: <TeamOutlined />,
      gradient: 'linear-gradient(135deg, #34d399, #059669)',
      shadow: 'rgba(5, 150, 105, 0.35)',
      tint: 'rgba(16, 185, 129, 0.12)',
      path: '/system/role',
    },
    {
      key: 'permissions',
      visible: canPerms,
      title: '权限数量',
      value: stats.permissions,
      icon: <SafetyOutlined />,
      gradient: 'linear-gradient(135deg, #fbbf24, #f59e0b)',
      shadow: 'rgba(245, 158, 11, 0.35)',
      tint: 'rgba(245, 158, 11, 0.11)',
      path: '/system/permission',
    },
    {
      key: 'menus',
      visible: canMenus,
      title: '菜单数量',
      value: stats.menus,
      icon: <MenuOutlined />,
      gradient: 'linear-gradient(135deg, #f472b6, #db2777)',
      shadow: 'rgba(219, 39, 119, 0.35)',
      tint: 'rgba(219, 39, 119, 0.11)',
      path: '/system/menu',
    },
  ]

  const trendData = trend ?? []
  const maxCount = Math.max(...trendData.map((t) => t.count), 1)
  const hasTrendData = trendData.some((t) => t.count > 0)

  return (
    <div>
      <div className="dash-hero">
        <div className="dash-hero-content">
          <div className="dash-hero-greeting">
            {greeting}，<em>{userInfo?.nickname || userInfo?.username}</em> 👋
          </div>
          <div className="dash-hero-sub">欢迎回到 Go Admin Kit 管理后台，祝您工作顺利。</div>
          {lastLogin?.created_at && (
            <div className="dash-hero-meta">
              上次登录 {dayjs(lastLogin.created_at).format('YYYY-MM-DD HH:mm')}
              {lastLogin.ip ? ` · ${lastLogin.ip}` : ''}
            </div>
          )}
        </div>
        <div className="dash-hero-glow" />
      </div>

      <Row gutter={[20, 20]}>
        {allCards.filter((c) => c.visible).map((c, i) => (
          <Col xs={24} sm={12} lg={6} key={c.key}>
            <Card
              className="stat-card glass-rise"
              hoverable
              styles={{ body: { padding: 20 } }}
              style={{ '--tint': c.tint, '--i': i } as React.CSSProperties}
              onClick={() => navigate(c.path)}
            >
              <div className="stat-card-row">
                <div>
                  <div className="stat-card-title">{c.title}</div>
                  {loading ? (
                    <Skeleton.Button active size="large" style={{ width: 72, height: 36, marginTop: 6 }} />
                  ) : (
                    <div className="stat-card-value"><CountUpValue value={c.value} /></div>
                  )}
                </div>
                <div
                  className="stat-card-icon"
                  style={{ background: c.gradient, '--icon-shadow': c.shadow } as React.CSSProperties}
                >
                  {c.icon}
                </div>
              </div>
              <div className="stat-card-foot">
                查看详情 <ArrowRightOutlined />
              </div>
            </Card>
          </Col>
        ))}
      </Row>

      <Row gutter={[20, 20]} style={{ marginTop: 20 }}>
        {canTrend && (
        <Col xs={24} lg={16}>
          <Card
            title={
              <span>
                <LineChartOutlined className="card-title-icon" />
                近 {trendDays} 天登录趋势
              </span>
            }
            extra={
              <Space size={16}>
                <Segmented
                  size="small"
                  value={trendDays}
                  onChange={(v) => setTrendDays(v as number)}
                  options={[
                    { label: '7天', value: 7 },
                    { label: '15天', value: 15 },
                    { label: '30天', value: 30 },
                  ]}
                />
                <div className="trend-legend">
                  <span>
                    <span className="trend-legend-dot trend-legend-dot-success" />
                    成功
                  </span>
                  <span>
                    <span className="trend-legend-dot trend-legend-dot-failed" />
                    失败
                  </span>
                </div>
              </Space>
            }
            style={{ height: '100%' }}
          >
            {trend === null ? (
              <Skeleton active paragraph={{ rows: 5 }} />
            ) : !hasTrendData ? (
              <Empty description="暂无登录数据" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '48px 0' }} />
            ) : (
              <div className="trend-chart" style={{ gap: trendDays > 15 ? 4 : trendDays > 7 ? 8 : 12 }}>
                {trendData.map((t, i) => {
                  // 柱子多时日期隔行展示，避免标签互相压住
                  const labelEvery = trendDays > 15 ? 4 : trendDays > 7 ? 2 : 1
                  return (
                    <div className="trend-col" key={t.date}>
                      <div className="trend-count">{trendDays <= 7 && t.count > 0 ? t.count : ''}</div>
                      <Tooltip
                        title={
                          <>
                            {t.date}
                            <br />
                            成功 {t.success} 次 · 失败 {t.failed} 次
                          </>
                        }
                      >
                        <div className="trend-bar-area">
                          {t.failed > 0 && (
                            <div
                              className="trend-bar-failed"
                              style={{ height: `${(t.failed / maxCount) * 100}%` }}
                            />
                          )}
                          <div
                            className="trend-bar-success"
                            style={{ height: `${(t.success / maxCount) * 100}%` }}
                          />
                        </div>
                      </Tooltip>
                      <div className="trend-date">
                        {i % labelEvery === 0 ? dayjs(t.date).format('MM-DD') : ''}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </Card>
        </Col>
        )}

        <Col xs={24} lg={canTrend ? 8 : 24}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20, height: '100%' }}>
            {canOnline && (
            <Card
              className="stat-card"
              hoverable
              onClick={() => navigate('/system/online-user')}
              styles={{ body: { padding: 20 } }}
              style={{ '--tint': 'rgba(14, 165, 233, 0.13)' } as React.CSSProperties}
            >
              <div className="stat-card-row">
                <div>
                  <div className="stat-card-title">
                    <span className="live-dot" />
                    当前在线用户
                  </div>
                  <div className="dash-online-num" style={{ marginTop: 8 }}>
                    {onlineCount === null ? '-' : <CountUpValue value={onlineCount} />}
                  </div>
                </div>
                <div
                  className="stat-card-icon"
                  style={{ background: 'linear-gradient(135deg, #38bdf8, #0284c7)', '--icon-shadow': 'rgba(2,132,199,0.35)' } as React.CSSProperties}
                >
                  <WifiOutlined />
                </div>
              </div>
            </Card>
            )}

            <Card
              title={
                <span>
                  <SoundOutlined className="card-title-icon-warn" />
                  系统公告
                </span>
              }
              extra={
                <Button type="link" size="small" onClick={() => navigate('/system/notice')}>
                  更多
                </Button>
              }
              style={{ flex: 1 }}
              styles={{ body: { padding: '8px 16px' } }}
            >
              {notices === null ? (
                <Skeleton active paragraph={{ rows: 3 }} />
              ) : notices.length === 0 ? (
                <Empty description="暂无公告" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: '24px 0' }} />
              ) : (
                notices.slice(0, 5).map((n) => (
                  <div className="dash-notice-item" key={n.id}>
                    <Tag
                      color={n.type === 2 ? 'orange' : 'blue'}
                      variant="filled"
                      style={{ marginInlineEnd: 0, flexShrink: 0 }}
                    >
                      {noticeTypeLabels[n.type] ?? '通知'}
                    </Tag>
                    <Tooltip title={n.title}>
                      <span className="dash-notice-title">{n.title}</span>
                    </Tooltip>
                    <span className="dash-notice-time">
                      {n.created_at ? dayjs(n.created_at).format('MM-DD') : ''}
                    </span>
                  </div>
                ))
              )}
            </Card>
          </div>
        </Col>
      </Row>
    </div>
  )
}
