import { createContext, useContext } from 'react'

export type Locale = 'zh' | 'en'

export const LOCALE_STORAGE_KEY = 'app_locale'

export const LocaleContext = createContext<{ locale: Locale; setLocale: (l: Locale) => void }>({
  locale: 'zh',
  setLocale: () => {},
})

export const useLocale = () => useContext(LocaleContext)
