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
  // B0
  'src/pages/system/audit-log/index.tsx',
  'src/components/common/TableToolbar.tsx',
  'src/components/common/GlassEmpty.tsx',
  'src/utils/format.ts',
  // B1 共享组件
  'src/components/common/StatusPill.tsx',
  'src/components/common/ErrorBoundary.tsx',
  'src/components/common/TrendChart.tsx',
  'src/components/common/NotificationBell.tsx',
  'src/components/common/ExcelImportModal.tsx',
  'src/components/common/CommandPalette.tsx',
  // B2 壳层
  'src/layouts/MainLayout.tsx',
  'src/layouts/menu-defs.tsx',
  'src/layouts/menu-build.tsx',
  // B3 pages/system
  'src/pages/system/login-log/index.tsx',
  'src/pages/system/online-user/index.tsx',
  'src/pages/system/permission/index.tsx',
  'src/pages/system/permission-diagnostics/index.tsx',
  'src/pages/system/ai-usage/index.tsx',
  'src/pages/system/posts/index.tsx',
  'src/pages/system/codegen/index.tsx',
  'src/pages/system/codegen/TablePicker.tsx',
  'src/pages/system/security-events/index.tsx',
  'src/pages/system/department/index.tsx',
  'src/pages/system/errcodes/index.tsx',
  'src/pages/system/dict/index.tsx',
  'src/pages/system/file/index.tsx',
  'src/pages/system/inbox/index.tsx',
  'src/pages/system/menu/index.tsx',
  'src/pages/system/role/index.tsx',
  'src/pages/system/oauth2/index.tsx',
  'src/pages/system/notice/index.tsx',
  'src/pages/system/ai-models/index.tsx',
  'src/pages/system/ai-quotas/index.tsx',
  'src/pages/system/notify-templates/index.tsx',
  'src/pages/system/operation-log/index.tsx',
  'src/pages/system/setting/index.tsx',
  'src/pages/system/tenant/index.tsx',
  'src/pages/system/tenant-packages/index.tsx',
  'src/pages/system/user/index.tsx',
  'src/pages/system/sms/index.tsx',
  // B4 独立页
  'src/pages/login/index.tsx',
  'src/pages/register/index.tsx',
  'src/pages/forgot-password/index.tsx',
  'src/pages/reset-password/index.tsx',
  'src/pages/cashier/index.tsx',
  'src/pages/profile/index.tsx',
  // B5 业务域页面
  'src/pages/monitor/redis/index.tsx',
  'src/pages/monitor/grafana/index.tsx',
  'src/pages/monitor/server/index.tsx',
  'src/pages/monitor/mysql/index.tsx',
  'src/pages/monitor/alerts/index.tsx',
  'src/pages/monitor/job/index.tsx',
  'src/pages/bpm/tasks/index.tsx',
  'src/pages/bpm/start/index.tsx',
  'src/pages/bpm/designer/index.tsx',
  'src/pages/bpm/instances/index.tsx',
  'src/pages/bpm/definitions/index.tsx',
  'src/pages/crm/customers/index.tsx',
  'src/pages/crm/pipeline/index.tsx',
  'src/pages/crm/leads/index.tsx',
  'src/pages/crm/contracts/index.tsx',
  'src/pages/crm/pool-rules/index.tsx',
  'src/pages/crm/dashboard/index.tsx',
  'src/pages/im/desk/index.tsx',
  'src/pages/im/sites/index.tsx',
  'src/pages/im/sensitive/index.tsx',
  'src/pages/im/kb-misses/index.tsx',
  'src/pages/im/skills/index.tsx',
  'src/pages/im/stats/index.tsx',
  'src/pages/im/low-ratings/index.tsx',
  'src/pages/mp/messages/index.tsx',
  'src/pages/mp/articles/index.tsx',
  'src/pages/mp/accounts/index.tsx',
  'src/pages/mp/menu/index.tsx',
  'src/pages/mp/fans/index.tsx',
  'src/pages/ai/assistant/index.tsx',
  'src/pages/ai/knowledge/index.tsx',
  'src/pages/dashboard/index.tsx',
  'src/pages/ticket/index.tsx',
  'src/pages/visibility/brands/index.tsx',
  'src/pages/visibility/geo/index.tsx',
  'src/pages/visibility/geo-compare/index.tsx',
  'src/pages/visibility/overview/index.tsx',
  'src/pages/visibility/seo/index.tsx',
  // B6 CC 呼叫中心
  'src/pages/cc/trunks/index.tsx',
  'src/pages/cc/billing/index.tsx',
  'src/pages/cc/queues/index.tsx',
  'src/pages/cc/console/index.tsx',
  'src/pages/cc/console/WebPhone.tsx',
  'src/pages/cc/monitor/index.tsx',
  'src/pages/cc/calls/index.tsx',
  'src/pages/cc/conference/index.tsx',
  'src/pages/cc/ivr/index.tsx',
  'src/pages/cc/extensions/index.tsx',
  'src/pages/cc/workbench/index.tsx',
  'src/pages/cc/qc/index.tsx',
  'src/pages/cc/ringgroups/index.tsx',
  'src/pages/cc/timeroutes/index.tsx',
  'src/pages/cc/prompts/index.tsx',
  'src/pages/cc/reports/index.tsx',
  'src/pages/cc/blacklist/index.tsx',
  'src/pages/cc/moh/index.tsx',
  'src/pages/cc/voicemail/index.tsx',
  'src/pages/cc/recordings/index.tsx',
  // B7 Hermes
  'src/pages/hermes/office/index.tsx',
  'src/pages/hermes/office/Scene.tsx',
  'src/pages/hermes/mine/index.tsx',
  'src/pages/hermes/skills-market/index.tsx',
  'src/pages/hermes/collaborations/index.tsx',
  'src/pages/hermes/devices/index.tsx',
  'src/pages/hermes/mcp-market/index.tsx',
  'src/pages/hermes/cron/index.tsx',
  'src/pages/hermes/instances/index.tsx',
  'src/pages/hermes/artifacts/index.tsx',
  'src/pages/hermes/policies/index.tsx',
  'src/pages/hermes/clusters/index.tsx',
  'src/pages/hermes/overview/index.tsx',
  'src/pages/hermes/ai-ops/index.tsx',
  'src/pages/hermes/tasks/index.tsx',
  'src/pages/hermes/entitlements/index.tsx',
  'src/pages/hermes/runtimes/index.tsx',
  'src/pages/hermes/skills/index.tsx',
  'src/pages/hermes/_shared.ts',
  // B8 面 + 业务组件 + API 字段类型字典
  'src/pages/pay/index.tsx',
  'src/pages/oauth/authorize/index.tsx',
  'src/pages/social/content/index.tsx',
  'src/pages/social/publish/index.tsx',
  'src/pages/system/codegen/FieldConfigTable.tsx',
  'src/pages/system/codegen/RelationConfig.tsx',
  'src/pages/system/codegen/PlanPreview.tsx',
  'src/components/bpm/BpmTaskActions.tsx',
  'src/components/bpm/BpmStatsPanel.tsx',
  'src/components/crm/CrmDialButton.tsx',
  'src/components/cc/CcCallNotifier.tsx',
  'src/components/customer360/Customer360Panel.tsx',
  'src/components/dashboard/CsLoopPanel.tsx',
  'src/components/ticket/TicketDetailDrawer.tsx',
  'src/api/bpm.ts',
  // B9 结果页 + BPM/地图组件漏迁补扫
  'src/pages/result/403/index.tsx',
  'src/pages/result/404/index.tsx',
  'src/pages/result/500/index.tsx',
  'src/components/bpm/BpmResubmitModal.tsx',
  'src/components/bpm/BpmInstanceTimeline.tsx',
  'src/components/bpm/BpmDynamicForm.tsx',
  'src/components/common/GeoMap.tsx',
  // 管理员移动端第一批
  'src/pages/mobile/MobileLayout.tsx',
  'src/pages/mobile/home/index.tsx',
  'src/pages/mobile/inbox/index.tsx',
  'src/pages/mobile/tasks/index.tsx',
  'src/pages/mobile/workbench/index.tsx',
  'src/pages/mobile/me/index.tsx',
  'src/pages/mobile/soon/index.tsx',
  'src/pages/mobile/nav.ts',
  'src/pages/mobile/ops/health.tsx',
  'src/pages/mobile/ops/alerts.tsx',
  'src/pages/mobile/ops/jobs.tsx',
  'src/pages/mobile/ops/online.tsx',
  'src/pages/mobile/security/LogListPage.tsx',
  'src/pages/mobile/security/login-log.tsx',
  'src/pages/mobile/security/security-events.tsx',
  'src/pages/mobile/security/operation-log.tsx',
  'src/pages/mobile/security/audit-log.tsx',
  'src/pages/mobile/directory/users.tsx',
  'src/pages/mobile/directory/roles.tsx',
  'src/pages/mobile/directory/depts.tsx',
  'src/pages/mobile/directory/tenants.tsx',
  'src/pages/mobile/catalog/files.tsx',
  'src/pages/mobile/catalog/dicts.tsx',
  'src/pages/mobile/catalog/templates.tsx',
  'src/pages/mobile/notices/index.tsx',
  'src/pages/mobile/me/password.tsx',
  'src/pages/mobile/me/totp.tsx',
  'src/pages/mobile/me/logins.tsx',
  'src/pages/mobile/pwa.ts',
  'src/pages/mobile/native.ts',
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
