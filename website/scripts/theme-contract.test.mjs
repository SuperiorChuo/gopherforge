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
  releaseComponent,
  showcaseComponent,
  zhHome,
  enHome,
  config,
  zhArchitecture,
  enArchitecture,
  appTokens,
  zhDist,
  enDist,
  siteMeta,
  webPackage,
  webViteConfig,
  rootMakefile,
  microMakefile,
  releaseWorkflow,
  zhGettingStarted,
  enGettingStarted,
  zhFrontendOverview,
  enFrontendOverview,
  zhObservability,
  enObservability,
  zhDeployment,
  enDeployment,
  changelog,
] = await Promise.all([
  read('../.vitepress/theme/index.ts'),
  read('../.vitepress/theme/styles/foundation.css'),
  read('../.vitepress/theme/styles/home.css'),
  read('../.vitepress/theme/styles/docs.css'),
  read('../.vitepress/theme/styles/responsive.css'),
  read('../.vitepress/theme/components/HeroForge.vue'),
  read('../.vitepress/theme/components/HomeJourney.vue'),
  read('../.vitepress/theme/components/HomeReleaseBar.vue'),
  read('../.vitepress/theme/components/HomeShowcase.vue'),
  read('../index.md'),
  read('../en/index.md'),
  read('../.vitepress/config.mts'),
  read('../guide/architecture.md'),
  read('../en/guide/architecture.md'),
  read('../../microservices/web/src/index.css'),
  read('../.vitepress/dist/index.html'),
  read('../.vitepress/dist/en/index.html'),
  read('../.vitepress/site-meta.ts'),
  read('../../microservices/web/package.json'),
  read('../../microservices/web/vite.config.ts'),
  read('../../Makefile'),
  read('../../microservices/Makefile'),
  read('../../.github/workflows/release.yml'),
  read('../guide/getting-started.md'),
  read('../en/guide/getting-started.md'),
  read('../frontend/overview.md'),
  read('../en/frontend/overview.md'),
  read('../modules/observability.md'),
  read('../en/modules/observability.md'),
  read('../reference/deployment.md'),
  read('../en/reference/deployment.md'),
  read('../../CHANGELOG.md'),
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

test('细节打磨：首页信息收敛、明暗过渡与正文玻璃细节', () => {
  // 明暗渐变双伪元素交叉淡化，切换主题不跳变
  assert.match(foundation, /body::before,\nbody::after/)
  assert.match(foundation, /\.dark body::after \{ opacity: 1; \}/)
  assert.match(foundation, /body \{\n  min-width: 320px;\n  overflow-x: hidden;\n  background-color: var\(--vp-c-bg\);/)
  // 自定义滚动条明暗两套
  assert.match(foundation, /::-webkit-scrollbar-thumb/)
  assert.match(foundation, /\.dark \* \{ scrollbar-color:/)
  // 版本边界前置，上手路径合并资源入口，截图跟随主题
  assert.match(themeEntry, /home-hero-after/)
  assert.match(releaseComponent, /SITE_META\.release\.tag/)
  assert.match(releaseComponent, /Live Demo uses mock data|在线 Demo 使用演示数据/)
  assert.match(journeyComponent, /journey-resources/)
  assert.match(showcaseComponent, /isDark\.value/)
  assert.match(showcaseComponent, /width="1432" height="895"/)
  assert.match(homeStyles, /\.home-release-bar \{/)
  assert.match(homeStyles, /\.journey-resources \{[\s\S]*?grid-template-columns: repeat\(3, 1fr\);/)
  assert.match(responsive, /\.journey-resources \{ grid-template-columns: 1fr; \}/)
  assert.match(responsive, /\.hero-float-card,\n  \.hero-metrics \{ display: none; \}/)
  assert.doesNotMatch(`${zhHome}${enHome}`, /^## /m)
  // 正文降低整页滤镜负担，宽屏提高有效阅读宽度
  assert.match(foundation, /--gf-article-bg:/)
  assert.match(docsStyles, /background: var\(--gf-article-bg\)/)
  assert.match(docsStyles, /backdrop-filter: blur\(14px\) saturate\(1\.25\)/)
  assert.match(docsStyles, /\.VPDoc\.has-aside \.content-container \{ max-width: 760px !important; \}/)
  assert.match(responsive, /\.glass-aurora \{ animation: none;/)
  assert.match(responsive, /\.glass-aurora-three,\n  \.glass-noise \{ display: none; \}/)
  assert.match(responsive, /backdrop-filter: none;/)
  assert.match(docsStyles, /\.vp-doc blockquote::before/)
  assert.match(docsStyles, /\.vp-doc h1::before/)
  assert.match(docsStyles, /\.VPFooter::before/)
  // 发布分享元信息
  assert.match(config, /property: 'og:title'/)
  assert.match(config, /property: 'og:image'/)
})

test('公开事实与仓库真源一致，首页不展示虚构命令或状态', () => {
  const pkg = JSON.parse(webPackage)
  const routerMajor = Number(pkg.dependencies['react-router-dom'].match(/\d+/)?.[0])
  assert.equal(routerMajor, 7)
  for (const overview of [zhFrontendOverview, enFrontendOverview]) {
    assert.match(overview, new RegExp(`react-router-dom ${routerMajor}`))
    assert.match(overview, /5174/)
  }
  assert.match(webViteConfig, /VITE_DEV_PORT \|\| 5174/)

  for (const command of ['compose-up', 'smoke-api', 'test']) {
    assert.match(`${rootMakefile}\n${microMakefile}`, new RegExp(`(?:^|[\\s:])${command}(?:[\\s:]|$)`, 'm'))
  }
  assert.match(journeyComponent, /SITE_META\.homepageCommands/)
  assert.doesNotMatch(journeyComponent, /['"]make (?:smoke|verify)['"]/)

  assert.match(siteMeta, /goServices: 7/)
  assert.match(siteMeta, /releaseImages: 8/)
  assert.match(heroComponent, /SITE_META\.architecture\.goServices/)
  assert.doesNotMatch(heroComponent, /All systems operational|全系统运行正常|100%|>LIVE</)

  for (const page of [zhObservability, enObservability]) {
    assert.match(page, /--profile monitoring/)
    assert.doesNotMatch(page, /--profile observability/)
  }
  assert.match(releaseWorkflow, /v0\.3\.0 起双架构/)
  for (const page of [zhDeployment, enDeployment]) {
    assert.match(page, /v0\.3\.0/)
    assert.doesNotMatch(page, /v0\.4\.0/)
  }
  for (const page of [zhGettingStarted, enGettingStarted]) {
    assert.match(page, /Docker Engine \*\*24\+\*\*/)
    assert.doesNotMatch(page, /唯一硬依赖|only hard requirement/)
  }

  const latestStable = changelog.match(/^## \[(\d+\.\d+\.\d+)\]/m)?.[1]
  assert.ok(latestStable)
  assert.match(siteMeta, new RegExp(`version: '${latestStable}'`))
})

test('同源交互架构页保留 sandbox 且不组合 allow-scripts 与 allow-same-origin', () => {
  for (const markdown of [zhArchitecture, enArchitecture]) {
    assert.match(markdown, /sandbox="allow-scripts allow-downloads allow-popups"/)
    assert.doesNotMatch(markdown, /allow-same-origin/)
    assert.match(markdown, /loading="lazy"/)
    assert.doesNotMatch(markdown, /loading="eager"/)
    assert.match(markdown, /allowfullscreen/)
  }
})
