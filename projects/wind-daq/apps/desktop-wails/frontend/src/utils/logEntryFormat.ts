// 日志条目格式化工具：时间、级别徽章、分类标签、tooltip 拼接。
//
// 从 LogViewer.vue 和 LogEntryRow.vue 抽取共享逻辑，避免重复定义。
// 复用 logStore 中的 CATEGORY_LABELS / LOG_GROUP_LABELS / mapCategoryToGroup，
// 确保前端日志显示口径一致。
import { CATEGORY_LABELS, LOG_GROUP_LABELS, mapCategoryToGroup } from '@stores/logStore'
import type { LogEntry, LogLevel } from '@api/types'

// formatTime 将 ISO 时间戳格式化为 HH:MM:SS.mmm 等宽显示，便于对齐排查时序问题。
// 无效日期返回 '--:--:--.---' 占位。
export function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '--:--:--.---'
  return (
    d.toLocaleTimeString('zh-CN', { hour12: false }) +
    '.' +
    String(d.getMilliseconds()).padStart(3, '0')
  )
}

// levelBadge 返回级别单字母徽章（D/I/W/E），节省横向空间。
export function levelBadge(level: LogLevel): string {
  switch (level) {
    case 'debug':
      return 'D'
    case 'info':
      return 'I'
    case 'warn':
      return 'W'
    case 'error':
      return 'E'
    default:
      return '?'
  }
}

// categoryLabel 返回日志条目的分类标签：
// 显式 category 优先取 CATEGORY_LABELS（未命中时回退原值），否则按 group 取 LOG_GROUP_LABELS。
export function categoryLabel(entry: LogEntry): string {
  if (entry.category) return CATEGORY_LABELS[entry.category] ?? entry.category
  return LOG_GROUP_LABELS[mapCategoryToGroup(entry.category)]
}

// buildEntryTooltip 拼接日志条目各部分为单行字符串，用于 title 提示。
// 单行显示后超出部分被 ellipsis 截断，hover 时需显示完整内容便于排查。
export function buildEntryTooltip(entry: LogEntry): string {
  const parts: string[] = [entry.message]
  if (entry.source) parts.push(`· ${entry.source}`)
  if (entry.deviceId) parts.push(`· dev=${entry.deviceId}`)
  if (entry.details) parts.push(`· ${entry.details}`)
  return parts.join(' ')
}
