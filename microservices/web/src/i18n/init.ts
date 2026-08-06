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
  // message-as-key：zh 模式无 zh.json，key 走这里返回。必须用传入 options 对 key 插值，
  // 否则 `t('近 {{n}} 天', { n: 7 })` 在 zh 模式返回字面 `{{n}}`（曾致全站插值泄漏）。
  // dev + en 下给未译串打 ⟦⟧ 标记（同样先插值）。
  parseMissingKeyHandler: (key, _defaultValue, options) => {
    const interpolated = i18n.services.interpolator.interpolate(
      key,
      options ?? {},
      i18n.resolvedLanguage ?? i18n.language,
      i18n.options.interpolation ?? {},
    )
    if (import.meta.env.DEV && i18n.resolvedLanguage === 'en') return `⟦${interpolated}⟧`
    return interpolated
  },
  missingKeyHandler: (_lngs, _ns, key) => {
    if (typeof window !== 'undefined') {
      const w = window as unknown as { __GAK_MISSING?: Set<string> }
      w.__GAK_MISSING ||= new Set()
      w.__GAK_MISSING.add(key)
    }
  },
})

export default i18n
