import path from 'path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  base: process.env.VITE_BASE || '/',
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        // rolldown 对自动拆分出的共享块会生成 name~name~… 形式的长名
        // （可超百字符），截回首段，靠 hash 区分文件
        chunkFileNames: (info) => `assets/${info.name.split('~')[0]}-[hash].js`,
        assetFileNames: (info) => {
          const base = (info.names[0] ?? 'asset').replace(/\.[^.]+$/, '').split('~')[0]
          return `assets/${base}-[hash][extname]`
        },
        // 只把「被单个懒加载路由独占的大依赖」单列成组：它们本就不进首屏，
        // 拆出来只是让路由包更小、缓存更稳。刻意不对 antd/vendor 做分组——
        // 实测把跨路由共享模块并成大块会放大过量拉取，首屏 JS 从 993KB 涨到
        // 1218KB（Vite 的自动拆分在本项目已接近最优）。
        codeSplitting: {
          groups: [
            { name: 'geo-data', test: /src[\\/]assets[\\/](china-provinces|world-countries)\.json/, priority: 40 },
            { name: 'sip', test: /node_modules[\\/]sip\.js[\\/]/, priority: 30 },
            // 刻意不给 three/@react-three 分组：实测加 { name:'three' } 组后
            // rolldown 会把该组的共享依赖（react、react-dom）一并并入，导致所有
            // chunk 都静态引用这个 1.1MB 的块，它进而被 modulepreload 进首屏，
            // 懒加载完全失效。交给自动拆分即可——three 只会落在 office 路由块里。
            // 图标不分组时被自动拆成 86 个 1-3KB 碎片，进管理台一次触发
            // ~46 个请求；并成单块后是一个长缓存文件（gzip 后 ~35KB）。
            // 取舍：登录页原本只拉自己用的两三个图标碎片，现在也拉整块——
            // 但该块与 MainLayout 空闲预取共享缓存，等于把预取提前。
            { name: 'icons', test: /node_modules[\\/]@ant-design[\\/]icons/, priority: 20 },
          ],
        },
      },
    },
  },
  server: {
    port: Number(process.env.VITE_DEV_PORT || 5174),
    host: true,
    proxy: process.env.VITE_DEMO === '1' ? undefined : {
      // 本地默认 8000；远程 dev:lan 可通过 VITE_DEV_API_TARGET 指到网关
      '/api': {
        target: process.env.VITE_DEV_API_TARGET || 'http://localhost:8000',
        changeOrigin: true,
        ws: true,
      },
      '/im': {
        target: process.env.VITE_DEV_API_TARGET || 'http://localhost:8000',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
