/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

const workspaceRoot = fileURLToPath(new URL('../../../../..', import.meta.url))

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@components': fileURLToPath(new URL('./src/components', import.meta.url)),
      '@views': fileURLToPath(new URL('./src/views', import.meta.url)),
      '@stores': fileURLToPath(new URL('./src/stores', import.meta.url)),
      '@api': fileURLToPath(new URL('./src/api', import.meta.url)),
      '@styles': fileURLToPath(new URL('./src/styles', import.meta.url)),
      '@shared': fileURLToPath(new URL('./src/shared', import.meta.url)),
      // workspace 级共享前端模块（motion-utils 等），与 wind-daq 配置保持一致
      '@shared-frontend': fileURLToPath(new URL('../../../../../shared/frontend', import.meta.url)),
      '@composables': fileURLToPath(new URL('./src/composables', import.meta.url)),
    },
  },
  build: {
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      output: {
        manualChunks: {
          echarts: ['echarts', 'vue-echarts'],
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts'],
    env: {
      VITE_API_BASE: 'http://localhost:8080',
    },
  },
  server: {
    host: '127.0.0.1',
    port: 9245,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:8080',
    },
    // 允许 dev server 访问 workspace 根目录下的共享前端模块（@shared-frontend）
    fs: {
      allow: [workspaceRoot],
    },
  },
})
