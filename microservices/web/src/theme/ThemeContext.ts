import { createContext, useContext } from 'react'

export type ThemeMode = 'dark' | 'light'

export const THEME_STORAGE_KEY = 'app_theme'

export const ThemeContext = createContext<{
  mode: ThemeMode
  toggle: (point?: { x: number; y: number }) => void
}>({
  mode: 'dark',
  toggle: () => {},
})

export const useThemeMode = () => useContext(ThemeContext)
