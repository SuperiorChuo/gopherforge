#!/usr/bin/env node
// en.json 骨架生成器：扫描 src/ 下 t() 用到的全部中文 key（词法级，TS7 无经典 AST API），
// 合并进 en.json（保留已译值,新 key 置空串 → en 模式回落中文）。
// 用法：node scripts/i18n/extract.mjs [--write]   # 不带 --write 只预览新增
import { readFileSync, writeFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB = dirname(dirname(dirname(fileURLToPath(import.meta.url)))) // .../microservices/web
const SRC = join(WEB, 'src')
const EN = join(SRC, 'i18n/locales/en.json')
const WRITE = process.argv.includes('--write')

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

const KEYS_RE = /(?:^|[^\w.])(?:i18n\.)?t\(\s*['"`]([^'"`]*[一-鿿][^'"`]*)['"`]/g
const keys = new Set()
for (const f of files) {
  const src = readFileSync(f, 'utf8').replace(/\/\*[\s\S]*?\*\//g, ' ')
  let m
  while ((m = KEYS_RE.exec(src))) keys.add(m[1].trim())
}

const en = existsSync(EN) ? JSON.parse(readFileSync(EN, 'utf8')) : {}
const added = [...keys].filter((k) => !(k in en)).sort()
if (!added.length) {
  console.log('✅ en.json 已覆盖全部 t() key,无新增')
  process.exit(0)
}
console.log(`新增 ${added.length} 个 key:`)
for (const k of added) console.log(`  "${k}": ""`)

if (WRITE) {
  for (const k of added) en[k] = ''
  writeFileSync(EN, JSON.stringify(en, null, 2) + '\n')
  console.log(`✅ 已写入 ${EN}（共 ${Object.keys(en).length} 个 key）`)
}
