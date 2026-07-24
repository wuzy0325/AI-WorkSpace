// Electron IPC 桥接 —— Win7 分支替代 Wails v3 的文件选择对话框。
//
// 设计要点：
//   - 主进程（main.cjs）通过 ipcMain.handle 注册 dialog:pick-file / dialog:pick-files
//   - preload.cjs 通过 contextBridge 暴露 window.electronAPI.pickFile / pickFiles
//   - 渲染进程调用此 bridge 触发原生文件选择对话框，返回选中文件绝对路径
//
// 与原 Wails 版差异：
//   - Wails 通过 application.Dialog 文件选择，返回 OpenDialogResult
//   - Win7 版精简为只返回文件路径数组/单字符串，调用方按需处理
//
// 文件过滤器（filters）格式与 Electron FileFilter 一致：
//   [{ name: '校准文件', extensions: ['prb'] }, { name: 'CSV 文件', extensions: ['csv', 'txt', 'dat'] }]

/** Electron 文件过滤器（与 main.cjs dialog.showOpenDialog filters 参数对应） */
export interface ElectronFileFilter {
  name: string
  extensions: string[]
}

/** 文件选择对话框选项 */
export interface PickFileOptions {
  title?: string
  filters?: ElectronFileFilter[]
}

/** 单选文件对话框返回值：用户取消返回空字符串 */
export type PickFileResult = string

/** 多选文件对话框返回值：用户取消返回空数组 */
export type PickFilesResult = string[]

/** Electron preload 注入的 API 形状（与 preload.cjs contextBridge.exposeInMainWorld 对应） */
interface ElectronAPI {
  pickFile: (opts?: PickFileOptions) => Promise<PickFileResult>
  pickFiles: (opts?: PickFileOptions) => Promise<PickFilesResult>
}

/** window.electronAPI 类型扩展（与 env.d.ts 中的 declare global 互补） */
declare global {
  interface Window {
    electronAPI?: ElectronAPI
  }
}

/**
 * 单选文件对话框。
 *
 * @param opts 标题与过滤器配置
 * @returns 选中文件的绝对路径；用户取消返回空字符串
 *
 * 调用方应在收到空字符串时中止后续操作（如 LoadThreeHolePrbFiles / ImportCsvData）。
 */
export async function pickFile(opts?: PickFileOptions): Promise<PickFileResult> {
  if (!window.electronAPI?.pickFile) {
    throw new Error('Electron bridge 不可用：window.electronAPI.pickFile 未注入')
  }
  return window.electronAPI.pickFile(opts)
}

/**
 * 多选文件对话框。
 *
 * @param opts 标题与过滤器配置
 * @returns 选中文件的绝对路径数组；用户取消返回空数组
 *
 * 用于 5 孔 / 7 孔 LoadPrbFiles（一次加载多个 .prb 校准文件）。
 */
export async function pickFiles(opts?: PickFileOptions): Promise<PickFilesResult> {
  if (!window.electronAPI?.pickFiles) {
    throw new Error('Electron bridge 不可用：window.electronAPI.pickFiles 未注入')
  }
  return window.electronAPI.pickFiles(opts)
}

/** PRB 文件过滤器（5/3/7 孔通用） */
export const PRB_FILTERS: ElectronFileFilter[] = [
  { name: '校准文件 (*.prb)', extensions: ['prb'] },
  { name: '所有文件', extensions: ['*'] },
]

/** CSV 文件过滤器（5/3/7 孔通用，含 txt/dat 兼容老数据） */
export const CSV_FILTERS: ElectronFileFilter[] = [
  { name: '数据文件 (*.csv;*.txt;*.dat)', extensions: ['csv', 'txt', 'dat'] },
  { name: '所有文件', extensions: ['*'] },
]
