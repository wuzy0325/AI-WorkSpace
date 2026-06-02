<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { filterHzToLabel, filterLabelToHz, FILTER_HZ_OPTIONS } from '@bridge/deviceBridge'
import {
  Settings2, Thermometer, Snowflake, Activity, Gauge,
  Save, RotateCcw, ChevronDown, CheckCircle2, AlertCircle,
  SlidersHorizontal, Zap
} from '@lucide/vue'

const props = defineProps<{ deviceId: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const deviceStore = useDeviceStore()
const profile = computed(() => deviceStore.profiles.find((p) => p.id === props.deviceId))

const thermocoupleOptions = ['K', 'J', 'T', 'E', 'N', 'S', 'R', 'B']
const filterLabels = FILTER_HZ_OPTIONS.map(filterHzToLabel)

const tcType = ref('K')
const cjcEnabled = ref(true)
const filterFreq = ref('50Hz')
const channelNames = ref<string[]>(Array(16).fill(''))
const channelEnabled = ref<boolean[]>(Array(16).fill(true))
const channelColors = ref<string[]>(Array(16).fill(''))

const COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#ec4899', '#14b8a6', '#84cc16', '#8b5cf6',
  '#ef4444', '#0ea5e9', '#eab308', '#64748b',
]

const hasChanges = ref(false)
const saveStatus = ref<'idle' | 'saving' | 'success' | 'error'>('idle')
const saveMessage = ref('')

watch(
  () => profile.value,
  (p) => {
    if (!p) return
    tcType.value = p.t1603Config?.thermocoupleType || 'K'
    cjcEnabled.value = p.t1603Config?.coldJunction === 'internal'
    filterFreq.value = filterHzToLabel(p.t1603Config?.filterHz ?? 50)
    channelNames.value = p.channels.map((c) => c.name || '')
    channelEnabled.value = p.channels.map((c) => c.enabled)
    channelColors.value = p.channels.map((c) => c.color || '')
    hasChanges.value = false
    saveStatus.value = 'idle'
  },
  { immediate: true }
)

watch([tcType, cjcEnabled, filterFreq, channelNames, channelEnabled, channelColors], () => {
  hasChanges.value = true
  saveStatus.value = 'idle'
}, { deep: true })

const enabledCount = computed(() => channelEnabled.value.filter(Boolean).length)

async function saveConfig() {
  if (!profile.value) return
  saveStatus.value = 'saving'
  saveMessage.value = ''
  try {
    await deviceStore.updateT1603Config(props.deviceId, {
      thermocoupleType: tcType.value,
      coldJunction: cjcEnabled.value ? 'internal' : 'disabled',
      filterHz: filterLabelToHz(filterFreq.value),
    })
    for (let i = 0; i < 16; i++) {
      await deviceStore.updateChannel(props.deviceId, i, {
        name: channelNames.value[i] || undefined,
        enabled: channelEnabled.value[i],
        color: channelColors.value[i] || undefined,
      })
    }
    saveStatus.value = 'success'
    saveMessage.value = '配置已保存'
    hasChanges.value = false
    setTimeout(() => { saveStatus.value = 'idle' }, 2000)
  } catch (err) {
    saveStatus.value = 'error'
    saveMessage.value = err instanceof Error ? err.message : '保存失败'
  }
}

function resetConfig() {
  const p = profile.value
  if (!p) return
  tcType.value = p.t1603Config?.thermocoupleType || 'K'
  cjcEnabled.value = p.t1603Config?.coldJunction === 'internal'
  filterFreq.value = filterHzToLabel(p.t1603Config?.filterHz ?? 50)
  channelNames.value = p.channels.map((c) => c.name || '')
  channelEnabled.value = p.channels.map((c) => c.enabled)
  channelColors.value = p.channels.map((c) => c.color || '')
  hasChanges.value = false
  saveStatus.value = 'idle'
}

function toggleChannel(index: number) {
  channelEnabled.value[index] = !channelEnabled.value[index]
}

function selectColor(index: number, color: string) {
  channelColors.value[index] = color
}
</script>

<template>
  <div v-if="profile" class="config">
    <!-- 头部 -->
    <div class="config__header">
      <div class="config__header-left">
        <Settings2 class="config__header-icon" />
        <div class="config__header-text">
          <h3 class="config__header-title">{{ profile.name || '设备配置' }}</h3>
          <p class="config__header-subtitle">
            {{ profile.address }}:{{ profile.port }}
            <span v-if="hasChanges" class="config__header-unsaved">未保存</span>
          </p>
        </div>
      </div>
      <button class="config__close" title="关闭" @click="emit('close')">
        <span class="config__close-line"></span>
        <span class="config__close-line"></span>
      </button>
    </div>

    <!-- 内容区 -->
    <div class="config__body">
      <!-- 硬件参数 -->
      <section class="config__section">
        <div class="config__section-header">
          <SlidersHorizontal class="config__section-icon" />
          <h4 class="config__section-title">硬件参数</h4>
        </div>
        <div class="config__section-body">
          <div class="config__field">
            <label class="config__label">
              <Thermometer class="config__label-icon" />
              <span>热电偶类型</span>
            </label>
            <div class="config__select-wrap">
              <select v-model="tcType" class="config__select">
                <option v-for="t in thermocoupleOptions" :key="t" :value="t">{{ t }} 型</option>
              </select>
              <ChevronDown class="config__select-arrow" />
            </div>
          </div>

          <div class="config__field">
            <label class="config__label">
              <Snowflake class="config__label-icon" />
              <span>冷端补偿</span>
            </label>
            <button
              class="config__toggle"
              :class="{ 'config__toggle--on': cjcEnabled }"
              @click="cjcEnabled = !cjcEnabled"
            >
              <span class="config__toggle-track">
                <span class="config__toggle-thumb"></span>
              </span>
              <span class="config__toggle-text">{{ cjcEnabled ? '启用' : '禁用' }}</span>
            </button>
          </div>

          <div class="config__field">
            <label class="config__label">
              <Gauge class="config__label-icon" />
              <span>滤波频率</span>
            </label>
            <div class="config__select-wrap">
              <select v-model="filterFreq" class="config__select">
                <option v-for="f in filterLabels" :key="f" :value="f">{{ f }}</option>
              </select>
              <ChevronDown class="config__select-arrow" />
            </div>
          </div>
        </div>
      </section>

      <!-- 通道列表 -->
      <section class="config__section">
        <div class="config__section-header">
          <Activity class="config__section-icon" />
          <h4 class="config__section-title">通道配置</h4>
          <span class="config__section-badge">{{ enabledCount }}/16 启用</span>
        </div>
        <div class="config__channels">
          <div
            v-for="i in 16"
            :key="i"
            class="config__channel"
            :class="{ 'config__channel--disabled': !channelEnabled[i - 1] }"
          >
            <div class="config__channel-row">
              <div class="config__channel-info">
                <span class="config__channel-index mono">CH{{ String(i).padStart(2, '0') }}</span>
                <input
                  v-model="channelNames[i - 1]"
                  class="config__channel-name"
                  :placeholder="`通道 ${i}`"
                  :disabled="!channelEnabled[i - 1]"
                />
              </div>
              <div class="config__channel-actions">
                <div class="config__channel-colors">
                  <button
                    v-for="color in COLORS"
                    :key="color"
                    class="config__color-dot"
                    :class="{ 'config__color-dot--active': channelColors[i - 1] === color }"
                    :style="{ background: color }"
                    :disabled="!channelEnabled[i - 1]"
                    @click="selectColor(i - 1, color)"
                  />
                </div>
                <button
                  class="config__channel-toggle"
                  :class="{ 'config__channel-toggle--on': channelEnabled[i - 1] }"
                  @click="toggleChannel(i - 1)"
                >
                  <Zap class="config__channel-toggle-icon" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>

    <!-- 底部操作栏 -->
    <div class="config__footer">
      <div v-if="saveStatus !== 'idle'" class="config__status" :class="`config__status--${saveStatus}`">
        <CheckCircle2 v-if="saveStatus === 'success'" class="config__status-icon" />
        <AlertCircle v-else-if="saveStatus === 'error'" class="config__status-icon" />
        <RotateCcw v-else class="config__status-icon config__status-icon--spin" />
        <span>{{ saveMessage || (saveStatus === 'saving' ? '保存中...' : '') }}</span>
      </div>
      <div v-else class="config__status config__status--idle">
        <span v-if="hasChanges">有未保存的更改</span>
        <span v-else>所有更改已保存</span>
      </div>
      <div class="config__actions">
        <button class="config__btn config__btn--secondary" @click="resetConfig">
          <RotateCcw class="config__btn-icon" />
          <span>重置</span>
        </button>
        <button
          class="config__btn config__btn--primary"
          :disabled="saveStatus === 'saving'"
          @click="saveConfig"
        >
          <Save class="config__btn-icon" />
          <span>保存</span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.config {
  display: flex;
  flex-direction: column;
  max-height: inherit;
  background: var(--bg-panel);
  border-radius: var(--radius-xl);
  overflow: hidden;
}

/* 头部 */
.config__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--divider-color);
  flex-shrink: 0;
}

.config__header-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
}

.config__header-icon {
  width: 20px;
  height: 20px;
  color: var(--accent);
  flex-shrink: 0;
}

.config__header-text {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.config__header-title {
  font-size: var(--font-size-md);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.01em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.config__header-subtitle {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.config__header-unsaved {
  padding: 0.1rem 0.4rem;
  background: var(--warning-muted);
  color: var(--warning);
  border-radius: var(--radius-sm);
  font-size: 0.6rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.config__close {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  position: relative;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.config__close:hover {
  background: var(--danger-muted);
  border-color: var(--danger-border);
}

.config__close:hover .config__close-line {
  background: var(--danger);
}

.config__close-line {
  position: absolute;
  width: 12px;
  height: 1.5px;
  background: var(--text-muted);
  border-radius: 1px;
  transition: background var(--motion-fast) var(--easing-standard);
}

.config__close-line:first-child {
  transform: rotate(45deg);
}

.config__close-line:last-child {
  transform: rotate(-45deg);
}

/* 内容区 */
.config__body {
  flex: 1;
  overflow-y: auto;
  padding: 1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

/* 分区 */
.config__section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.config__section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--divider-color);
}

.config__section-icon {
  width: 14px;
  height: 14px;
  color: var(--accent);
}

.config__section-title {
  font-size: var(--font-size-sm);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: 0.02em;
  flex: 1;
}

.config__section-badge {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-weight: 600;
  padding: 0.2rem 0.5rem;
  background: var(--btn-bg);
  border-radius: var(--radius-sm);
}

.config__section-body {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}

/* 表单字段 */
.config__field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.5rem 0.75rem;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  transition: border-color var(--motion-fast) var(--easing-standard);
}

.config__field:hover {
  border-color: var(--border-hover);
}

.config__label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-secondary);
}

.config__label-icon {
  width: 14px;
  height: 14px;
  color: var(--accent);
  opacity: 0.8;
}

/* 下拉选择 */
.config__select-wrap {
  position: relative;
  min-width: 7rem;
}

.config__select {
  width: 100%;
  padding: 0.45rem 1.75rem 0.45rem 0.75rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  background: var(--bg-input);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  cursor: pointer;
  appearance: none;
  transition: border-color var(--motion-fast) var(--easing-standard),
              box-shadow var(--motion-fast) var(--easing-standard);
}

.config__select:hover {
  border-color: var(--border-hover);
}

.config__select:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.config__select-arrow {
  position: absolute;
  right: 0.5rem;
  top: 50%;
  transform: translateY(-50%);
  width: 14px;
  height: 14px;
  color: var(--text-muted);
  pointer-events: none;
}

/* 开关 */
.config__toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.25rem;
}

.config__toggle-track {
  width: 36px;
  height: 20px;
  border-radius: 10px;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  position: relative;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config__toggle--on .config__toggle-track {
  background: var(--accent);
  border-color: var(--accent);
}

.config__toggle-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--text-primary);
  transition: transform var(--motion-fast) var(--easing-standard);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.config__toggle--on .config__toggle-thumb {
  transform: translateX(16px);
  background: #ffffff;
}

.config__toggle-text {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  min-width: 2.5rem;
  text-align: left;
  transition: color var(--motion-fast) var(--easing-standard);
}

.config__toggle--on .config__toggle-text {
  color: var(--accent);
}

/* 通道列表 */
.config__channels {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.config__channel {
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.5rem 0.75rem;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config__channel:hover {
  border-color: var(--border-hover);
}

.config__channel--disabled {
  opacity: 0.55;
}

.config__channel-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.config__channel-info {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex: 1;
  min-width: 0;
}

.config__channel-index {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  background: var(--btn-bg);
  padding: 0.15rem 0.4rem;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

.config__channel-name {
  flex: 1;
  min-width: 0;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  padding: 0.3rem 0.5rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-primary);
  transition: all var(--motion-fast) var(--easing-standard);
}

.config__channel-name:hover {
  background: var(--bg-input);
  border-color: var(--border-default);
}

.config__channel-name:focus {
  outline: none;
  background: var(--bg-input);
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.config__channel-name::placeholder {
  color: var(--text-muted);
  font-weight: 500;
}

.config__channel-name:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.config__channel-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

.config__channel-colors {
  display: flex;
  gap: 0.25rem;
}

.config__color-dot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  position: relative;
}

.config__color-dot:hover {
  transform: scale(1.2);
  box-shadow: 0 0 8px currentColor;
}

.config__color-dot--active {
  border-color: var(--text-primary);
  box-shadow: 0 0 0 2px var(--bg-panel), 0 0 0 3px var(--text-primary);
}

.config__color-dot:disabled {
  opacity: 0.3;
  cursor: not-allowed;
  transform: none;
  box-shadow: none;
}

.config__channel-toggle {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  transition: all var(--motion-fast) var(--easing-standard);
}

.config__channel-toggle:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
}

.config__channel-toggle--on {
  background: var(--accent);
  border-color: var(--accent);
  color: #ffffff;
}

.config__channel-toggle--on:hover {
  background: var(--accent-hover);
  border-color: var(--accent-hover);
}

.config__channel-toggle-icon {
  width: 12px;
  height: 12px;
}

/* 底部 */
.config__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.85rem 1.25rem;
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--divider-color);
  flex-shrink: 0;
  gap: 1rem;
}

.config__status {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.config__status--idle {
  color: var(--text-muted);
}

.config__status--success {
  color: var(--accent);
}

.config__status--error {
  color: var(--danger);
}

.config__status--saving {
  color: var(--text-secondary);
}

.config__status-icon {
  width: 14px;
  height: 14px;
}

.config__status-icon--spin {
  animation: spin 1s linear infinite;
}

.config__actions {
  display: flex;
  gap: 0.5rem;
}

.config__btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 1rem;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 700;
  transition: all var(--motion-fast) var(--easing-standard);
}

.config__btn--secondary {
  background: var(--btn-bg);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
}

.config__btn--secondary:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
  border-color: var(--border-hover);
}

.config__btn--primary {
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  color: #ffffff;
  border: 1px solid var(--accent-border);
  box-shadow: 0 4px 12px var(--accent-glow);
}

.config__btn--primary:hover {
  box-shadow: 0 6px 18px var(--accent-glow);
}

.config__btn--primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  box-shadow: none;
}

.config__btn-icon {
  width: 14px;
  height: 14px;
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .config__toggle-track,
  .config__toggle-thumb,
  .config__color-dot,
  .config__close-line,
  .config__field,
  .config__channel,
  .config__channel-name,
  .config__btn {
    transition: none;
  }

  .config__color-dot:hover {
    transform: none;
  }

  .config__status-icon--spin {
    animation: none;
  }
}
</style>
