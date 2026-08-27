<template>
  <div class="comm-log-panel">
    <div class="log-toolbar">
      <div
        v-if="!embedded"
        class="toolbar-left"
      >
        <span class="toolbar-title">系统日志</span>
        <el-tag
          size="small"
          type="info"
          class="count-tag"
        >
          {{ filtered.length }}
        </el-tag>
      </div>
      <div class="toolbar-right">
        <el-input
          v-model="searchKeyword"
          size="small"
          placeholder="关键字过滤"
          clearable
          style="width: 140px"
        />
        <el-select
          v-model="filterKind"
          size="small"
          style="width: 110px"
          placeholder="全部类型"
          clearable
        >
          <el-option
            label="发送命令"
            value="hw-cmd"
          />
          <el-option
            label="设备响应"
            value="hw-res"
          />
          <el-option
            label="系统错误"
            value="sys-error"
          />
        </el-select>
        <el-button
          size="small"
          @click="store.clear()"
        >
          清空
        </el-button>
        <el-button
          size="small"
          @click="toggleAutoScroll"
        >
          {{ autoScroll ? '停止滚动' : '自动滚动' }}
        </el-button>
        <el-switch
          v-model="hidePoll"
          size="small"
          active-text="隐藏轮询"
          inactive-text="显示全部"
          inline-prompt
        />
      </div>
    </div>
    <div
      ref="logListEl"
      class="log-list"
      @scroll="onScroll"
    >
      <div
        v-for="entry in filtered"
        :key="entry.id"
        class="log-row"
        :class="entry.kind"
      >
        <span class="log-time">{{ formatTime(entry.timestamp) }}</span>
        <span class="log-kind">{{ kindLabel(entry.kind) }}</span>
        <span class="log-model">{{ entry.model }}</span>
        <span class="log-proto">{{ entry.proto }}</span>
        <span
          class="log-detail"
          :title="entry.detail"
        >{{ entry.detail }}</span>
      </div>
      <div
        v-if="filtered.length === 0"
        class="log-empty"
      >
        暂无系统日志
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useHardwareLogStore, type HwLogKind } from '@/stores/hardwareLog'

defineProps<{ embedded?: boolean }>()

const store = useHardwareLogStore()
const filterKind = ref<HwLogKind | ''>('')
const searchKeyword = ref('')
const autoScroll = ref(true)
const hidePoll = ref(false)
const logListEl = ref<HTMLElement | null>(null)

const filtered = computed(() => {
  let result = store.entries

  if (hidePoll.value) {
    result = result.filter(e => !e.poll)
  }

  if (filterKind.value) {
    result = result.filter(e => e.kind === filterKind.value)
  }

  if (searchKeyword.value.trim()) {
    const kw = searchKeyword.value.trim().toLowerCase()
    result = result.filter(e =>
      e.model.toLowerCase().includes(kw) ||
      e.proto.toLowerCase().includes(kw) ||
      e.detail.toLowerCase().includes(kw)
    )
  }

  return result
})

function kindLabel(kind: HwLogKind): string {
  switch (kind) {
    case 'hw-cmd': return 'CMD'
    case 'hw-res': return 'RES'
    case 'sys-error': return 'ERR'
  }
}

function formatTime(ts: number): string {
  const d = new Date(ts)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${hh}:${mm}:${ss}.${ms}`
}

function onScroll() {
  const el = logListEl.value
  if (!el) return
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30
  autoScroll.value = atBottom
}

function toggleAutoScroll() {
  autoScroll.value = !autoScroll.value
  if (autoScroll.value) {
    nextTick(() => {
      const el = logListEl.value
      if (el) el.scrollTop = el.scrollHeight
    })
  }
}

watch(
  () => filtered.value.length,
  () => {
    if (!autoScroll.value) return
    nextTick(() => {
      const el = logListEl.value
      if (el) el.scrollTop = el.scrollHeight
    })
  }
)
</script>

<style scoped lang="scss">
.comm-log-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  font-family: 'Consolas', 'Menlo', 'Courier New', monospace;
  font-size: 12px;
  background: #1e1e1e;
  color: #d4d4d4;
  border-top: 2px solid #10b981;
  border-radius: 8px;
  overflow: hidden;
}

.log-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 10px;
  background: #2d2d2d;
  border-bottom: 1px solid #3c3c3c;
  flex-shrink: 0;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.toolbar-title {
  font-weight: 600;
  font-size: 13px;
  color: #e0e0e0;
}

.count-tag {
  font-size: 11px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.log-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 2px 0;
}

.log-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 1px 10px;
  white-space: nowrap;
  min-height: 18px;
  line-height: 18px;

  &:hover { background: rgba(255, 255, 255, 0.04); }

  &.hw-cmd .log-kind { color: #569cd6; }
  &.hw-res .log-kind { color: #4ec9b0; }
  &.sys-error {
    background: rgba(248, 81, 73, 0.08);

    .log-kind { color: #f85149; }
    .log-model { color: #ffa198; }
    .log-proto { color: #ffa198; }
    .log-detail { color: #ff7b72; }
  }
}

.log-time {
  color: #6a9955;
  flex-shrink: 0;
}

.log-kind {
  font-weight: 700;
  flex-shrink: 0;
  width: 32px;
  text-align: center;
}

.log-model {
  color: #9cdcfe;
  flex-shrink: 0;
  width: 80px;
}

.log-proto {
  color: #ce9178;
  flex-shrink: 0;
  width: 60px;
}

.log-detail {
  color: #b0b0b0;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}

.log-empty {
  padding: 20px;
  text-align: center;
  color: #666;
}
</style>
