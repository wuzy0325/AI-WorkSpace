/** Wails 桌面环境绑定类型声明。 */
declare global {
interface Window {
    go: {
      main: {
        App: {
          GetAPIPort: () => Promise<number>
          AppendFrontendLog: (message: string) => Promise<void>
          ShowSaveFilePath: (defaultName: string, filterName: string, filterPattern: string) => Promise<string>
        }
      }
    }
  }
}

export {}
