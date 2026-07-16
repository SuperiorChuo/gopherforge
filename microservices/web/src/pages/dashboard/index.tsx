import { useEffect, useState } from 'react'
import { Card, Row, Col, Skeleton, Tag, Tooltip, Button, Space } from 'antd'
import {
  UserOutlined,
  TeamOutlined,
  SafetyOutlined,
  MenuOutlined,
  ArrowRightOutlined,
  WifiOutlined,
  SoundOutlined,
  LineChartOutlined,
  CalendarOutlined,
  HistoryOutlined,
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
import { getLiveWeather, type LiveWeather } from '@/api/system/weather'
import type { Notice } from '@/types'
import { usePermission } from '@/hooks/usePermission'
import GlassEmpty from '@/components/GlassEmpty'
import CountUpValue from '@/components/CountUpValue'
import dayjs from 'dayjs'

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

// 高德天气现象文本 → 表情（按关键字匹配，未命中回退 🌤️；夜间晴/多云换月亮）
function weatherEmoji(text: string, hour: number): string {
  const night = hour >= 19 || hour < 6
  if (/雷/.test(text)) return '⛈️'
  if (/雪/.test(text)) return '❄️'
  if (/雨/.test(text)) return '🌧️'
  if (/雾|霾|浮尘|扬沙|沙尘/.test(text)) return '🌫️'
  if (/阴/.test(text)) return '☁️'
  if (/云/.test(text)) return night ? '☁️' : '🌤️'
  if (/晴/.test(text)) return night ? '🌙' : '☀️'
  return '🌤️'
}

const WEATHER_CACHE_KEY = 'dash-weather-cache'
const WEATHER_CACHE_TTL = 30 * 60 * 1000

// localStorage 缓存：页面往返 dashboard 不重复请求；失败静默（返回 null 则不渲染 chip）
async function loadWeather(): Promise<LiveWeather | null> {
  try {
    const cached = localStorage.getItem(WEATHER_CACHE_KEY)
    if (cached) {
      const { data, ts } = JSON.parse(cached) as { data: LiveWeather; ts: number }
      if (Date.now() - ts < WEATHER_CACHE_TTL && data?.weather) return data
    }
  } catch { /* 缓存损坏视为无缓存 */ }
  try {
    const data = await getLiveWeather()
    if (!data?.weather) return null
    localStorage.setItem(WEATHER_CACHE_KEY, JSON.stringify({ data, ts: Date.now() }))
    return data
  } catch {
    return null
  }
}

export default function DashboardPage() {
  const navigate = useNavigate()
  const { userInfo } = useAppSelector((s) => s.auth)
  const { hasPerm } = usePermission()
  const [stats, setStats] = useState({ users: 0, roles: 0, permissions: 0, menus: 0 })
  const [loading, setLoading] = useState(true)
  const [trend, setTrend] = useState<LoginTrendItem[] | null>(null)
  const [trendDays, setTrendDays] = useState(7)
  const [trendFetching, setTrendFetching] = useState(false)
  const [onlineCount, setOnlineCount] = useState<number | null>(null)
  const [notices, setNotices] = useState<Notice[] | null>(null)
  const [lastLogin, setLastLogin] = useState<LoginLog | null>(null)
  const [weather, setWeather] = useState<LiveWeather | null>(null)

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
    loadWeather().then(setWeather)
    // 权限在进入布局前已加载完成，挂载时值即最终值
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 趋势图随天数切换单独拉取;切换期间保留旧图并雾化,数据到位后一次成型
  useEffect(() => {
    if (!canTrend) return
    setTrendFetching(true)
    getLoginTrend(trendDays)
      .then(setTrend)
      .catch(() => setTrend([]))
      .finally(() => setTrendFetching(false))
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
    <div className="dash-page">
      <div className="dash-hero liquid-dash is-alive">
        <div className="liquid-sheen" aria-hidden="true">
          <i />
          <i />
        </div>
        <div className="dash-hero-content">
          <div className="dash-hero-greeting">
            {greeting}，<em>{userInfo?.nickname || userInfo?.username}</em>
          </div>
          <div className="dash-hero-sub">欢迎回到 Go Admin Kit · 以工程之美，驱动今日工作</div>
          <div className="dash-hero-chips">
            <span className="hero-chip">
              <CalendarOutlined />
              {dayjs().format('YYYY年M月D日')} · {['周日', '周一', '周二', '周三', '周四', '周五', '周六'][dayjs().day()]}
            </span>
            {weather?.weather && (
              <span className="hero-chip">
                <span aria-hidden>{weatherEmoji(weather.weather, hour)}</span>
                {weather.city ? `${weather.city} · ` : ''}
                {weather.weather}
                {weather.temperature != null && weather.temperature !== ''
                  ? ` ${weather.temperature}°C`
                  : ''}
              </span>
            )}
            {lastLogin?.created_at && (
              <span className="hero-chip">
                <HistoryOutlined />
                上次登录 {dayjs(lastLogin.created_at).format('MM-DD HH:mm')}
                {lastLogin.ip ? ` · ${lastLogin.ip}` : ''}
              </span>
            )}
          </div>
        </div>
        {/* 轨道装饰:两圈细环 + 沿环缓慢公转的光点 */}
        <div className="dash-hero-orbit" aria-hidden>
          <span className="orbit-ring orbit-ring-1"><span className="orbit-dot" /></span>
          <span className="orbit-ring orbit-ring-2"><span className="orbit-dot orbit-dot-2" /></span>
        </div>
        <div className="dash-hero-glow" />
      </div>

      <Row gutter={[20, 20]}>
        {allCards.filter((c) => c.visible).map((c, i) => (
          <Col xs={24} sm={12} lg={6} key={c.key}>
            <Card
              className="stat-card glass-rise liquid-dash-stat is-alive"
              hoverable
              styles={{ body: { padding: 20 } }}
              style={{ '--tint': c.tint, '--i': i } as React.CSSProperties}
              onClick={() => navigate(c.path)}
            >
              <div className="liquid-sheen" aria-hidden="true">
                <i />
                <i />
              </div>
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
            className="liquid-dash-panel is-alive"
            title={
              <span>
                <LineChartOutlined className="card-title-icon" />
                近 {trendDays} 天登录趋势
              </span>
            }
            extra={
              <Space size={12} wrap className="trend-card-extra">
                <div className="trend-days" role="group" aria-label="登录趋势天数">
                  {([7, 15, 30] as const).map((d) => (
                    <button
                      key={d}
                      type="button"
                      className={`trend-days-btn${trendDays === d ? ' is-active' : ''}`}
                      aria-pressed={trendDays === d}
                      onClick={() => setTrendDays(d)}
                    >
                      {d} 天
                    </button>
                  ))}
                </div>
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
            <div className="liquid-sheen" aria-hidden="true">
              <i />
              <i />
            </div>
            {trend === null ? (
              <Skeleton active paragraph={{ rows: 5 }} />
            ) : !hasTrendData ? (
              <GlassEmpty text="暂无登录数据" />
            ) : (
              // 布局参数由已到位的数据长度驱动(而非 trendDays),
              // 避免切换瞬间旧柱先按新间距挤一次、数据到了再跳一次
              <div
                className={`trend-chart ${trendFetching ? 'trend-chart-switching' : ''}`}
                style={{ gap: trendData.length > 15 ? 4 : trendData.length > 7 ? 8 : 12 }}
              >
                {trendData.map((t, i) => {
                  // 柱子多时日期隔行展示，避免标签互相压住
                  const labelEvery = trendData.length > 15 ? 4 : trendData.length > 7 ? 2 : 1
                  return (
                    <div className="trend-col" key={t.date}>
                      <div className="trend-count">
                        {trendData.length <= 7 && t.count > 0 ? t.count : ''}
                      </div>
                      <Tooltip
                        title={
                          <>
                            {t.date}
                            <br />
                            成功 {t.success} 次 · 失败 {t.failed} 次
                          </>
                        }
                      >
                        <div className="trend-bar-area" style={{ animationDelay: `${i * 16}ms` }}>
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
              className="stat-card liquid-dash-stat is-alive"
              hoverable
              onClick={() => navigate('/system/online-user')}
              styles={{ body: { padding: 20 } }}
              style={{ '--tint': 'rgba(14, 165, 233, 0.13)' } as React.CSSProperties}
            >
              <div className="liquid-sheen" aria-hidden="true">
                <i />
                <i />
              </div>
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
              className="liquid-dash-panel is-alive"
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
              <div className="liquid-sheen" aria-hidden="true">
                <i />
                <i />
              </div>
              {notices === null ? (
                <Skeleton active paragraph={{ rows: 3 }} />
              ) : notices.length === 0 ? (
                <GlassEmpty text="暂无公告" compact />
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
