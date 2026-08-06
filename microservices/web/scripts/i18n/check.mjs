#!/usr/bin/env node
// i18n 兜底检查（message-as-key 策略）。TS7 原生编译器无经典 AST API，故用
// 词法级启发式（注释已剔除）。核心判定：**漏网 = 展示位 CJK 串但不在 en.json 词典里**。
// 词典即 key 真源——被 t() 消费的 key 必然进词典（extract.mjs 保证），所以：
//   - `title="审计日志"`（TableToolbar 内部 t() 消费）→ '审计日志' 在词典 → 非漏网 ✓
//   - `>查询<` 或 `placeholder="新词"` 未包 t() 且不在词典 → 漏网 ✓
// 已知局限：未包 t() 但碰巧在用 key（别处 t() 过）的串会被漏过——概率低，接受。
//
// 渐进机制：MIGRATED 清单限定「已迁移文件」才判展示位（每批追加；全量后=全部）。
// 用法：node scripts/i18n/check.mjs [-v]
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB = dirname(dirname(dirname(fileURLToPath(import.meta.url)))) // .../microservices/web
const SRC = join(WEB, 'src')
const EN = join(SRC, 'i18n/locales/en.json')
const VERBOSE = process.argv.includes('-v')

const MIGRATED = [
  'src/components/BpmStatsPanel.tsx',
  'src/components/BpmTaskActions.tsx',
  'src/components/CommandPalette.tsx',
  'src/components/ErrorBoundary.tsx',
  'src/components/ExcelImportModal.tsx',
  'src/components/GlassEmpty.tsx',
  'src/components/StatusPill.tsx',
  'src/components/TrendChart.tsx',
  'src/layouts/MainLayout.tsx',
  'src/pages/bpm/designer/index.tsx',
  'src/pages/bpm/instances/index.tsx',
  'src/pages/bpm/tasks/index.tsx',
  'src/pages/forgot-password/index.tsx',
  'src/pages/login/index.tsx',
  'src/pages/monitor/alerts/index.tsx',
  'src/pages/monitor/mysql/index.tsx',
  'src/pages/monitor/redis/index.tsx',
  'src/pages/monitor/server/index.tsx',
  'src/pages/oauth/authorize/index.tsx',
  'src/pages/profile/index.tsx',
  'src/pages/register/index.tsx',
  'src/pages/reset-password/index.tsx',
  'src/pages/system/audit-log/index.tsx',
  'src/pages/system/codegen/FieldConfigTable.tsx',
  'src/pages/system/codegen/PlanPreview.tsx',
  'src/pages/system/codegen/RelationConfig.tsx',
  'src/pages/system/codegen/index.tsx',
  'src/pages/system/department/index.tsx',
  'src/pages/system/dict/index.tsx',
  'src/pages/system/errcodes/index.tsx',
  'src/pages/system/file/index.tsx',
  'src/pages/system/menu/index.tsx',
  'src/pages/system/oauth2/index.tsx',
  'src/pages/system/online-user/index.tsx',
  'src/pages/system/operation-log/index.tsx',
  'src/pages/system/permission/index.tsx',
  'src/pages/system/posts/index.tsx',
  'src/pages/system/role/index.tsx',
  'src/pages/system/setting/index.tsx',
  'src/pages/system/sms/index.tsx',
  'src/pages/system/tenant-packages/index.tsx',
  'src/pages/system/tenant/index.tsx',
  'src/pages/system/user/index.tsx',
  'src/utils/format.ts',
]

const files = []
const walk = (d) => {
  for (const name of readdirSync(d)) {
    if (name === 'i18n' || name === 'node_modules' || name === '.vite') continue
    const f = join(d, name)
    if (statSync(f).isDirectory()) walk(f)
    else if (/\.(ts|tsx)$/.test(name)) files.push(f)
  }
}
walk(SRC)

const en = existsSync(EN) ? JSON.parse(readFileSync(EN, 'utf8')) : {}
const DISPLAY_JSX_ATTRS = new Set(['title', 'placeholder', 'label', 'aria-label', 'text', 'description', 'content', 'extra'])
const USED_KEYS_RE = /(?:^|[^\w.])(?:i18n\.)?t\(\s*['"`]([^'"`]*[一-鿿][^'"`]*)['"`]/g

// 候选 = 展示位 CJK；漏网 = 候选且不在词典（key 真源）。
const leaks = [] // { rel, ln, text, soft }
const isLeak = (text) => !(text.trim() in en)

for (const f of files) {
  // rel 带 src/ 前缀以匹配 MIGRATED 清单（清单写法 src/xxx）；曾因前缀不齐致清单永不命中、检查假绿
  const rel = 'src/' + f.replace(SRC + '/', '')
  if (!MIGRATED.includes(rel)) continue
  const src = readFileSync(f, 'utf8')
  const noComments = src
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:"'`])?\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, ' '))
  const lines = noComments.split('\n')

  lines.forEach((line, i) => {
    const ln = i + 1
    // a. JSX 文本裸中文
    const jsxText = line.match(/>([^<>{}]*[一-鿿][^<>{}]*)</)
    if (jsxText) leaks.push({ rel, ln, text: jsxText[1].trim() })
    // b. JSX 展示属性字符串值
    const attrRe = new RegExp(`(${[...DISPLAY_JSX_ATTRS].join('|')})="([^"]*[\\u4e00-\\u9fff][^"]*)"`, 'g')
    let am
    while ((am = attrRe.exec(line))) leaks.push({ rel, ln, text: am[2].trim() })
    // c. message.*/notification.* 字符串参数
    const msgRe = /(?:message|notification)\.[a-z]+\(\s*['"`]([^'"`]*[一-鿿][^'"`]*)['"`]/g
    let mm
    while ((mm = msgRe.exec(line))) leaks.push({ rel, ln, text: mm[1].trim() })
    // d. 软：title:/label: 属性值（可能是动态 key 数据）
    const propRe = /(title|label):\s*['"`]([^'"`]*[一-鿿][^'"`]*)['"`]/g
    let pm
    while ((pm = propRe.exec(line))) leaks.push({ rel, ln, text: pm[2].trim(), soft: true })
  })
}

// t() 字面 key（全 src）
const usedKeys = new Set()
for (const f of files) {
  const src = readFileSync(f, 'utf8').replace(/\/\*[\s\S]*?\*\//g, ' ')
  let m
  while ((m = USED_KEYS_RE.exec(src))) usedKeys.add(m[1].trim())
}

// CJK 对象属性键（动态 t(var) 候选）——全 src 软扫
const propKeys = []
for (const f of files) {
  const src = readFileSync(f, 'utf8').replace(/\/\*[\s\S]*?\*\//g, ' ')
  const pk = src.match(/['"]([^'"]*[一-鿿][^'"]*)['"]\s*:/g) || []
  for (const p of pk) propKeys.push(p.replace(/['":]/g, '').trim())
}

let fail = 0
const err = (m) => { console.error('❌ ' + m); fail = 1 }
const warn = (m) => console.warn('  ⚠️  ' + m)

const hard = leaks.filter((l) => !l.soft && isLeak(l.text))
const soft = leaks.filter((l) => l.soft && isLeak(l.text))
if (hard.length) {
  err(`MIGRATED 文件漏网硬编码 ${hard.length} 处（展示位中文不在 en.json 词典）:`)
  const shown = VERBOSE ? hard : hard.slice(0, 8)
  for (const l of shown) console.error(`    ${l.rel}:${l.ln}  「${l.text.slice(0, 30)}」`)
  if (!VERBOSE && hard.length > 8) console.error(`    … 其余 ${hard.length - 8} 处（-v 看全量）`)
} else {
  console.log(`✅ MIGRATED ${MIGRATED.length} 文件无展示位漏网（词典外中文 ${leaks.filter((l) => !l.soft).length - hard.length} 处为已译 key）`)
}
if (soft.length) warn(`软：${soft.length} 处 title:/label: 值不在词典（可能是未入词典的动态 key 数据）`)

const missing = [...usedKeys].filter((k) => !(k in en))
if (missing.length) {
  err(`en.json 缺 ${missing.length} 个已用 key（跑 node scripts/i18n/extract.mjs --write 补骨架）: ${missing.slice(0, 5).join(' / ')}…`)
} else {
  console.log(`✅ en.json 覆盖全部 ${usedKeys.size} 个 t() key`)
}
const empty = Object.values(en).filter((v) => typeof v === 'string' && v === '').length
if (empty) warn(`en.json ${empty} 个空值条目（en 回落中文,渐进翻译中）`)
const pkMiss = [...new Set(propKeys)].filter((k) => !(k in en))
if (pkMiss.length) warn(`CJK 属性键 ${pkMiss.length} 个未入 en.json（动态 t(var) 回落中文）: ${pkMiss.slice(0, 5).join(' / ')}…`)

console.log(fail ? '❌ i18n 检查未通过' : '✅ i18n 检查通过')
process.exit(fail)
