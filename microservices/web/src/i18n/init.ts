import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './locales/en.json'

// message-as-key 策略：t() 的 key 即中文原文，零 zh.json——中文是源码真源。
// en.json 是渐进词典（缺失/空串回落中文），未译时 en 模式显示原文中文。
// 语言切换 = 手动按钮 + localStorage（复刻主题范式，不装 languagedetector 防浏览器自动覆盖）。
const stored = typeof window !== 'undefined' ? window.localStorage.getItem('app_locale') : null

i18n.use(initReactI18next).init({
  resources: { en: { translation: en } },
  lng: stored === 'en' ? 'en' : 'zh',
  fallbackLng: 'zh',
  supportedLngs: ['zh', 'en'],
  // key 是中文，可能含 `.`/`:`/全角括号/空格——必须关掉 key/ns 分隔符解析
  nsSeparator: false,
  keySeparator: false,
  // en.json 空串条目回落中文而不是渲染空白
  returnEmptyString: false,
  returnNull: false,
  interpolation: { escapeValue: false },
  react: { useSuspense: false },
  // dev + en 下给未译串打 ⟦⟧ 标记；zh 模式永远返回 key 本身（渲染与迁移前逐字节一致）
  parseMissingKeyHandler: (key) =>
    import.meta.env.DEV && i18n.resolvedLanguage === 'en' ? `⟦${key}⟧` : key,
  missingKeyHandler: (_lngs, _ns, key) => {
    if (typeof window !== 'undefined') {
      const w = window as unknown as { __GAK_MISSING?: Set<string> }
      w.__GAK_MISSING ||= new Set()
      w.__GAK_MISSING.add(key)
    }
  },
})

export default i18n
