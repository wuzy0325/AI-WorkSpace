// 模态对话框焦点陷阱
//
// 把 Tab 键限制在对话框内循环；用于满足 WCAG 键盘可访问性要求。
// 用法：
//   const dialogRef = ref<HTMLElement | null>(null)
//   const { trapFocus } = useFocusTrap(dialogRef)
//   <div ref="dialogRef" @keydown.tab="trapFocus" />
//
// 设计目的：把这段重复的 DOM 焦点循环逻辑从业务组件 (TraversalMain.vue) 中抽出，
// 以便其他对话框（如运动控制器配置确认框、校准启动确认框）复用。
import type { Ref } from 'vue'

/** 焦点陷阱内可获焦元素的 CSS 选择器 */
const FOCUSABLE_SELECTOR =
  'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'

/**
 * 焦点陷阱
 *
 * @param containerRef 对话框根元素的 ref；为 null 时 trapFocus 不做任何事，
 *                     允许对话框未挂载时安全调用。
 */
export function useFocusTrap(containerRef: Ref<HTMLElement | null>) {
  function trapFocus(event: KeyboardEvent): void {
    const container = containerRef.value
    if (!container) return

    const focusable = Array.from(
      container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
    )
    if (focusable.length === 0) return

    const first = focusable[0]
    const last = focusable[focusable.length - 1]

    if (event.shiftKey) {
      // Shift+Tab：在第一个元素时跳到最后一个
      if (document.activeElement === first) {
        event.preventDefault()
        last.focus()
      }
    } else {
      // Tab：在最后一个元素时跳到第一个
      if (document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
  }

  return { trapFocus }
}
