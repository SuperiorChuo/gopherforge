/**
 * GitHub Pages 深链接预渲染：把 dist/index.html 复制到每个已知路由目录
 * （dist/dashboard/index.html、dist/system/user/index.html …），
 * 让 /go-admin-kit/dashboard 这类直链命中真实文件返回 200，
 * 而不是靠 404.html 兜底（那样状态码是 404，会被 CDN/浏览器按 404 缓存，
 * 用户看起来就是"页面 404"）。404.html 仍保留，只兜未知路径。
 *
 * 路由清单直接从 src/router/index.tsx 提取，避免两处维护。
 */
import { copyFileSync, mkdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = dirname(dirname(fileURLToPath(import.meta.url)))

const routerSrc = readFileSync(join(webRoot, 'src/router/index.tsx'), 'utf8')
const routePaths = [...routerSrc.matchAll(/path:\s*'([^']+)'/g)]
  .map((m) => m[1])
  .filter((p) => p !== '*' && p !== '/')
  .map((p) => p.replace(/^\//, ''))

// 路由表结构变了导致提取不到，宁可让部署失败也不要静默退化回 404 兜底
if (routePaths.length < 10) {
  console.error(`路由提取异常：仅 ${routePaths.length} 条（${routePaths.join(', ')}），请检查 src/router/index.tsx`)
  process.exit(1)
}

const dist = join(webRoot, 'dist')
const indexHtml = join(dist, 'index.html')
for (const p of routePaths) {
  const dir = join(dist, p)
  mkdirSync(dir, { recursive: true })
  copyFileSync(indexHtml, join(dir, 'index.html'))
}
console.log(`已为 ${routePaths.length} 个路由预渲染 index.html：${routePaths.join(', ')}`)
