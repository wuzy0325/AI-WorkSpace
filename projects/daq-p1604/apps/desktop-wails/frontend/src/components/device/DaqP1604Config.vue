<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import CustomSelect from './CustomSelect.vue'
import type { SelectOption } from './CustomSelect.vue'
import {
  Settings2, Activity,
  Save, RotateCcw, CheckCircle2, AlertCircle,
  SlidersHorizontal, Hash, Clock, Wifi, Gauge, Crosshair,
} from '@lucide/vue'

const props = defineProps<{ deviceId: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const deviceStore = useDeviceStore()
const profile = computed(() => deviceStore.profiles.find((p) => p.id === props.deviceId))
const isAcquiring = computed(() => deviceStore.acquiringFor(props.deviceId))

// 压力单位选项
const pressureUnitOptions: SelectOption[] = [
  { value: 'psi', label: 'psi' },
  { value: 'Pa', label: 'Pa' },
  { value: 'kPa', label: 'kPa' },
  { value: 'MPa', label: 'MPa' },
  { value: 'kgf/cm²', label: 'kgf/cm²' },
]

// 精度选项（0-6 位小数）
const precisionOptions: SelectOption[] = Array.from({ length: 7 }, (_, i) => ({
  value: String(i),
  label: `${i} 位小数`,
}))

// 采样频率（Hz），UI 层展示频率，保存时换算为周期毫秒
// 频率范围：1000/60000 ≈ 0.017 Hz 到 1000/10 = 100 Hz
const samplingFreq = ref(10)
const autoConnect = ref(false) // 启动时自动连接
const pressureUnit = ref('psi') // 全局压力单位
const globalPrecision = ref(3) // 全局默认精度（小数位数）
const channelNames = ref<string[]>(Array(18).fill(''))
const channelEnabled = ref<boolean[]>(Array(18).fill(true))
const channelColors = ref<string[]>(Array(18).fill(''))
const channelPrecisions = ref<number[]>(Array(18).fill(3)) // 每通道独立精度

// 默认颜色，用于未设置颜色时的回退值
const DEFAULT_COLOR = '#3b82f6'

const hasChanges = ref(false)
const saveStatus = ref<'idle' | 'saving' | 'success' | 'error'>('idle')
const saveMessage = ref('')
/** 保存同步标志：为 true 时禁止 watcher 更新表单状态，防止覆盖保存结果 */
const syncing = ref(false)

/** 判断通道是否为特殊通道（CH17 大气压力、CH18 大气温度） */
function isSpecialChannel(index: number): boolean {
  return index === 16 || index === 17
}

/** 获取通道默认名称 */
function getDefaultChannelName(index: number): string {
  if (index === 16) return '大气压力'
  if (index === 17) return '大气温度'
  return `通道 ${index + 1}`
}

/** 采样周期（毫秒）转频率（Hz），保留 2 位小数 */
function periodMsToHz(ms: number): number {
  if (!ms || ms < 10) return 100
  const hz = 1000 / ms
  return Math.round(hz * 100) / 100
}

/** 频率（Hz）转采样周期（毫秒），限制 10-60000 */
function hzToPeriodMs(hz: number): number {
  if (!hz || hz <= 0) return 100
  const ms = Math.round(1000 / hz)
  return Math.max(10, Math.min(60000, ms))
}

function syncFormFromProfile(profileData: typeof profile.value) {
  if (!profileData) return
  // 保存同步中跳过表单同步，避免触发 watcher 覆盖 saveStatus
  if (syncing.value) return
  samplingFreq.value = periodMsToHz(profileData.p1604Config?.samplingRate || 100)
  autoConnect.value = profileData.p1604Config?.autoConnect ?? false
  pressureUnit.value = profileData.p1604Config?.unit || 'psi'
  globalPrecision.value = profileData.p1604Config?.precision ?? 3
  channelNames.value = profileData.channels.map((c) => c.name || '')
  channelEnabled.value = profileData.channels.map((c) => c.enabled)
  channelColors.value = profileData.channels.map((c) => c.color || '')
  channelPrecisions.value = profileData.channels.map((c) => c.precision ?? globalPrecision.value)
  hasChanges.value = false
  saveStatus.value = 'idle'
}

watch(
  () => profile.value,
  syncFormFromProfile,
  { immediate: true }
)

watch([samplingFreq, autoConnect, pressureUnit, globalPrecision, channelNames, channelEnabled, channelColors, channelPrecisions], () => {
  // 保存同步中跳过，避免覆盖 saveStatus
  if (syncing.value) return
  hasChanges.value = true
  saveStatus.value = 'idle'
}, { deep: true })

const enabledCount = computed(() => channelEnabled.value.filter(Boolean).length)

/** 判断硬件配置是否发生变更（排除纯UI设置如界面刷新率） */
function hasHardwareConfigChanged(current: typeof profile.value, next: typeof current): boolean {
  if (!current || !next) return true
  const cur = current.p1604Config
  const nxt = next.p1604Config
  if (cur.samplingRate !== nxt.samplingRate) return true
  if (cur.autoConnect !== nxt.autoConnect) return true
  if (cur.unit !== nxt.unit) return true
  if (cur.precision !== nxt.precision) return true
  // 通道配置变更
  for (let i = 0; i < current.channels.length; i++) {
    const cc = current.channels[i]
    const nc = next.channels[i]
    if (!cc || !nc) return true
    if (cc.enabled !== nc.enabled) return true
    if (cc.precision !== nc.precision) return true
  }
  return false
}

async function saveConfig() {
  if (!profile.value) return
  saveStatus.value = 'saving'
  saveMessage.value = ''
  syncing.value = true
  try {
    const nextProfile = {
      ...profile.value,
      p1604Config: {
        ...profile.value.p1604Config,
        samplingRate: hzToPeriodMs(samplingFreq.value),
        autoConnect: autoConnect.value,
        unit: pressureUnit.value,
        precision: globalPrecision.value,
      },
      channels: profile.value.channels.map((channel, index) => ({
        ...channel,
        name: channelNames.value[index] || '',
        enabled: channelEnabled.value[index],
        color: channelColors.value[index] || '',
        precision: channelPrecisions.value[index] ?? globalPrecision.value,
      })),
    }
    await deviceStore.saveProfile(nextProfile)

    const hwChanged = hasHardwareConfigChanged(profile.value, nextProfile)
    if (hwChanged) {
      try {
        await deviceStore.applyConfig(props.deviceId, nextProfile.p1604Config)
        saveMessage.value = '配置已保存并应用到设备'
      } catch (hwErr) {
        saveMessage.value = hwErr instanceof Error ? hwErr.message : '硬件配置应用失败'
      }
    } else {
      saveMessage.value = '配置已保存'
    }

    saveStatus.value = 'success'
    hasChanges.value = false
    setTimeout(() => { saveStatus.value = 'idle' }, 2000)
  } catch (err) {
    saveStatus.value = 'error'
    saveMessage.value = err instanceof Error ? err.message : '保存失败'
  } finally {
    syncing.value = false
  }
}

function resetConfig() {
  if (isAcquiring.value) return
  syncFormFromProfile(profile.value)
}

function toggleChannel(index: number) {
  if (isAcquiring.value) return
  channelEnabled.value[index] = !channelEnabled.value[index]
}

/** 全局精度变更时同步应用到所有通道（UI 层面，保存时持久化） */
function applyGlobalPrecisionToAll() {
  for (let i = 0; i < 18; i++) {
    channelPrecisions.value[i] = globalPrecision.value
  }
}

/** 采样频率输入处理，限制 0.02-100 Hz */
function onFreqInput(e: Event) {
  const target = e.target as HTMLInputElement
  let v = parseFloat(target.value)
  if (isNaN(v)) v = 10
  v = Math.max(0.02, Math.min(100, v))
  samplingFreq.value = v
}

/** 单通道精度变更 */
function onChannelPrecisionChange(index: number, value: string) {
  channelPrecisions.value[index] = parseInt(value, 10) || 0
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
      <button type="button" class="config__close" title="关闭" @click.stop="emit('close')">
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
          <!-- 采样频率（Hz） -->
          <div class="config__field">
            <label class="config__label">
              <Clock class="config__label-icon" />
              <span>采样频率</span>
            </label>
            <div class="config__rate-wrapper">
              <input
                v-model.number="samplingFreq"
                type="number"
                step="0.1"
                class="config__rate-input"
                :min="0.02"
                :max="100"
                :disabled="isAcquiring"
                @input="onFreqInput"
              />
              <span class="config__rate-unit">Hz</span>
            </div>
          </div>

          <!-- 压力单位选择 -->
          <div class="config__field">
            <label class="config__label">
              <Gauge class="config__label-icon" />
              <span>压力单位（全部通道）</span>
            </label>
            <CustomSelect
              :model-value="pressureUnit"
              :options="pressureUnitOptions"
              :disabled="isAcquiring"
              @update:model-value="pressureUnit = $event as string"
            />
          </div>

          <!-- 全局精度设置：应用到所有通道的默认显示精度 -->
          <div class="config__field">
            <label class="config__label">
              <Crosshair class="config__label-icon" />
              <span>全局精度（全部通道）</span>
            </label>
            <div class="config__precision-wrapper">
              <CustomSelect
                :model-value="String(globalPrecision)"
                :options="precisionOptions"
                :disabled="isAcquiring"
                @update:model-value="globalPrecision = parseInt($event as string, 10); applyGlobalPrecisionToAll()"
              />
              <button
                type="button"
                class="config__precision-apply"
                :disabled="isAcquiring"
                title="将全局精度应用到所有通道"
                @click="applyGlobalPrecisionToAll"
              >
                应用到全部
              </button>
            </div>
          </div>

          <!-- 自动连接开关：启动应用时自动连接此设备 -->
          <div class="config__field">
            <label class="config__label">
              <Wifi class="config__label-icon" />
              <span>自动连接</span>
            </label>
            <button
              type="button"
              class="config__toggle"
              :class="{ 'config__toggle--on': autoConnect }"
              :disabled="isAcquiring"
              @click="autoConnect = !autoConnect"
            >
              <span class="config__toggle-track">
                <span class="config__toggle-thumb"></span>
              </span>
              <span class="config__toggle-text">{{ autoConnect ? '开启' : '关闭' }}</span>
            </button>
          </div>
        </div>
        <p v-if="isAcquiring" class="config__section-hint">采集中不允许变更配置，请先停止采集。</p>
      </section>

      <!-- 通道列表 -->
      <section class="config__section">
        <div class="config__section-header">
          <Activity class="config__section-icon" />
          <h4 class="config__section-title">通道配置</h4>
          <span class="config__section-badge">{{ enabledCount }}/18 启用</span>
        </div>
        <div class="config__channels">
          <div
            v-for="i in 18"
            :key="i"
            class="config__channel"
            :class="{
              'config__channel--disabled': !channelEnabled[i - 1],
              'config__channel--special': isSpecialChannel(i - 1),
            }"
          >
            <div class="config__channel-row">
              <div class="config__channel-info">
                <span class="config__channel-index mono">CH{{ String(i).padStart(2, '0') }}</span>
                <!-- CH17/CH18 特殊标注 -->
                <span v-if="i === 17" class="config__channel-badge">大气压力</span>
                <span v-else-if="i === 18" class="config__channel-badge">大气温度</span>
                <input
                  v-model="channelNames[i - 1]"
                  class="config__channel-name"
                  :placeholder="getDefaultChannelName(i - 1)"
                  :disabled="isAcquiring || !channelEnabled[i - 1]"
                />
              </div>
              <div class="config__channel-actions">
                <!-- 单通道精度选择：覆盖全局精度 -->
                <div class="config__channel-precision">
                  <span class="config__channel-precision-label">精度</span>
                  <select
                    class="config__channel-precision-select"
                    :value="channelPrecisions[i - 1]"
                    :disabled="isAcquiring || !channelEnabled[i - 1]"
                    @change="onChannelPrecisionChange(i - 1, ($event.target as HTMLSelectElement).value)"
                  >
                    <option v-for="n in 7" :key="n - 1" :value="n - 1">{{ n - 1 }}</option>
                  </select>
                </div>
                <button
                  type="button"
                  class="config__toggle"
                  :class="{ 'config__toggle--on': channelEnabled[i - 1] }"
                  :title="channelEnabled[i - 1] ? '禁用通道' : '启用通道'"
                  :disabled="isAcquiring"
                  @click="toggleChannel(i - 1)"
                >
                  <span class="config__toggle-track">
                    <span class="config__toggle-thumb"></span>
                  </span>
                  <span class="config__toggle-text">{{ channelEnabled[i - 1] ? '开' : '关' }}</span>
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
        <span v-if="isAcquiring">采集中不允许变更配置</span>
        <span v-else-if="hasChanges">有未保存的更改</span>
        <span v-else>所有更改已保存</span>
      </div>
      <div class="config__actions">
        <button type="button" class="config__btn config__btn--secondary" :disabled="isAcquiring || saveStatus === 'saving'" @click.stop="resetConfig">
          <RotateCcw class="config__btn-icon" />
          <span>重置</span>
        </button>
        <button
          type="button"
          class="config__btn config__btn--primary"
          :disabled="saveStatus === 'saving'"
          @click.stop="saveConfig"
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

.config__section-hint {
  font-size: var(--font-size-xs);
  color: var(--warning);
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

/* 采样周期输入包装器 */
.config__rate-wrapper {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.config__rate-input {
  width: 6rem;
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-input, var(--bg-panel));
  color: var(--text-primary);
  font-size: var(--font-size-sm);
  font-weight: 700;
  text-align: right;
  outline: none;
  transition: border-color var(--motion-fast) var(--easing-standard);
}

.config__rate-input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.config__rate-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.config__rate-unit {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
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

.config__toggle:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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

/* 特殊通道（CH17/CH18）高亮边框 */
.config__channel--special {
  border-color: var(--accent-border);
  background: var(--accent-soft);
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

/* 特殊通道标注徽章 */
.config__channel-badge {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--accent);
  background: var(--accent-muted);
  padding: 0.1rem 0.4rem;
  border-radius: var(--radius-sm);
  white-space: nowrap;
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

/* 全局精度包装器 */
.config__precision-wrapper {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.config__precision-apply {
  padding: 0.3rem 0.6rem;
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--accent);
  background: var(--accent-muted);
  border: 1px solid var(--accent-border);
  border-radius: var(--radius-sm);
  transition: all var(--motion-fast) var(--easing-standard);
  white-space: nowrap;
}

.config__precision-apply:hover:not(:disabled) {
  background: var(--accent-soft);
}

.config__precision-apply:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 单通道精度选择 */
.config__channel-precision {
  display: flex;
  align-items: center;
  gap: 0.3rem;
}

.config__channel-precision-label {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
}

.config__channel-precision-select {
  width: 2.6rem;
  padding: 0.2rem 0.3rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-input, var(--bg-panel));
  color: var(--text-primary);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-align: center;
  cursor: pointer;
  transition: border-color var(--motion-fast) var(--easing-standard);
}

.config__channel-precision-select:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.config__channel-precision-select:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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

.config__btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
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
  .config__close-line,
  .config__field,
  .config__channel,
  .config__channel-name,
  .config__btn {
    transition: none;
  }

  .config__channel:hover {
    transform: none;
  }

  .config__status-icon--spin {
    animation: none;
  }
}
</style>
