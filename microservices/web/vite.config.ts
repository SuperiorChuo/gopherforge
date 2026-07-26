import path from 'path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  // 演示模式部署到 GitHub Pages 子路径时用 VITE_BASE=/go-admin-kit/
  base: process.env.VITE_BASE || '/',
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        // entriesAware 子组名形如 antd~index~index~…（112 字符），截回首段，
        // 靠 hash 区分文件
        chunkFileNames: (info) => `assets/${info.name.split('~')[0]}-[hash].js`,
        assetFileNames: (info) => {
          const base = (info.names[0] ?? 'asset').replace(/\.[^.]+$/, '').split('~')[0]
          return `assets/${base}-[hash][extname]`
        },
        // 依赖收敛为少数稳定命名 chunk（rolldown codeSplitting，Vite 8 下
        // 取代 manualChunks）：改业务代码不再抖动 vendor 哈希，路由间共享
        // 依赖不再碎成上百个自动小块。地图 GeoJSON 只被单页动态引用，
        // 单列成组保持懒加载不混进公共 vendor。
        codeSplitting: {
          groups: [
            { name: 'geo-data', test: /src[\\/]assets[\\/](china-provinces|world-countries)\.json/, priority: 40 },
            {
              name: 'react-vendor',
              test: /node_modules[\\/](react|react-dom|scheduler|react-router|react-router-dom|react-redux|@reduxjs|redux|use-sync-external-store)[\\/]/,
              priority: 20,
            },
            // antd 及其内部依赖较大：entriesAware 按「哪些入口真正用到」分子组，
            // 懒加载路由独享的组件不会被并进首屏切片（否则入口会 preload 整组）
            {
              name: 'antd',
              test: /node_modules[\\/](antd|@ant-design|@rc-component|rc-[^\\/]+)[\\/]/,
              priority: 15,
              entriesAware: true,
              entriesAwareMergeThreshold: 30_000,
              maxSize: 300_000,
            },
            { name: 'vendor', test: /node_modules[\\/]/, priority: 10, entriesAware: true, entriesAwareMergeThreshold: 30_000 },
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
