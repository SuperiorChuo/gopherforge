import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitepress'
import { SITE_META } from './site-meta'

// GopherForge 文档站：与在线 Demo 同一 GitHub Pages 站点，
// Demo 在 /gopherforge/，文档在 /gopherforge/docs/（deploy-demo 工作流合并产物）。
// 双语：根路径中文，/en/ 英文。

const zhDescription = 'GopherForge 开源 Go 微服务后台管理脚手架文档：快速上手、架构设计、RBAC 权限、多租户、审批流、代码生成器与二次开发指南'
const enDescription = 'GopherForge documentation for an open-source Go microservices admin scaffold: setup, architecture, RBAC, multi-tenancy, workflow, code generation and extension guides'

const publicPagePath = (relativePath: string) => {
  const htmlPath = relativePath.replace(/\.md$/, '.html')
  if (htmlPath === 'index.html') return ''
  return htmlPath.replace(/\/index\.html$/, '/')
}

const pageUrl = (relativePath: string) => `${SITE_META.urls.docs}/${publicPagePath(relativePath)}`
const counterpartPath = (relativePath: string, isEnglish: boolean) => isEnglish ? relativePath.slice(3) : `en/${relativePath}`
const sourcePageExists = (relativePath: string) => existsSync(fileURLToPath(new URL(`../${relativePath}`, import.meta.url)))

const zhSidebar = {
  '/guide/': [
    {
      text: '指南',
      items: [
        { text: '快速上手（15 分钟）', link: '/guide/getting-started' },
        { text: '架构总览', link: '/guide/architecture' },
        { text: '二次开发：加一个业务服务', link: '/guide/extend' },
      ],
    },
  ],
  '/frontend/': [
    {
      text: '前端开发',
      items: [
        { text: '前端架构总览', link: '/frontend/overview' },
        { text: '请求层与 API 封装', link: '/frontend/request' },
        { text: '路由与权限', link: '/frontend/routing' },
        { text: '页面开发规范', link: '/frontend/page-dev' },
        { text: '状态管理', link: '/frontend/state' },
        { text: '主题与样式', link: '/frontend/theme' },
        { text: '演示模式', link: '/frontend/demo' },
      ],
    },
  ],
  '/modules/': [
    {
      text: '功能模块',
      items: [
        { text: '认证与安全', link: '/modules/auth' },
        { text: 'RBAC 权限体系', link: '/modules/rbac' },
        { text: '多租户与套餐', link: '/modules/tenant' },
        { text: '系统配置与运营', link: '/modules/system' },
        { text: '代码生成器', link: '/modules/codegen' },
        { text: '审批流（BPM）', link: '/modules/bpm' },
        { text: 'Excel 导入导出', link: '/modules/excel' },
        { text: '文件服务', link: '/modules/file' },
        { text: '审计日志', link: '/modules/audit' },
        { text: '监控与可观测', link: '/modules/observability' },
      ],
    },
  ],
  '/reference/': [
    {
      text: '参考',
      items: [
        { text: 'API 参考', link: '/reference/api' },
        { text: '生产部署', link: '/reference/deployment' },
        { text: '版本升级', link: '/reference/upgrade' },
        { text: '常见问题 FAQ', link: '/reference/faq' },
        { text: '数据库表结构', link: '/reference/database' },
        { text: '同类项目对比', link: '/reference/comparison' },
      ],
    },
  ],
}

const enSidebar = {
  '/en/guide/': [
    {
      text: 'Guide',
      items: [
        { text: 'Getting Started (15 min)', link: '/en/guide/getting-started' },
        { text: 'Architecture', link: '/en/guide/architecture' },
        { text: 'Extending: Add a Service', link: '/en/guide/extend' },
      ],
    },
  ],
  '/en/frontend/': [
    {
      text: 'Frontend',
      items: [
        { text: 'Frontend Overview', link: '/en/frontend/overview' },
        { text: 'Request Layer & API', link: '/en/frontend/request' },
        { text: 'Routing & Permissions', link: '/en/frontend/routing' },
        { text: 'Page Development', link: '/en/frontend/page-dev' },
        { text: 'State Management', link: '/en/frontend/state' },
        { text: 'Theme & Styling', link: '/en/frontend/theme' },
        { text: 'Demo Mode', link: '/en/frontend/demo' },
      ],
    },
  ],
  '/en/modules/': [
    {
      text: 'Modules',
      items: [
        { text: 'Auth & Security', link: '/en/modules/auth' },
        { text: 'RBAC', link: '/en/modules/rbac' },
        { text: 'Multi-tenancy & Packages', link: '/en/modules/tenant' },
        { text: 'System Config & Ops', link: '/en/modules/system' },
        { text: 'Code Generator', link: '/en/modules/codegen' },
        { text: 'Workflow (BPM)', link: '/en/modules/bpm' },
        { text: 'Excel Import/Export', link: '/en/modules/excel' },
        { text: 'File Service', link: '/en/modules/file' },
        { text: 'Audit Logs', link: '/en/modules/audit' },
        { text: 'Monitoring & Observability', link: '/en/modules/observability' },
      ],
    },
  ],
  '/en/reference/': [
    {
      text: 'Reference',
      items: [
        { text: 'API Reference', link: '/en/reference/api' },
        { text: 'Production Deployment (Summary)', link: '/en/reference/deployment' },
        { text: 'Upgrading', link: '/en/reference/upgrade' },
        { text: 'FAQ', link: '/en/reference/faq' },
        { text: 'Database Schema', link: '/en/reference/database' },
        { text: 'Comparison (Summary)', link: '/en/reference/comparison' },
      ],
    },
  ],
}

export default defineConfig({
  title: 'GopherForge',
  description: zhDescription,
  base: '/gopherforge/docs/',
  // Algolia DocSearch 域名所有权验证 + 品牌图标/主题色
  head: [
    ['meta', { name: 'algolia-site-verification', content: 'F83830458D62489E' }],
    ['link', { rel: 'icon', href: '/gopherforge/docs/brand/gopherforge-mark.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#f6f8ff', media: '(prefers-color-scheme: light)' }],
    ['meta', { name: 'theme-color', content: '#070812', media: '(prefers-color-scheme: dark)' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],
  // 教程里的 localhost 入口地址不是死链
  ignoreDeadLinks: [/^https?:\/\/localhost/],
  lastUpdated: true,
  sitemap: { hostname: `${SITE_META.urls.docs}/` },
  transformHead({ pageData }) {
    const relativePath = pageData.relativePath
    const isEnglish = relativePath.startsWith('en/')
    const language = isEnglish ? 'en-US' : 'zh-CN'
    const locale = isEnglish ? 'en_US' : 'zh_CN'
    const siteTitle = isEnglish ? 'GopherForge Docs' : 'GopherForge 文档'
    const fallbackDescription = isEnglish ? enDescription : zhDescription
    const description = pageData.description || fallbackDescription
    const title = pageData.title ? `${pageData.title} | ${siteTitle}` : siteTitle
    const canonical = pageUrl(relativePath)
    const alternate = counterpartPath(relativePath, isEnglish)
    const socialImage = `${SITE_META.urls.docs}/brand/gopherforge-social.png`
    const isHomepage = relativePath === 'index.md' || relativePath === 'en/index.md'
    const structuredData = isHomepage ? {
      '@context': 'https://schema.org',
      '@type': 'SoftwareSourceCode',
      name: SITE_META.name,
      description,
      codeRepository: SITE_META.urls.repository,
      url: canonical,
      license: 'https://opensource.org/license/mit',
      programmingLanguage: 'Go',
      runtimePlatform: 'Docker Compose',
      version: SITE_META.release.version,
      inLanguage: language,
    } : {
      '@context': 'https://schema.org',
      '@type': 'TechArticle',
      headline: pageData.title || siteTitle,
      description,
      url: canonical,
      inLanguage: language,
      isPartOf: { '@type': 'WebSite', name: siteTitle, url: `${SITE_META.urls.docs}/${isEnglish ? 'en/' : ''}` },
      ...(pageData.lastUpdated ? { dateModified: new Date(pageData.lastUpdated).toISOString() } : {}),
    }
    const result = [
      ['link', { rel: 'canonical', href: canonical }],
      ['link', { rel: 'alternate', hreflang: language, href: canonical }],
      ['link', { rel: 'alternate', hreflang: 'x-default', href: pageUrl(isEnglish ? relativePath.slice(3) : relativePath) }],
      ['meta', { property: 'og:type', content: isHomepage ? 'website' : 'article' }],
      ['meta', { property: 'og:site_name', content: siteTitle }],
      ['meta', { property: 'og:locale', content: locale }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:url', content: canonical }],
      ['meta', { property: 'og:image', content: socialImage }],
      ['meta', { property: 'og:image:width', content: '1200' }],
      ['meta', { property: 'og:image:height', content: '630' }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
      ['meta', { name: 'twitter:image', content: socialImage }],
      ['script', { type: 'application/ld+json' }, JSON.stringify(structuredData)],
    ]
    if (sourcePageExists(alternate)) {
      result.splice(2, 0, ['link', {
        rel: 'alternate',
        hreflang: isEnglish ? 'zh-CN' : 'en-US',
        href: pageUrl(alternate),
      }])
    }
    return result
  },
  locales: {
    root: {
      label: '简体中文',
      lang: 'zh-CN',
      themeConfig: {
        siteTitle: 'GopherForge 文档',
        nav: [
          { text: '指南', link: '/guide/getting-started' },
          { text: '前端开发', link: '/frontend/overview' },
          { text: '功能模块', link: '/modules/auth' },
          { text: '参考', link: '/reference/deployment' },
          { text: '更新日志', link: '/changelog' },
          { text: '在线 Demo', link: 'https://superiorchuo.github.io/gopherforge/' },
        ],
        sidebar: zhSidebar,
        outline: { label: '本页目录', level: [2, 3] },
        docFooter: { prev: '上一篇', next: '下一篇' },
        lastUpdatedText: '最后更新',
        editLink: {
          pattern: 'https://github.com/SuperiorChuo/gopherforge/edit/main/website/:path',
          text: '在 GitHub 上编辑此页',
        },
        footer: {
          message: 'MIT Licensed · 开源 Go 微服务后台管理脚手架',
          copyright: 'Copyright © 2026 GopherForge',
        },
      },
    },
    en: {
      label: 'English',
      lang: 'en-US',
      description: enDescription,
      themeConfig: {
        siteTitle: 'GopherForge Docs',
        nav: [
          { text: 'Guide', link: '/en/guide/getting-started' },
          { text: 'Frontend', link: '/en/frontend/overview' },
          { text: 'Modules', link: '/en/modules/auth' },
          { text: 'Reference', link: '/en/reference/deployment' },
          { text: 'Changelog', link: '/en/changelog' },
          { text: 'Live Demo', link: 'https://superiorchuo.github.io/gopherforge/' },
        ],
        sidebar: enSidebar,
        outline: { label: 'On this page', level: [2, 3] },
        docFooter: { prev: 'Previous', next: 'Next' },
        lastUpdatedText: 'Last updated',
        editLink: {
          pattern: 'https://github.com/SuperiorChuo/gopherforge/edit/main/website/:path',
          text: 'Edit this page on GitHub',
        },
        footer: {
          message: 'MIT Licensed · Open-source Go microservices admin scaffold',
          copyright: 'Copyright © 2026 GopherForge',
        },
      },
    },
  },
  themeConfig: {
    socialLinks: [{ icon: 'github', link: 'https://github.com/SuperiorChuo/gopherforge' }],
    search: {
      provider: 'algolia',
      options: {
        // 索引由 website/scripts/algolia-index.mjs 在 deploy-demo 工作流中推送（自建管道，非官方 crawler）
        appId: '23Y7MRK7R7',
        apiKey: '299b2c1413f745126387be19bd58553f', // search-only 公钥，可安全入库
        indexName: 'gopherforge',
        locales: {
          root: {
            placeholder: '搜索文档',
            searchParameters: { facetFilters: ['lang:zh-CN'] },
            translations: {
              button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
              modal: {
                searchBox: {
                  resetButtonTitle: '清除查询条件',
                  resetButtonAriaLabel: '清除查询条件',
                  cancelButtonText: '取消',
                  cancelButtonAriaLabel: '取消',
                },
                startScreen: {
                  recentSearchesTitle: '搜索历史',
                  noRecentSearchesText: '没有搜索历史',
                  saveRecentSearchButtonTitle: '保存至搜索历史',
                  removeRecentSearchButtonTitle: '从搜索历史中移除',
                  favoriteSearchesTitle: '收藏',
                  removeFavoriteSearchButtonTitle: '从收藏中移除',
                },
                errorScreen: { titleText: '无法获取结果', helpText: '你可能需要检查网络连接' },
                footer: { selectText: '选择', navigateText: '切换', closeText: '关闭', searchByText: '搜索提供者' },
                noResultsScreen: {
                  noResultsText: '无法找到相关结果',
                  suggestedQueryText: '你可以尝试查询',
                  reportMissingResultsText: '你认为该查询应该有结果？',
                  reportMissingResultsLinkText: '点击反馈',
                },
              },
            },
          },
          en: {
            placeholder: 'Search docs',
            searchParameters: { facetFilters: ['lang:en-US'] },
          },
        },
      },
    },
  },
})
