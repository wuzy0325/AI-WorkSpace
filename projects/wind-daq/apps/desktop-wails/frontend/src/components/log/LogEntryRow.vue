<script setup lang="ts">
// 日志条目单行渲染组件。
// 从 LogViewer.vue 抽取以满足 validate-frontend-structure 的文件行数约束，
// 同时让日志条目的时间/级别/分类/消息渲染逻辑自包含，便于后续复用。
// 格式化函数（formatTime / levelBadge / categoryLabel / buildEntryTooltip）
// 共享自 utils/logEntryFormat.ts，与 LogViewer.vue 复用同一份实现。
import { mapCategoryToGroup } from '@stores/logStore'
import type { LogEntry } from '@api/types'
import { formatTime, levelBadge, categoryLabel, buildEntryTooltip } from '@utils/logEntryFormat'

defineProps<{
  entry: LogEntry
}>()
</script>

<template>
  <article
    class="log-entry"
    :class="`log-entry--${entry.level}`"
  >
    <time class="log-time" :datetime="entry.timestamp">{{ formatTime(entry.timestamp) }}</time>
    <span class="log-badge" :class="`log-badge--${entry.level}`" :title="entry.level">{{ levelBadge(entry.level) }}</span>
    <span class="log-category" :class="`log-category--${mapCategoryToGroup(entry.category)}`">{{ categoryLabel(entry) }}</span>
    <span class="log-message" :title="buildEntryTooltip(entry)">
      <span class="log-message__text">{{ entry.message }}</span>
      <span v-if="entry.source" class="log-source" :title="entry.source">· {{ entry.source }}</span>
      <span v-if="entry.deviceId" class="log-device">· dev={{ entry.deviceId }}</span>
      <code v-if="entry.details" class="log-details">· {{ entry.details }}</code>
    </span>
  </article>
</template>

<style scoped>
/* 4 列网格布局，与父组件 .log-table__head 对齐：
 * 时间 7.5rem / 级别 1.5rem / 分类 3rem / 消息 1fr。
 * 子组件自包含 grid-template-columns，避免依赖父级样式。*/
.log-entry {
  display: grid;
  grid-template-columns: 7.5rem 1.5rem 3rem minmax(0, 1fr);
  gap: var(--space-1-5);
  align-items: baseline;
  padding: 0.1rem var(--space-2);
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
}

.log-entry:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 65%, transparent);
}

.log-entry--error {
  background: color-mix(in srgb, var(--accent-danger) 7%, transparent);
}

.log-entry--warn {
  background: color-mix(in srgb, var(--accent-warning) 5%, transparent);
}

.log-time {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.log-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.1rem;
  height: 1.1rem;
  border-radius: var(--radius-sm);
  text-align: center;
  font-size: 0.625rem;
  font-weight: 800;
  letter-spacing: 0;
}

.log-badge--debug {
  background: color-mix(in srgb, var(--text-muted) 12%, transparent);
  color: var(--text-secondary);
}

.log-badge--info {
  background: color-mix(in srgb, var(--accent-info) 14%, transparent);
  color: var(--accent-info);
}

.log-badge--warn {
  background: color-mix(in srgb, var(--accent-warning) 15%, transparent);
  color: var(--accent-warning);
}

.log-badge--error {
  background: color-mix(in srgb, var(--accent-danger) 15%, transparent);
  color: var(--accent-danger);
}

.log-category {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  font-size: 0.625rem;
}

.log-category--communication {
  color: var(--accent-info);
}

.log-category--acquisition {
  color: var(--accent-success);
}

.log-category--business {
  color: var(--accent-warning);
}

.log-message {
  min-width: 0;
  color: var(--text-primary);
  user-select: text;
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
  /* 单行显示：消息 + source + device + details 全部内联到同一行。
   * text-overflow: ellipsis 在 flex 容器上不生效，由内部可收缩的子元素
   * (.log-message__text / .log-details) 各自承担截断。*/
  flex-wrap: nowrap;
  overflow: hidden;
  white-space: nowrap;
}

.log-message__text {
  /* 主消息文本：优先收缩目标，但保留 min-width 避免被挤到 0 宽度消失 */
  flex: 1 1 auto;
  min-width: 6rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-source {
  color: var(--text-muted);
  font-size: 0.625rem;
  font-weight: 500;
  white-space: nowrap;
  flex-shrink: 0;
}

.log-device {
  color: var(--text-muted);
  font-size: 0.625rem;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}

.log-details {
  /* 内联显示，跟随 message 行；作为次要收缩目标，超出时 ellipsis 截断。
   * 等宽字体区分命令帧详情与普通文本，便于排查设备协议问题。*/
  display: inline-block;
  max-width: 40%;
  color: var(--text-muted);
  font-size: 0.625rem;
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 1;
}
</style>
