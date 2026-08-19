import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const read = (path) => readFile(new URL(path, import.meta.url), 'utf8')

const [
  themeEntry,
  foundation,
  homeStyles,
  docsStyles,
  responsive,
  heroComponent,
  journeyComponent,
  zhHome,
  enHome,
  config,
  zhArchitecture,
  enArchitecture,
  appTokens,
  zhDist,
  enDist,
] = await Promise.all([
  read('../.vitepress/theme/index.ts'),
  read('../.vitepress/theme/styles/foundation.css'),
  read('../.vitepress/theme/styles/home.css'),
  read('../.vitepress/theme/styles/docs.css'),
  read('../.vitepress/theme/styles/responsive.css'),
  read('../.vitepress/theme/components/HeroForge.vue'),
  read('../.vitepress/theme/components/HomeJourney.vue'),
  read('../index.md'),
  read('../en/index.md'),
  read('../.vitepress/config.mts'),
  read('../guide/architecture.md'),
  read('../en/guide/architecture.md'),
  read('../../microservices/web/src/index.css'),
  read('../.vitepress/dist/index.html'),
  read('../.vitepress/dist/en/index.html'),
])

test('文档站沿用控制台品牌色并拆分主题职责', () => {
  for (const path of [
    './styles/foundation.css',
    './styles/home.css',
    './styles/docs.css',
    './styles/responsive.css',
  ]) {
    assert.match(themeEntry, new RegExp(path.replaceAll('.', '\\.')))
  }

  for (const color of ['#6366f1', '#7c3aed', '#0ea5e9']) {
    assert.ok(`${foundation}${homeStyles}`.toLowerCase().includes(color), `文档主题缺少品牌色 ${color}`)
  }
  assert.match(appTokens.toLowerCase(), /--c-primary:\s*#6366f1/)
})

test('中英文首页具备同构 Hero、六项能力与独立交付路径', () => {
  for (const [name, markdown] of [['中文', zhHome], ['英文', enHome]]) {
    assert.match(markdown, /pageClass: gopherforge-home/)
    assert.equal((markdown.match(/^  - icon:/gm) ?? []).length, 6, `${name}首页能力卡数量应为 6`)
    assert.match(markdown, /hero:\n/)
  }

  assert.match(themeEntry, /home-hero-image/)
  assert.match(themeEntry, /home-features-after/)
  assert.match(heroComponent, /hero-forge-window/)
  assert.match(journeyComponent, /withBase\(`\$\{link\}\.html`\)/)
})

test('品牌资源遵循 VitePress base 且语言首页回链由 locale 决定', () => {
  assert.match(themeEntry, /withBase\('\/brand\/gopherforge-mark\.svg'\)/)
  assert.match(heroComponent, /withBase\('\/brand\/gopherforge-mark\.svg'\)/)
  assert.doesNotMatch(`${themeEntry}${heroComponent}`, /\/gopherforge\/docs\/brand/)
  assert.doesNotMatch(config, /logoLink:/)
  assert.match(config, /siteTitle: 'GopherForge 文档'/)
  assert.match(config, /siteTitle: 'GopherForge Docs'/)

  assert.match(zhDist, /class="title" href="\/gopherforge\/docs\/"/)
  assert.match(enDist, /class="title" href="\/gopherforge\/docs\/en\/"/)
  for (const html of [zhDist, enDist]) {
    assert.match(html, /src="\/gopherforge\/docs\/brand\/gopherforge-mark\.svg"/)
  }
  assert.match(zhDist, /href="\/gopherforge\/docs\/guide\/getting-started\.html"/)
  assert.match(enDist, /href="\/gopherforge\/docs\/en\/guide\/getting-started\.html"/)
})

test('移动菜单、窄屏与减少动效规则保持可用', () => {
  const navBlock = foundation.match(/\.VPNav \{[\s\S]*?\n\}/)?.[0] ?? ''
  assert.doesNotMatch(navBlock, /backdrop-filter/)
  assert.match(foundation, /\.VPNavBar \{[\s\S]*?backdrop-filter:/)
  assert.match(foundation, /\.VPNavScreen \{[\s\S]*?backdrop-filter:/)
  assert.match(responsive, /@media \(max-width: 639px\)/)
  assert.match(responsive, /@media \(prefers-reduced-motion: reduce\)/)
})

test('细节打磨：明暗过渡、滚动条、玻璃卡与发布元信息', () => {
  // 明暗渐变双伪元素交叉淡化，切换主题不跳变
  assert.match(foundation, /body::before,\nbody::after/)
  assert.match(foundation, /\.dark body::after \{ opacity: 1; \}/)
  assert.match(foundation, /body \{\n  min-width: 320px;\n  overflow-x: hidden;\n  background-color: var\(--vp-c-bg\);/)
  // 自定义滚动条明暗两套
  assert.match(foundation, /::-webkit-scrollbar-thumb/)
  assert.match(foundation, /\.dark \* \{ scrollbar-color:/)
  // 首页三列玻璃卡（唯一 ul 随 h2 精确命中）与 Hero/终端细节
  assert.match(homeStyles, /\.VPHome \.vp-doc h2 \+ ul \{/)
  assert.match(homeStyles, /\.VPHome \.vp-doc h2 \+ ul \{[\s\S]*?grid-template-columns: repeat\(3, 1fr\);/)
  assert.match(homeStyles, /\.hero-version \{/)
  assert.match(homeStyles, /\.terminal-cursor \{/)
  assert.match(responsive, /\.VPHome \.vp-doc h2 \+ ul \{ grid-template-columns: 1fr; \}/)
  // 正文玻璃卡细节与页脚渐变线
  assert.match(docsStyles, /\.vp-doc blockquote::before/)
  assert.match(docsStyles, /\.vp-doc h1::before/)
  assert.match(docsStyles, /\.VPFooter::before/)
  // 发布分享元信息
  assert.match(config, /property: 'og:title'/)
  assert.match(config, /property: 'og:image'/)
})

test('同源交互架构页保留 sandbox 且不组合 allow-scripts 与 allow-same-origin', () => {
  for (const markdown of [zhArchitecture, enArchitecture]) {
    assert.match(markdown, /sandbox="allow-scripts allow-downloads allow-popups"/)
    assert.doesNotMatch(markdown, /allow-same-origin/)
    assert.match(markdown, /allowfullscreen/)
  }
})
