/**
 * 工作台快捷键 composable（P1-8）
 *
 * 提供统一的键盘快捷键支持，标定与计量两个工作台共用。
 * 快捷键设计：
 * - Space：开始 / 暂停切换（idle 态开始，running 态暂停）
 * - Esc：停止当前操作
 * - Ctrl+E：导出报告
 * - Ctrl+S：阻止浏览器默认保存（避免误触弹窗干扰采集）
 *
 * 浮层拦截策略（分层，避免误伤非模态浮层）：
 * - 模态浮层（.el-overlay：el-dialog / el-drawer）：跳过所有快捷键，
 *   因为模态浮层独占交互焦点，工作台操作不应穿透。
 * - 非模态浮层（.el-select-dropdown / .el-popper：下拉 / tooltip / popover）：
 *   仅 Esc 跳过（让浮层自己关闭），Space / Ctrl+E / Ctrl+S 不跳过。
 *   否则用户 hover 带 tooltip 的按钮时按 Space 会失效（tooltip 不监听键盘，
 *   不应阻塞 Space）。典型场景：CalibrationControl 主按钮 disabled 时
 *   用 el-tooltip 提示原因，用户 hover 查看 tooltip 时仍需 Space 可用。
 */
import { onMounted, onUnmounted } from 'vue'

export interface WorkbenchShortcutHandlers {
  /** Space 键：开始/暂停切换 */
  onSpace: () => void
  /** Esc 键：停止 */
  onEscape: () => void
  /** Ctrl+E：导出报告 */
  onExport: () => void
}

export function useWorkbenchShortcuts(handlers: WorkbenchShortcutHandlers): void {
  const onKeydown = (e: KeyboardEvent): void => {
    // 焦点在输入控件时不拦截，避免影响用户输入
    const target = e.target as HTMLElement
    if (
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.isContentEditable
    ) {
      return
    }

    // 模态浮层（dialog / drawer）打开时跳过所有快捷键：
    // 模态独占交互焦点，工作台操作不应穿透。
    if (document.querySelector('.el-overlay')) return

    // Ctrl+S：阻止浏览器默认保存弹窗
    if (e.ctrlKey && e.code === 'KeyS') {
      e.preventDefault()
      return
    }

    // Ctrl+E：导出报告
    if (e.ctrlKey && e.code === 'KeyE') {
      e.preventDefault()
      handlers.onExport()
      return
    }

    // Space：开始/暂停切换
    if (e.code === 'Space') {
      e.preventDefault()
      handlers.onSpace()
      return
    }

    // Esc：停止。
    // 非模态浮层（下拉 / tooltip / popover）打开时仅 Esc 跳过，
    // 让浮层自己关闭而非触发工作台停止。
    // 用 [style*="display: none"] 排除已隐藏的（el-popper 关闭后 DOM 仍保留但 display:none）。
    if (e.code === 'Escape') {
      const hasNonModalFloating = document.querySelector(
        '.el-select-dropdown:not([style*="display: none"]), .el-popper:not([style*="display: none"])'
      )
      if (hasNonModalFloating) return

      e.preventDefault()
      handlers.onEscape()
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', onKeydown)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', onKeydown)
  })
}
