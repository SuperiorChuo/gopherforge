import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

// message-as-key 策略：t() 的 key 即中文原文，零 zh.json——中文是源码真源。
// en.json 是渐进词典（缺失/空串回落中文），未译时 en 模式显示原文中文。
// 语言切换 = 手动按钮 + localStorage（复刻主题范式，不装 languagedetector 防浏览器自动覆盖）。
//
// en 词典按需加载：不静态 import（曾把 ~312KB/约100KB gzip 的渐进词典打进首屏
// init chunk，而默认语言 zh 完全不用它）。改为动态 import 单飞加载：
// - zh 用户：永不下载词典；
// - en 用户：首次切到 en 时拉取一次并 addResourceBundle，随后常驻内存；
// - 加载失败仍切到 en，缺失 key 经 parseMissingKeyHandler 回落中文，页面不瘫。
// 注意：App.tsx 必须等本模块返回的 promise 完成、addResourceBundle 落地后再
// changeLanguage('en')，否则 i18next 不会因后续加词而主动重渲染（无
// languageChanged 事件），已渲染组件会停留在「key 即中文」状态。
type EnDictionary = Record<string, string>
let enDictionaryPromise: Promise<EnDictionary> | null = null

export function loadEnDictionary(): Promise<EnDictionary> {
  enDictionaryPromise ??= import('./locales/en.json').then(
    (m) => m.default as unknown as EnDictionary,
  )
  return enDictionaryPromise
}

const stored = typeof window !== 'undefined' ? window.localStorage.getItem('app_locale') : null

// 上次会话是 en 的用户：应用启动时即并行预热词典（与本 chunk 网络并行），
// 让 App 首个 effect 里 loadEnDictionary 命中同一单飞 promise。
if (stored === 'en') void loadEnDictionary()

i18n.use(initReactI18next).init({
  lng: 'zh',
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
  // dev + en 下给未译串打 ⟦⟧ 标记（同样先插值）。注意：en 词典按需加载期间
  // resolvedLanguage 可能仍是 zh（中文直出、无标记），待 addResourceBundle +
  // changeLanguage 后恢复标记语义。
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
