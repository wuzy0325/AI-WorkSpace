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
      '@bridge': fileURLToPath(new URL('./src/bridge', import.meta.url)),
      '@composables': fileURLToPath(new URL('./src/composables', import.meta.url)),
      '@shared-frontend': fileURLToPath(new URL('../../../../../shared/frontend', import.meta.url)),
      // shared/frontend 目录向上找不到 node_modules，需显式映射 vue/pinia 到项目本地依赖，
      // 让 shared/*.ts 文件能被 Rollup 正确解析
      'vue': fileURLToPath(new URL('./node_modules/vue', import.meta.url)),
      'pinia': fileURLToPath(new URL('./node_modules/pinia', import.meta.url)),
      '@lucide/vue': fileURLToPath(new URL('./node_modules/@lucide/vue', import.meta.url)),
      'naive-ui': fileURLToPath(new URL('./node_modules/naive-ui', import.meta.url)),
      'naive-ui/es': fileURLToPath(new URL('./node_modules/naive-ui/es', import.meta.url)),
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
    // 默认环境用 node，避免额外引入 jsdom 依赖；
    // 需要 DOM 的测试可用 `// @vitest-environment jsdom` pragma 覆盖
    environment: 'node',
    globals: true,
    include: ['src/**/*.test.ts'],
  },
  server: {
    port: 15175,
    strictPort: true,
  },
})
