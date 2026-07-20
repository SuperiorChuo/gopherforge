import { useCallback, useEffect, useState } from 'react'
import { flushSync } from 'react-dom'
import { Provider } from 'react-redux'
import { BrowserRouter, useRoutes } from 'react-router-dom'
import { ConfigProvider, App as AntApp, theme as antdTheme, type ThemeConfig } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { store } from '@/store'
import routes from '@/router'
import FeedbackBridge from '@/utils/feedback'
import GlassEmpty from '@/components/GlassEmpty'
import { ThemeContext, THEME_STORAGE_KEY, type ThemeMode } from '@/theme/ThemeContext'
import 'dayjs/locale/zh-cn'

function AppRoutes() {
  return useRoutes(routes)
}

// 深空暗色（默认）
const darkTheme: ThemeConfig = {
  algorithm: antdTheme.darkAlgorithm,
  token: {
    colorPrimary: '#6366f1',
    colorInfo: '#6366f1',
    colorSuccess: '#34d399',
    colorWarning: '#fbbf24',
    colorError: '#f87171',
    borderRadius: 8,
    fontSize: 14,
    // 深空底色体系：页面底 < 容器 < 悬浮层，层级靠亮度区分
    colorBgLayout: '#0a0b14',
    colorBgContainer: '#12142299',
    // 悬浮层必须半透明,否则 CSS 侧的 backdrop-filter 无从生效(玻璃变塑料板)
    colorBgElevated: 'rgba(24, 26, 46, 0.85)',
    colorBorder: 'rgba(148, 163, 184, 0.22)',
    colorBorderSecondary: 'rgba(148, 163, 184, 0.12)',
    colorText: 'rgba(226, 232, 240, 0.88)',
    colorTextSecondary: 'rgba(148, 163, 184, 0.85)',
    colorTextTertiary: 'rgba(148, 163, 184, 0.6)',
    colorTextQuaternary: 'rgba(148, 163, 184, 0.4)',
    boxShadowSecondary: '0 10px 34px rgba(0, 0, 0, 0.45)',
  },
  components: {
    Card: {
      borderRadiusLG: 14,
      paddingLG: 20,
      colorBgContainer: 'rgba(18, 20, 34, 0.6)',
      colorBorderSecondary: 'rgba(148, 163, 184, 0.12)',
    },
    Layout: {
      headerBg: 'transparent',
      bodyBg: '#0a0b14',
      siderBg: 'transparent',
    },
    Menu: {
      darkItemBg: 'transparent',
      darkSubMenuItemBg: 'transparent',
      darkItemSelectedBg: 'rgba(99, 102, 241, 0.85)',
      darkItemHoverBg: 'rgba(255, 255, 255, 0.07)',
      itemBorderRadius: 8,
      itemMarginInline: 8,
    },
    Table: {
      headerBg: 'rgba(255, 255, 255, 0.03)',
      headerColor: 'rgba(203, 213, 225, 0.8)',
      rowHoverBg: 'rgba(99, 102, 241, 0.07)',
      borderColor: 'rgba(148, 163, 184, 0.1)',
      colorBgContainer: 'transparent',
    },
    Modal: {
      contentBg: 'rgba(21, 23, 41, 0.82)',
      headerBg: 'transparent',
    },
    Button: {
      controlHeight: 34,
      primaryShadow: '0 4px 14px rgba(99, 102, 241, 0.35)',
    },
    Tooltip: {
      colorBgSpotlight: 'rgba(30, 33, 56, 0.9)',
    },
  },
}

// 白蓝液态玻璃（亮色）
const lightTheme: ThemeConfig = {
  algorithm: antdTheme.defaultAlgorithm,
  token: {
    colorPrimary: '#2563eb',
    colorInfo: '#2563eb',
    colorSuccess: '#059669',
    colorWarning: '#d97706',
    colorError: '#dc2626',
    borderRadius: 8,
    fontSize: 14,
    colorBgLayout: '#edf3fb',
    // 半透明白容器 + CSS 侧 backdrop-filter，构成液态玻璃
    colorBgContainer: 'rgba(255, 255, 255, 0.72)',
    colorBgElevated: '#ffffff',
    colorBorder: 'rgba(15, 23, 42, 0.15)',
    colorBorderSecondary: 'rgba(15, 23, 42, 0.06)',
    colorText: 'rgba(15, 23, 42, 0.88)',
    colorTextSecondary: 'rgba(71, 85, 105, 0.9)',
    colorTextTertiary: 'rgba(100, 116, 139, 0.75)',
    colorTextQuaternary: 'rgba(148, 163, 184, 0.6)',
    boxShadowSecondary: '0 10px 34px rgba(37, 99, 235, 0.1)',
  },
  components: {
    Card: {
      borderRadiusLG: 14,
      paddingLG: 20,
      colorBgContainer: 'rgba(255, 255, 255, 0.72)',
      colorBorderSecondary: 'rgba(15, 23, 42, 0.06)',
    },
    Layout: {
      headerBg: 'transparent',
      bodyBg: '#edf3fb',
      siderBg: 'transparent',
    },
    Menu: {
      itemBg: 'transparent',
      subMenuItemBg: 'transparent',
      itemSelectedBg: 'rgba(37, 99, 235, 0.1)',
      itemSelectedColor: '#2563eb',
      itemHoverBg: 'rgba(37, 99, 235, 0.06)',
      itemBorderRadius: 8,
      itemMarginInline: 8,
    },
    Table: {
      headerBg: 'rgba(37, 99, 235, 0.04)',
      headerColor: '#475569',
      rowHoverBg: 'rgba(37, 99, 235, 0.045)',
      borderColor: 'rgba(15, 23, 42, 0.06)',
      colorBgContainer: 'transparent',
    },
    Button: {
      controlHeight: 34,
      primaryShadow: '0 4px 14px rgba(37, 99, 235, 0.3)',
    },
  },
}

// View Transitions API(Chromium 111+ / Safari 18+),不支持时直接切换
type ViewTransitionDocument = Document & {
  startViewTransition?: (update: () => void) => { ready: Promise<void> }
}

// 指针高光:把鼠标在卡片内的坐标写进 --lgx/--lgy,
// index.css 里 .ant-card::after / .login-shell 的反光光点跟着游走
function useGlassPointerLight() {
  useEffect(() => {
    let raf = 0
    let lit: HTMLElement | null = null
    const clear = () => {
      lit?.style.removeProperty('--lgx')
      lit?.style.removeProperty('--lgy')
      lit?.classList.remove('is-pointer-lit')
      lit = null
    }
    let last: PointerEvent | null = null
    const onMove = (e: PointerEvent) => {
      last = e
      if (raf) return
      raf = requestAnimationFrame(() => {
        raf = 0
        const ev = last
        if (!ev) return
        const el =
          ev.target instanceof Element
            ? ev.target.closest<HTMLElement>('.ant-card, .login-shell')
            : null
        if (lit && lit !== el) clear()
        if (el) {
          const rect = el.getBoundingClientRect()
          el.style.setProperty('--lgx', `${Math.round(ev.clientX - rect.left)}px`)
          el.style.setProperty('--lgy', `${Math.round(ev.clientY - rect.top)}px`)
          el.classList.add('is-pointer-lit')
          lit = el
        }
      })
    }
    document.addEventListener('pointermove', onMove, { passive: true })
    document.documentElement.addEventListener('pointerleave', clear)
    return () => {
      document.removeEventListener('pointermove', onMove)
      document.documentElement.removeEventListener('pointerleave', clear)
      cancelAnimationFrame(raf)
      clear()
    }
  }, [])
}

export default function App() {
  const [mode, setMode] = useState<ThemeMode>(() =>
    localStorage.getItem(THEME_STORAGE_KEY) === 'light' ? 'light' : 'dark',
  )

  useEffect(() => {
    document.documentElement.dataset.theme = mode
    localStorage.setItem(THEME_STORAGE_KEY, mode)
  }, [mode])

  useGlassPointerLight()

  const toggle = useCallback((point?: { x: number; y: number }) => {
    const next: ThemeMode =
      document.documentElement.dataset.theme === 'light' ? 'dark' : 'light'
    const apply = () => {
      setMode(next)
      // 快照在 useEffect 之前捕获,这里同步写属性让 CSS 主题立刻生效(effect 幂等重放)
      document.documentElement.dataset.theme = next
    }
    const doc = document as ViewTransitionDocument
    if (!doc.startViewTransition || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      apply()
      return
    }
    // 新主题从切换按钮处以圆形"液面"漫开
    const x = point?.x ?? window.innerWidth / 2
    const y = point?.y ?? 60
    const radius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y))
    doc
      .startViewTransition(() => flushSync(apply))
      .ready.then(() => {
        document.documentElement.animate(
          { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${radius}px at ${x}px ${y}px)`] },
          {
            duration: 620,
            easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
            pseudoElement: '::view-transition-new(root)',
          },
        )
      })
      .catch(() => {
        // 快速连点导致过渡被跳过时静默,主题本身已切换
      })
  }, [])

  return (
    <Provider store={store}>
      <ThemeContext.Provider value={{ mode, toggle }}>
        <ConfigProvider
          locale={zhCN}
          theme={mode === 'dark' ? darkTheme : lightTheme}
          renderEmpty={() => <GlassEmpty />}
        >
          <AntApp>
            <FeedbackBridge />
            <BrowserRouter basename={import.meta.env.BASE_URL.replace(/\/$/, '')}>
              <AppRoutes />
            </BrowserRouter>
            {import.meta.env.VITE_DEMO === '1' && (
              <div
                style={{
                  position: 'fixed', bottom: 16, left: 16, zIndex: 9999, pointerEvents: 'none',
                  padding: '6px 14px', borderRadius: 999, fontSize: 12, color: '#fff',
                  background: 'rgba(37,99,235,.88)', boxShadow: '0 4px 16px rgba(37,99,235,.35)',
                }}
              >
                演示模式 · 纯前端假数据 · 任意账号可登录
              </div>
            )}
          </AntApp>
        </ConfigProvider>
      </ThemeContext.Provider>
    </Provider>
  )
}
