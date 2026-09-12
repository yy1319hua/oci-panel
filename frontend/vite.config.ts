import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// 构建时间戳：每次构建固定一个值，保证本次构建内各产物一致、跨构建各不相同。
// 用途：让产物文件名随每次部署变化，避免 Cloudflare 等 CDN 命中同名旧资源的缓存。
const BUILD_TIME = Date.now()

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8999',
        changeOrigin: true
      },
      '/ws': {
        target: 'ws://localhost:8999',
        ws: true
      }
    }
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (
              id.includes('/node_modules/vue/') ||
              id.includes('/node_modules/@vue/') ||
              id.includes('/node_modules/pinia/') ||
              id.includes('/node_modules/vue-router/')
            ) {
              return 'vendor-vue'
            }
            if (id.includes('lucide') || id.includes('motion') || id.includes('radix')) {
              return 'vendor-ui'
            }
            return 'vendor'
          }
          if (id.includes('/components/ui/')) {
            return 'ui-components'
          }
        },
        // 加入 [time] 构建时间戳，保证每次构建产物文件名都不同，
        // 避免 Cloudflare 等 CDN 命中同名旧资源的缓存（内容哈希偶发碰撞时会吐旧文件）。
        chunkFileNames: (chunkInfo) => `assets/${chunkInfo.name}-[hash]-${BUILD_TIME}.js`,
        entryFileNames: (chunkInfo) => `assets/${chunkInfo.name}-[hash]-${BUILD_TIME}.js`,
        assetFileNames: (assetInfo) => `assets/${assetInfo.name || 'asset'}-[hash]-${BUILD_TIME}.[ext]`
      }
    },
    chunkSizeWarningLimit: 1000
  }
})
