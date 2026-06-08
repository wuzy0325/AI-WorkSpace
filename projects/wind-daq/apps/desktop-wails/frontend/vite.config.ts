/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

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
      '@shared/motion': fileURLToPath(new URL('../../../../motion-controller/shared/frontend/motion/src', import.meta.url)),
      '@shared-frontend': fileURLToPath(new URL('../../../../../shared/frontend', import.meta.url)),
      '@composables': fileURLToPath(new URL('./src/composables', import.meta.url)),
      'naive-ui': fileURLToPath(new URL('./node_modules/naive-ui', import.meta.url)),
      'naive-ui/es': fileURLToPath(new URL('./node_modules/naive-ui/es', import.meta.url)),
    },
  },
  build: {
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      external: [
        '../wailsjs/go/backend/App',
        '../wailsjs/go/models',
      ],
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
    port: 15173,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
