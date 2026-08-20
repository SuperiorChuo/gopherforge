export const SITE_META = {
  name: 'GopherForge',
  release: {
    version: '0.6.0',
    tag: 'v0.6.0',
    url: 'https://github.com/SuperiorChuo/gopherforge/releases/tag/v0.6.0',
  },
  urls: {
    repository: 'https://github.com/SuperiorChuo/gopherforge',
    demo: 'https://superiorchuo.github.io/gopherforge/',
    docs: 'https://superiorchuo.github.io/gopherforge/docs',
  },
  architecture: {
    goServices: 7,
    releaseImages: 8,
  },
  stack: {
    go: '1.26.5',
    node: '24',
    react: '19',
    antd: '6',
    postgres: '18',
  },
  ports: {
    gateway: 8000,
    frontend: 3000,
    vite: 5174,
    viteLan: 13200,
  },
  composeProfiles: {
    monitoring: 'monitoring',
    tracing: 'tracing',
  },
  homepageCommands: ['make compose-up', 'make smoke-api', 'make test'],
} as const
