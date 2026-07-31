import type { TraversalSessionState } from '@shared/types/traversal'
import { safeInterpolate } from '@shared/i18nInterpolate'

type TranslationMap = Record<string, string>

/**
 * I-16 修复：后端在检测到 probe 仍有可恢复任务时返回的错误码字符串。
 * 抽取为共享常量，避免 dualTraversalStore.start 与 mapTraversalError 各自
 * 字符串匹配 'recoverable_task_exists'，降低后端改码时的散落修改风险。
 *
 * 注：后端目前以 `error: "recoverable_task_exists: ..."` 字符串形式返回，
 * 前端只能 includes 匹配；后续若后端改为结构化 code 字段，只需改本常量判定逻辑。
 */
export const RECOVERABLE_TASK_EXISTS_CODE = 'recoverable_task_exists'

export function isRecoverableTaskExistsError(message: string | undefined): boolean {
  return !!message && message.includes(RECOVERABLE_TASK_EXISTS_CODE)
}

export function mapTraversalError(message: string, translations: TranslationMap): string {
  const notAcquiring = message.match(/^device (.+?) is not acquiring(?:;.*)?$/)
  if (notAcquiring) {
    // C8 修复：notAcquiring[1] 来自后端错误消息（不可信输入），原 .replace('{device}', value)
    // 会让 value 中的 $ 字符被特殊解析（如 "1/$&" 会展开为 "1/<匹配占位符>"）。
    // 改用 safeInterpolate 函数式 replace，将 value 作为字面量插入。
    return safeInterpolate(
      translations['traversal.dual.device-not-acquiring'],
      '{device}',
      notAcquiring[1],
    )
  }
  if (isRecoverableTaskExistsError(message)) {
    return translations['traversal.dual.recoverable-task-exists']
  }
  return message
}

export function traversalSessionWarning(session: TraversalSessionState, translations: TranslationMap): string {
  const messages = [session.status?.warning, session.status?.lastError, session.error]
    .filter((message): message is string => !!message)
    .map((message) => mapTraversalError(message, translations))
  const message = Array.from(new Set(messages)).join(translations['traversal.dual.error-separator'])
  if (!session.checkpoint || !message) return message
  // C8 修复：message 可能含后端错误（不可信），改用 safeInterpolate 防止 $ 注入。
  return safeInterpolate(
    translations['traversal.dual.previous-task-error-format'],
    '{error}',
    message,
  )
}
