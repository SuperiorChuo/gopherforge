import { defineConfig } from 'vitepress'

// GopherForge 文档站：与在线 Demo 同一 GitHub Pages 站点，
// Demo 在 /gopherforge/，文档在 /gopherforge/docs/（deploy-demo 工作流合并产物）。
// 双语：根路径中文，/en/ 英文。

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
        { text: '监控与可观测', link: '/modules/observability' },
      ],
    },
  ],
  '/reference/': [
    {
      text: '参考',
      items: [
        { text: '生产部署', link: '/reference/deployment' },
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
        { text: 'Monitoring & Observability', link: '/en/modules/observability' },
      ],
    },
  ],
  '/en/reference/': [
    {
      text: 'Reference',
      items: [
        { text: 'Production Deployment', link: '/en/reference/deployment' },
        { text: 'Comparison', link: '/en/reference/comparison' },
      ],
    },
  ],
}

export default defineConfig({
  title: 'GopherForge',
  description:
    'GopherForge 开源 Go 微服务后台管理系统脚手架文档：快速上手、架构设计、RBAC 权限、多租户、审批流、代码生成器与二次开发指南',
  base: '/gopherforge/docs/',
  // Algolia DocSearch 域名所有权验证
  head: [['meta', { name: 'algolia-site-verification', content: 'F83830458D62489E' }]],
  // 教程里的 localhost 入口地址不是死链
  ignoreDeadLinks: [/^https?:\/\/localhost/],
  lastUpdated: true,
  sitemap: { hostname: 'https://superiorchuo.github.io/gopherforge/docs/' },
  locales: {
    root: {
      label: '简体中文',
      lang: 'zh-CN',
      themeConfig: {
        nav: [
          { text: '指南', link: '/guide/getting-started' },
          { text: '功能模块', link: '/modules/auth' },
          { text: '参考', link: '/reference/deployment' },
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
      description:
        'GopherForge — an open-source Go microservices admin scaffold: quick start, architecture, RBAC, multi-tenancy, workflow engine, code generator and extension guide',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/en/guide/getting-started' },
          { text: 'Modules', link: '/en/modules/auth' },
          { text: 'Reference', link: '/en/reference/deployment' },
          { text: 'Live Demo', link: 'https://superiorchuo.github.io/gopherforge/' },
        ],
        sidebar: enSidebar,
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
    siteTitle: 'GopherForge 文档',
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
