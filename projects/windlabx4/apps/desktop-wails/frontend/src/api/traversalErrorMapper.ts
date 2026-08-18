import type { TraversalSessionState } from '@shared/types/traversal'
import { safeInterpolate } from '@shared/i18nInterpolate'

type TranslationMap = Record<string, string>

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
  return message
}

export function traversalSessionWarning(session: TraversalSessionState, translations: TranslationMap): string {
  const messages = [session.status?.warning, session.status?.lastError, session.error]
    .filter((message): message is string => !!message)
    .map((message) => mapTraversalError(message, translations))
  const message = Array.from(new Set(messages)).join(translations['traversal.dual.error-separator'])
  return message
}
