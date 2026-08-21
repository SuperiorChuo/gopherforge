import path from 'path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  // 演示模式部署到 GitHub Pages 子路径时用 VITE_BASE=/go-admin-kit/
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
        // 只把「被单个懒加载路由独占的大依赖」单列成组：地图 GeoJSON 本就不进
        // 首屏，拆出来只是让路由包更小。刻意不对 antd/vendor 做分组——实测把
        // 跨路由共享模块并成大块会放大过量拉取，首屏 JS 反而涨约 25%
        // （Vite 的自动拆分在本项目已接近最优）。
        codeSplitting: {
          groups: [
            { name: 'geo-data', test: /src[\\/]assets[\\/](china-provinces|world-countries)\.json/, priority: 40 },
            // 图标不分组时被自动拆成大量 1-3KB 碎片，进管理台一次触发几十个
            // 请求；并成单块后是一个长缓存文件（gzip 后约 35KB）。取舍：登录页
            // 原本只拉自己用的两三个图标碎片，现在也拉整块——但该块与
            // MainLayout 空闲预取共享缓存，等于把预取提前。
            { name: 'icons', test: /node_modules[\\/]@ant-design[\\/]icons/, priority: 20 },
          ],
        },
      },
    },
  },
  server: {
    port: Number(process.env.VITE_DEV_PORT || 5174),
    host: true,
    proxy: {
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
