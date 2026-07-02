/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

const workspaceRoot = fileURLToPath(new URL('../../../../..', import.meta.url))

const wailsUnsafeHeaders = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
  'etag',
  'last-modified',
  'content-length',
  'date',
  'vary',
  'cache-control',
])

function stripWailsUnsafeDevHeaders() {
  return {
    name: 'strip-wails-unsafe-dev-headers',
    apply: 'serve' as const,
    configureServer(server: import('vite').ViteDevServer) {
      server.middlewares.use((_req, res, next) => {
        const stripHeaders = () => {
          for (const header of wailsUnsafeHeaders) {
            res.removeHeader(header)
          }
        }

        const setHeader = res.setHeader.bind(res)
        res.setHeader = (name, value) => {
          if (wailsUnsafeHeaders.has(String(name).toLowerCase())) {
            return res
          }
          return setHeader(name, value)
        }

        const writeHead = res.writeHead.bind(res)
        res.writeHead = ((...args: Parameters<typeof res.writeHead>) => {
          stripHeaders()
          return writeHead(...args)
        }) as typeof res.writeHead

        next()
      })
    },
  }
}

export default defineConfig({
  plugins: [stripWailsUnsafeDevHeaders(), vue()],
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
      '@utils': fileURLToPath(new URL('./src/utils', import.meta.url)),
      'naive-ui': fileURLToPath(new URL('./node_modules/naive-ui', import.meta.url)),
      'naive-ui/es': fileURLToPath(new URL('./node_modules/naive-ui/es', import.meta.url)),
    },
  },
  build: {
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      // 注意：曾经在这里通过 manualChunks 把 echarts 单独拆出来，
      // 但被 Vite 视为关键依赖会自动加 <link rel="modulepreload">，
      // 导致首屏即使不需要图表也下载 echarts（拖慢 LCP）。
      // 移除手动分块后，echarts 会跟随其异步消费者（DeviceDetailPanel 的 RealtimeChart，
      // traversal 可视化视图）自然分到 lazy chunk，仅在进入相关视图时下载。
      // 详见 docs/runbooks/perf-frontend-bundle-baseline.md。
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
    // 端口固定为 9246：wind-daq 专属 dev server 端口。
    // 之前与 daq-t1603 / motion-controller 共用 9245，切换项目时残留进程会占端口，
    // 配合 strictPort=true 直接启动失败并触发 libuv UV_HANDLE_CLOSING 断言。
    port: 9246,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:8080',
    },
    fs: {
      allow: [workspaceRoot],
    },
  },
  css: {
    devSourcemap: false,
  },
})
