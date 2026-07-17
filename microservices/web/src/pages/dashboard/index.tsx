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
  EnvironmentOutlined,
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

/** 天气语义色：驱动玻璃卡色晕 / 图标主题 / 氛围粒子 */
type WeatherKind = 'sunny' | 'night' | 'cloudy' | 'overcast' | 'rain' | 'thunder' | 'snow' | 'fog'

function weatherKind(text: string, hour: number): WeatherKind {
  const night = hour >= 19 || hour < 6
  if (/雷/.test(text)) return 'thunder'
  if (/雪|冰雹/.test(text)) return 'snow'
  if (/雨|阵雨|毛毛雨|小雨|中雨|大雨|暴雨/.test(text)) return 'rain'
  if (/雾|霾|浮尘|扬沙|沙尘/.test(text)) return 'fog'
  if (/阴/.test(text)) return 'overcast'
  if (/云|多云/.test(text)) return night ? 'night' : 'cloudy'
  if (/晴/.test(text)) return night ? 'night' : 'sunny'
  return night ? 'night' : 'cloudy'
}

/** 高德风力等级文本 → 粗估风速 m/s（体感公式用） */
function windPowerToMs(power?: string): number {
  if (!power) return 1.5
  const m = power.match(/(\d+(?:\.\d+)?)/)
  const level = m ? Number(m[1]) : 2
  // 蒲福风级近似中值
  const table = [0.2, 1.5, 3.3, 5.4, 7.9, 10.7, 13.8, 17.1, 20.7, 24.4, 28.4, 32.6]
  if (level <= 0) return table[0]
  if (level >= table.length) return table[table.length - 1]
  return table[Math.round(level)] ?? 3
}

/** 澳大利亚表观温度（体感）近似，单位 °C；算不出则返回 null */
function feelsLikeC(tempStr?: string, humidityStr?: string, windPower?: string): number | null {
  const t = Number(tempStr)
  const rh = Number(humidityStr)
  if (!Number.isFinite(t) || !Number.isFinite(rh)) return null
  const ws = windPowerToMs(windPower)
  const e = (rh / 100) * 6.105 * Math.exp((17.27 * t) / (237.7 + t))
  const at = t + 0.33 * e - 0.7 * ws - 4.0
  return Math.round(at)
}

/** 矢量天气标：比 emoji 更精致，随 kind 换形态 */
function WeatherGlyph({ kind }: { kind: WeatherKind }) {
  const common = {
    viewBox: '0 0 64 64',
    width: 48,
    height: 48,
    fill: 'none',
    xmlns: 'http://www.w3.org/2000/svg',
    'aria-hidden': true as const,
  }
  switch (kind) {
    case 'sunny':
      return (
        <svg {...common} className="wx-glyph wx-glyph-sun">
          <circle className="wx-sun-core" cx="32" cy="32" r="11" fill="url(#wg-sun)" />
          <g className="wx-sun-rays" stroke="url(#wg-sun-ray)" strokeWidth="2.2" strokeLinecap="round" opacity="0.95">
            <path d="M32 6v7M32 51v7M6 32h7M51 32h7M13.5 13.5l5 5M45.5 45.5l5 5M13.5 50.5l5-5M45.5 18.5l5-5" />
          </g>
          <defs>
            <radialGradient id="wg-sun" cx="40%" cy="35%" r="65%">
              <stop offset="0%" stopColor="#fef9c3" />
              <stop offset="55%" stopColor="#fbbf24" />
              <stop offset="100%" stopColor="#f59e0b" />
            </radialGradient>
            <linearGradient id="wg-sun-ray" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0%" stopColor="#fde68a" />
              <stop offset="100%" stopColor="#fbbf24" />
            </linearGradient>
          </defs>
        </svg>
      )
    case 'night':
      return (
        <svg {...common} className="wx-glyph wx-glyph-moon">
          <path
            d="M40 12c-1.2 3.4-1.2 7.2.2 10.6 2.4 5.8 8 9.8 14.3 10.4-2.2 8.6-10 14.8-19.2 14.8-11 0-20-9-20-20 0-9.2 6.2-17 14.7-19.3 1.4 5.8 5.2 10.6 10 13.5z"
            fill="url(#wg-moon)"
          />
          <circle className="wx-star" cx="48" cy="16" r="1.4" fill="#e0e7ff" opacity="0.9" />
          <circle className="wx-star" cx="54" cy="28" r="1" fill="#c7d2fe" opacity="0.75" />
          <circle className="wx-star" cx="44" cy="8" r="0.9" fill="#a5b4fc" opacity="0.7" />
          <defs>
            <linearGradient id="wg-moon" x1="0.2" y1="0" x2="0.9" y2="1">
              <stop offset="0%" stopColor="#e0e7ff" />
              <stop offset="55%" stopColor="#a5b4fc" />
              <stop offset="100%" stopColor="#6366f1" />
            </linearGradient>
          </defs>
        </svg>
      )
    case 'rain':
    case 'thunder':
      return (
        <svg {...common} className={`wx-glyph wx-glyph-${kind}`}>
          <path
            d="M22 40c-6 0-11-4.5-11-10.2C11 24 15.5 19.5 21 19c1.6-5.5 6.7-9.5 12.7-9.5 6.6 0 12.1 4.6 13.4 10.8 5.2.6 9.2 4.9 9.2 10.1 0 5.6-4.6 10.1-10.3 10.1H22z"
            fill="url(#wg-cloud)"
          />
          {kind === 'thunder' ? (
            <path className="wx-bolt" d="M33 38l-4 10h5l-2 10 10-14h-5l3-6z" fill="#fde047" stroke="#fbbf24" strokeWidth="0.5" />
          ) : (
            <g className="wx-drops" stroke="#7dd3fc" strokeWidth="2.2" strokeLinecap="round">
              <path d="M24 46v8M32 44v10M40 46v8" opacity="0.9" />
            </g>
          )}
          <defs>
            <linearGradient id="wg-cloud" x1="0" y1="0" x2="0.5" y2="1">
              <stop offset="0%" stopColor="#e2e8f0" />
              <stop offset="100%" stopColor="#94a3b8" />
            </linearGradient>
          </defs>
        </svg>
      )
    case 'snow':
      return (
        <svg {...common} className="wx-glyph wx-glyph-snow">
          <path
            d="M22 38c-6 0-11-4.5-11-10.2C11 22 15.5 17.5 21 17c1.6-5.5 6.7-9.5 12.7-9.5 6.6 0 12.1 4.6 13.4 10.8 5.2.6 9.2 4.9 9.2 10.1 0 5.6-4.6 10.1-10.3 10.1H22z"
            fill="url(#wg-cloud-snow)"
          />
          <g className="wx-flakes" fill="#e0f2fe" opacity="0.95">
            <circle cx="24" cy="48" r="1.6" />
            <circle cx="33" cy="52" r="1.4" />
            <circle cx="40" cy="47" r="1.5" />
            <circle cx="28" cy="54" r="1.1" />
          </g>
          <defs>
            <linearGradient id="wg-cloud-snow" x1="0" y1="0" x2="0.5" y2="1">
              <stop offset="0%" stopColor="#f1f5f9" />
              <stop offset="100%" stopColor="#cbd5e1" />
            </linearGradient>
          </defs>
        </svg>
      )
    case 'fog':
      return (
        <svg {...common} className="wx-glyph wx-glyph-fog">
          <g className="wx-mist" stroke="url(#wg-fog)" strokeWidth="3" strokeLinecap="round" opacity="0.85">
            <path d="M14 24h36M12 32h40M16 40h32M18 48h28" />
          </g>
          <defs>
            <linearGradient id="wg-fog" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="#94a3b8" stopOpacity="0.3" />
              <stop offset="50%" stopColor="#e2e8f0" />
              <stop offset="100%" stopColor="#94a3b8" stopOpacity="0.3" />
            </linearGradient>
          </defs>
        </svg>
      )
    case 'overcast':
      return (
        <svg {...common} className="wx-glyph wx-glyph-overcast">
          <path
            d="M18 42c-6.5 0-12-5-12-11.2C6 25 11 20 17 19.4 18.8 13 24.6 8.5 31.5 8.5c7.4 0 13.6 5.2 15 12.1 5.8.7 10.3 5.5 10.3 11.3 0 6.3-5.2 11.4-11.6 11.4H18z"
            fill="url(#wg-overcast)"
          />
          <defs>
            <linearGradient id="wg-overcast" x1="0.2" y1="0" x2="0.7" y2="1">
              <stop offset="0%" stopColor="#cbd5e1" />
              <stop offset="100%" stopColor="#64748b" />
            </linearGradient>
          </defs>
        </svg>
      )
    default: // cloudy
      return (
        <svg {...common} className="wx-glyph wx-glyph-cloudy">
          <circle className="wx-part-sun" cx="42" cy="22" r="8" fill="url(#wg-part-sun)" opacity="0.95" />
          <path
            d="M20 44c-5.8 0-10.5-4.4-10.5-9.8C9.5 28.8 13.8 24.5 19 24c1.5-5.2 6.3-9 12-9 6.2 0 11.4 4.3 12.6 10.2 4.9.6 8.7 4.6 8.7 9.5 0 5.3-4.4 9.6-9.8 9.6H20z"
            fill="url(#wg-part-cloud)"
          />
          <defs>
            <radialGradient id="wg-part-sun" cx="40%" cy="35%" r="65%">
              <stop offset="0%" stopColor="#fef08a" />
              <stop offset="100%" stopColor="#f59e0b" />
            </radialGradient>
            <linearGradient id="wg-part-cloud" x1="0" y1="0" x2="0.5" y2="1">
              <stop offset="0%" stopColor="#f8fafc" />
              <stop offset="100%" stopColor="#94a3b8" />
            </linearGradient>
          </defs>
        </svg>
      )
  }
}

/** 卡内氛围层：雨丝/雪花/星点/雾带，纯装饰 */
function WeatherAtmosphere({ kind }: { kind: WeatherKind }) {
  if (kind === 'rain' || kind === 'thunder') {
    return (
      <div className="dash-weather-fx is-rain" aria-hidden>
        {Array.from({ length: 10 }, (_, i) => (
          <i key={i} style={{ '--d': i } as React.CSSProperties} />
        ))}
      </div>
    )
  }
  if (kind === 'snow') {
    return (
      <div className="dash-weather-fx is-snow" aria-hidden>
        {Array.from({ length: 8 }, (_, i) => (
          <i key={i} style={{ '--d': i } as React.CSSProperties} />
        ))}
      </div>
    )
  }
  if (kind === 'night') {
    return (
      <div className="dash-weather-fx is-night" aria-hidden>
        {Array.from({ length: 6 }, (_, i) => (
          <i key={i} style={{ '--d': i } as React.CSSProperties} />
        ))}
      </div>
    )
  }
  if (kind === 'fog') {
    return <div className="dash-weather-fx is-fog" aria-hidden><i /><i /></div>
  }
  if (kind === 'sunny') {
    return <div className="dash-weather-fx is-sunny" aria-hidden><i /></div>
  }
  return null
}

const WEATHER_CACHE_KEY = 'dash-weather-cache-v2'
const WEATHER_CACHE_TTL = 30 * 60 * 1000

// localStorage 缓存：页面往返 dashboard 不重复请求；失败静默（返回 null 则不渲染）
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
  const wKind = weather?.weather ? weatherKind(weather.weather, hour) : null
  const windLabel =
    weather?.wind_dir || weather?.wind_power
      ? [weather.wind_dir, weather.wind_power ? `${weather.wind_power}级` : '']
          .filter(Boolean)
          .join(' ')
      : ''
  const feels = weather
    ? feelsLikeC(weather.temperature, weather.humidity, weather.wind_power)
    : null
  const tempRange =
    weather?.temp_high && weather?.temp_low
      ? `${weather.temp_low}° / ${weather.temp_high}°`
      : ''

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
      <div className={`dash-hero liquid-dash is-alive${wKind ? ` has-weather weather-${wKind}` : ''}`}>
        <div className="liquid-sheen" aria-hidden="true">
          <i />
          <i />
        </div>
        <div className="dash-hero-main">
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
              {lastLogin?.created_at && (
                <span className="hero-chip">
                  <HistoryOutlined />
                  上次登录 {dayjs(lastLogin.created_at).format('MM-DD HH:mm')}
                  {lastLogin.ip ? ` · ${lastLogin.ip}` : ''}
                </span>
              )}
            </div>
          </div>

          {weather?.weather && wKind && (
            <div
              className={`dash-weather is-${wKind}`}
              title={
                weather.report_time
                  ? `观测时间 ${dayjs(weather.report_time).format('MM-DD HH:mm')}`
                  : undefined
              }
            >
              <WeatherAtmosphere kind={wKind} />
              <div className="dash-weather-glow" aria-hidden />
              <div className="dash-weather-sheen" aria-hidden />

              <div className="dash-weather-head">
                <div className="dash-weather-icon">
                  <WeatherGlyph kind={wKind} />
                </div>
                <div className="dash-weather-head-text">
                  <div className="dash-weather-city">
                    <EnvironmentOutlined />
                    {weather.city || '本地'}
                  </div>
                  <div className="dash-weather-cond">{weather.weather}</div>
                </div>
              </div>

              <div className="dash-weather-temp-row">
                <span className="dash-weather-temp">
                  {weather.temperature != null && weather.temperature !== ''
                    ? weather.temperature
                    : '—'}
                  <sup>°</sup>
                </span>
                <div className="dash-weather-side">
                  {feels != null && (
                    <span className="dash-weather-feel">体感 {feels}°</span>
                  )}
                  {tempRange && (
                    <span className="dash-weather-range">{tempRange}</span>
                  )}
                </div>
              </div>

              <div className="dash-weather-stats">
                {weather.humidity != null && weather.humidity !== '' && (
                  <div className="wx-stat">
                    <em>湿度</em>
                    <b>{weather.humidity}%</b>
                  </div>
                )}
                {windLabel && (
                  <div className="wx-stat">
                    <em>风力</em>
                    <b>{windLabel}</b>
                  </div>
                )}
                {weather.report_time && (
                  <div className="wx-stat">
                    <em>更新</em>
                    <b>{dayjs(weather.report_time).format('HH:mm')}</b>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        {/* 轨道装饰:两圈细环 + 沿环轨道公转的光点 */}
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
