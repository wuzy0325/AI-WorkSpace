/**
 * useFileSaveDialog — 文件"另存为"对话框。
 *
 * 桌面模式（Wails）：调用 Go 绑定的 ShowSaveFilePath
 * Web 模式：优先使用 showSaveFilePicker，否则返回默认文件名
 */

// showSaveFilePicker 的 TS 类型声明（File System Access API，非 lib 标准）
interface FilePickerAcceptType {
  description?: string
  accept: Record<string, string | string[]>
}
interface SaveFilePickerOptions {
  suggestedName?: string
  types?: FilePickerAcceptType[]
}
interface FileSystemFileHandle {
  name: string
  createWritable(): Promise<FileSystemWritableFileStream>
}
interface FileSystemWritableFileStream extends WritableStream {
  write(data: unknown): Promise<void>
  close(): Promise<void>
}

function isWails(): boolean {
  const w = window as unknown as { go?: { main?: { App?: { ShowSaveFilePath: unknown } } } }
  return typeof w.go?.main?.App?.ShowSaveFilePath === 'function'
}

/** 弹出"另存为"对话框，返回用户选择的路径（取消时返回空字符串）。 */
export async function showSaveDialog(
  defaultName: string,
  filterName: string,
  filterPattern: string,
): Promise<string> {
  if (isWails()) {
    const w = window as unknown as { go: { main: { App: { ShowSaveFilePath: (a: string, b: string, c: string) => Promise<string> } } } }
    return w.go.main.App.ShowSaveFilePath(defaultName, filterName, filterPattern)
  }

  // Web 模式：showSaveFilePicker (Chromium 86+, Edge)
  try {
    const opts: SaveFilePickerOptions = {
      suggestedName: defaultName,
      types: [
        {
          description: filterName,
          accept: { [mimeFromPattern(filterPattern)]: [filterPattern] },
        },
      ],
    }
    const w = window as unknown as Window & { showSaveFilePicker: (opts: SaveFilePickerOptions) => Promise<FileSystemFileHandle> }
    const handle = await w.showSaveFilePicker(opts)
    return handle.name
  } catch {
    // 浏览器不支持或用户取消
    return ''
  }
}

function mimeFromPattern(pattern: string): string {
  if (pattern.includes('xlsx') || pattern.includes('xls')) return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  if (pattern.includes('csv')) return 'text/csv'
  return 'application/octet-stream'
}

/** 触发浏览器下载（用于 Web 模式无对话框的场景）。 */
export function triggerBrowserDownload(filename: string, blobOrUrl: Blob | string): void {
  const a = document.createElement('a')
  if (blobOrUrl instanceof Blob) {
    a.href = URL.createObjectURL(blobOrUrl)
  } else {
    a.href = blobOrUrl
  }
  a.download = filename
  a.click()
}
