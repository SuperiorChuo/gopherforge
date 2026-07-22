import { defineConfig } from 'vitepress'

// GopherForge 文档站：与在线 Demo 同一 GitHub Pages 站点，
// Demo 在 /gopherforge/，文档在 /gopherforge/docs/（deploy-demo 工作流合并产物）。
export default defineConfig({
  lang: 'zh-CN',
  title: 'GopherForge',
  description:
    'GopherForge 开源 Go 微服务后台管理系统脚手架文档：快速上手、架构设计、RBAC 权限、多租户、审批流、代码生成器与二次开发指南',
  base: '/gopherforge/docs/',
  // 教程里的 localhost 入口地址不是死链
  ignoreDeadLinks: [/^https?:\/\/localhost/],
  lastUpdated: true,
  sitemap: { hostname: 'https://superiorchuo.github.io/gopherforge/docs/' },
  themeConfig: {
    siteTitle: 'GopherForge 文档',
    nav: [
      { text: '指南', link: '/guide/getting-started' },
      { text: '功能模块', link: '/modules/auth' },
      { text: '参考', link: '/reference/deployment' },
      { text: '在线 Demo', link: 'https://superiorchuo.github.io/gopherforge/' },
    ],
    sidebar: {
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
    },
    socialLinks: [{ icon: 'github', link: 'https://github.com/SuperiorChuo/gopherforge' }],
    footer: {
      message: 'MIT Licensed · 开源 Go 微服务后台管理脚手架',
      copyright: 'Copyright © 2026 GopherForge',
    },
    outline: { label: '本页目录', level: [2, 3] },
    docFooter: { prev: '上一篇', next: '下一篇' },
    lastUpdatedText: '最后更新',
    search: { provider: 'local' },
  },
})
