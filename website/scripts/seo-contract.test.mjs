import assert from 'node:assert/strict'
import { stat } from 'node:fs/promises'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const read = path => readFile(new URL(path, import.meta.url), 'utf8')
const [zhHome, enHome, zhGuide, enGuide, config] = await Promise.all([
  read('../.vitepress/dist/index.html'),
  read('../.vitepress/dist/en/index.html'),
  read('../.vitepress/dist/guide/getting-started.html'),
  read('../.vitepress/dist/en/guide/getting-started.html'),
  read('../.vitepress/config.mts'),
])

const attr = (html, property, value, attribute = 'content') => {
  const escaped = value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return html.match(new RegExp(`<[^>]+${property}="${escaped}"[^>]+${attribute}="([^"]+)"`))?.[1]
    ?? html.match(new RegExp(`<[^>]+${attribute}="([^"]+)"[^>]+${property}="${escaped}"`))?.[1]
}

test('中英文首页输出独立 canonical、hreflang 与 Open Graph 元数据', () => {
  assert.match(zhHome, /rel="canonical" href="https:\/\/superiorchuo\.github\.io\/gopherforge\/docs\/"/)
  assert.match(enHome, /rel="canonical" href="https:\/\/superiorchuo\.github\.io\/gopherforge\/docs\/en\/"/)
  for (const html of [zhHome, enHome]) {
    assert.match(html, /hreflang="zh-CN"/)
    assert.match(html, /hreflang="en-US"/)
    assert.match(html, /hreflang="x-default"/)
    assert.match(html, /gopherforge-social\.png/)
    assert.match(html, /property="og:image:width" content="1200"/)
    assert.match(html, /property="og:image:height" content="630"/)
  }
  assert.equal(attr(zhHome, 'property', 'og:locale'), 'zh_CN')
  assert.equal(attr(enHome, 'property', 'og:locale'), 'en_US')
  assert.notEqual(attr(zhHome, 'property', 'og:description'), attr(enHome, 'property', 'og:description'))
})

test('首页与正文输出对应 JSON-LD，关键页使用自己的 description', () => {
  for (const html of [zhHome, enHome]) assert.match(html, /"@type":"SoftwareSourceCode"/)
  for (const html of [zhGuide, enGuide]) assert.match(html, /"@type":"TechArticle"/)
  assert.match(zhGuide, /用 Docker Compose 拉起 GopherForge 网关/)
  assert.match(enGuide, /Start the GopherForge gateway/)
  assert.match(config, /sourcePageExists\(alternate\)/)
})

test('社交分享图尺寸固定为 1200×630', async () => {
  const image = await stat(new URL('../public/brand/gopherforge-social.png', import.meta.url))
  assert.ok(image.size > 20_000)
  const svg = await read('../public/brand/gopherforge-social.svg')
  assert.match(svg, /width="1200" height="630"/)
})
