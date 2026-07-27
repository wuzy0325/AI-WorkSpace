/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import pkg from './package.json'

// Wails v3 frontend Vite 配置
//   - 端口由 wails3 dev 通过 WAILS_VITE_PORT 注入；
//   - 不再需要 rollupOptions.external 排除 ../wailsjs/*，
//     绑定改为 wails3 generate bindings 产物（默认 frontend/bindings/）。
export default defineConfig({
  plugins: [vue()],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
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
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.test.ts'],
  },
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
})
