import { createContext, useContext } from 'react'

export type ThemeMode = 'dark' | 'light'

export const THEME_STORAGE_KEY = 'app_theme'

export const ThemeContext = createContext<{
  mode: ThemeMode
  // point: 触发切换的屏幕坐标,主题以此为圆心液态扩散(不传则从顶部中间漫开)
  toggle: (point?: { x: number; y: number }) => void
}>({
  mode: 'dark',
  toggle: () => {},
})

export const useThemeMode = () => useContext(ThemeContext)
