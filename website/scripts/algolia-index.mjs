// 自建 Algolia 搜索索引管道：解析 VitePress 构建产物 → DocSearch 格式 records → 推送 Algolia。
// 替代官方 Crawler（免费应用创建 crawler 受限），每次 Pages 部署后运行，索引与站点内容保持同步。
//
// 用法（在 website/ 目录）：
//   node scripts/algolia-index.mjs            # 需 ALGOLIA_ADMIN_KEY；缺失时打印摘要后跳过（退出 0，CI 安全）
//   node scripts/algolia-index.mjs --dry-run  # 只解析并打印 records 摘要，不推送
//
// 环境变量：ALGOLIA_APP_ID（默认 PPRG5T8BJE）、ALGOLIA_ADMIN_KEY、ALGOLIA_INDEX（默认 gopherforge）

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { parse } from 'node-html-parser'

const DIST = new URL('../.vitepress/dist', import.meta.url).pathname
const SITE = 'https://superiorchuo.github.io/gopherforge/docs/'
const APP_ID = process.env.ALGOLIA_APP_ID || '23Y7MRK7R7'
const ADMIN_KEY = process.env.ALGOLIA_ADMIN_KEY || ''
const INDEX = process.env.ALGOLIA_INDEX || 'gopherforge'
const DRY_RUN = process.argv.includes('--dry-run')

// lvl0 按路径首段映射为侧边栏分组名（与 config.mts 保持一致）
const LVL0 = {
  'zh-CN': { guide: '指南', modules: '功能模块', reference: '参考' },
  'en-US': { guide: 'Guide', modules: 'Modules', reference: 'Reference' },
}

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) yield* walk(p)
    else if (name.endsWith('.html')) yield p
  }
}

function textOf(el) {
  // 去掉锚点链接和复制按钮等装饰节点后取纯文本
  for (const junk of el.querySelectorAll('.header-anchor, .copy, .lang')) junk.remove()
  return el.textContent.replace(/\s+/g, ' ').trim()
}

function extractPage(file) {
  const html = readFileSync(file, 'utf8')
  const root = parse(html)
  const doc = root.querySelector('main .vp-doc')
  if (!doc) return [] // 首页（VPHome 布局）等无正文页跳过

  const lang = root.querySelector('html')?.getAttribute('lang') || 'zh-CN'
  const rel = relative(DIST, file).replace(/\\/g, '/')
  const url = SITE + rel
  const seg = rel.replace(/^en\//, '').split('/')[0]
  const lvl0 = LVL0[lang]?.[seg] || (lang === 'en-US' ? 'Documentation' : '文档')

  const records = []
  const hierarchy = { lvl0, lvl1: null, lvl2: null, lvl3: null, lvl4: null, lvl5: null, lvl6: null }
  let anchor = ''
  let position = 0

  const push = (type, content) => {
    records.push({
      objectID: `${rel}::${records.length}`,
      url: anchor ? `${url}#${anchor}` : url,
      url_without_anchor: url,
      anchor,
      type,
      content,
      hierarchy: { ...hierarchy },
      lang,
      weight: {
        pageRank: 0,
        level: type === 'content' ? 0 : 100 - Number(type.slice(3)) * 10,
        position: position++,
      },
    })
  }

  for (const el of doc.querySelectorAll('h1, h2, h3, h4, p, li, td')) {
    const tag = el.tagName.toLowerCase()
    if (/^h[1-4]$/.test(tag)) {
      const level = Number(tag[1])
      const id = el.getAttribute('id') || ''
      const text = textOf(el)
      if (!text) continue
      hierarchy[`lvl${level}`] = text
      for (let l = level + 1; l <= 6; l++) hierarchy[`lvl${l}`] = null
      anchor = id
      push(`lvl${level}`, null)
    } else {
      // 松散列表 li 内含 p 会重复计数，只收紧凑 li；跳过 custom-block 标题等噪声
      if (tag === 'li' && el.querySelector('p')) continue
      if (el.classList?.contains('custom-block-title')) continue
      const text = textOf(el)
      if (text.length < 3) continue
      push('content', text.slice(0, 600))
    }
  }
  return records
}

// —— 采集 ——
const records = []
for (const file of walk(DIST)) {
  const rel = relative(DIST, file)
  if (rel === '404.html') continue
  records.push(...extractPage(file))
}

const byLang = {}
for (const r of records) byLang[r.lang] = (byLang[r.lang] || 0) + 1
console.log(`解析完成：${records.length} 条 records（${Object.entries(byLang).map(([k, v]) => `${k}: ${v}`).join('，')}）`)

if (DRY_RUN) {
  console.log('示例 record：', JSON.stringify(records.find((r) => r.type === 'content'), null, 2))
  process.exit(0)
}
if (!ADMIN_KEY) {
  console.log('未设置 ALGOLIA_ADMIN_KEY，跳过推送（fork 或未配 secret 时属预期行为）')
  process.exit(0)
}

// —— 推送：写临时索引 + move 原子换版，避免线上出现空索引窗口 ——
const TMP = `${INDEX}_tmp`
const headers = {
  'X-Algolia-Application-Id': APP_ID,
  'X-Algolia-API-Key': ADMIN_KEY,
  'Content-Type': 'application/json',
}
async function api(method, path, body) {
  const res = await fetch(`https://${APP_ID}.algolia.net/1/indexes${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new Error(`${method} ${path} → ${res.status}: ${await res.text()}`)
  return res.json()
}

await api('PUT', `/${TMP}/settings`, {
  // DocSearch 标准索引设置（对齐官方 crawler 模板）
  searchableAttributes: [
    'unordered(hierarchy.lvl0)',
    'unordered(hierarchy.lvl1)',
    'unordered(hierarchy.lvl2)',
    'unordered(hierarchy.lvl3)',
    'unordered(hierarchy.lvl4)',
    'content',
  ],
  attributesToRetrieve: ['hierarchy', 'content', 'anchor', 'url', 'url_without_anchor', 'type'],
  attributesToHighlight: ['hierarchy', 'content'],
  attributesToSnippet: ['content:12'],
  attributesForFaceting: ['type', 'lang'],
  distinct: true,
  attributeForDistinct: 'url',
  customRanking: ['desc(weight.pageRank)', 'desc(weight.level)', 'asc(weight.position)'],
  ignorePlurals: true,
  advancedSyntax: true,
  removeWordsIfNoResults: 'allOptional',
  queryLanguages: ['zh', 'en'],
  indexLanguages: ['zh', 'en'],
})

for (let i = 0; i < records.length; i += 1000) {
  await api('POST', `/${TMP}/batch`, {
    requests: records.slice(i, i + 1000).map((r) => ({ action: 'addObject', body: r })),
  })
}
await api('POST', `/${TMP}/operation`, { operation: 'move', destination: INDEX })
console.log(`已推送 ${records.length} 条 records 到索引 ${INDEX}（appId ${APP_ID}）`)
