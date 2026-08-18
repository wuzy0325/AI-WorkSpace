/// <reference types="vite/client" />

declare const __APP_VERSION__: string

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

/**
 * Electron preload 注入的全局 API（Win7 分支）。
 *
 * 详见 desktop-electron/preload.cjs：contextBridge.exposeInMainWorld('electronAPI', {...})。
 * 在浏览器开发环境（vite dev server）下此对象不存在，bridge 层会回退到空串。
 */
interface Window {
  electronAPI?: {
    /** 显示原生目录选择对话框，返回所选目录绝对路径（用户取消时返回空串） */
    showOpenDialog?: () => Promise<string>
  }
}
