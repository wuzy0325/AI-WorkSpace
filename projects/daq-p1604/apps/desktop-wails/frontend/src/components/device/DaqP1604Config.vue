<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import CustomSelect from './CustomSelect.vue'
import type { SelectOption } from './CustomSelect.vue'
import {
  Settings2, Activity,
  Save, RotateCcw, CheckCircle2, AlertCircle,
  SlidersHorizontal, Hash, Clock, Timer, Wifi, Gauge, Crosshair,
} from '@lucide/vue'

const props = defineProps<{ deviceId: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
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

// 精度选项（0-6 位小数）：label 跟随当前语言
const precisionOptions = computed<SelectOption[]>(() =>
  Array.from({ length: 7 }, (_, i) => ({
    value: String(i),
    label: i18n.t('common.precisionDecimals', { n: i }),
  })),
)

// 采样频率（Hz），UI 层展示整数频率，保存时换算为周期毫秒
// 频率范围：1 Hz 到 500 Hz
const samplingFreq = ref(10)
const deviceName = ref('') // 设备名称（用户可读的唯一标识，可在此修改）
const autoConnect = ref(false) // 启动时自动连接
const pressureUnit = ref('psi') // 全局压力单位
const globalPrecision = ref(3) // 全局默认精度（小数位数）
const useDeviceTimestamp = ref(true) // 是否使用设备硬件时间戳（默认开启，关闭时回退到系统时间）
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
  if (index === 16) return i18n.t('config.atmosphericPressure')
  if (index === 17) return i18n.t('config.atmosphericTemperature')
  return i18n.t('config.defaultChannelName', { n: index + 1 })
}

/** 采样周期（毫秒）转频率（Hz），限制为 1-500 的整数 */
function periodMsToHz(ms: number): number {
  if (!Number.isFinite(ms) || ms <= 0) return 10
  const hz = Math.round(1000 / ms)
  return Math.max(1, Math.min(500, hz))
}

/** 频率（Hz）转采样周期（毫秒），频率限制为 1-500 的整数 */
function hzToPeriodMs(hz: number): number {
  const normalizedHz = normalizeSamplingFreq(hz)
  return Math.round(1000 / normalizedHz)
}

/** 标准化采样频率，禁止小数并限制在 1-500 Hz */
function normalizeSamplingFreq(value: number): number {
  if (!Number.isFinite(value)) return 10
  return Math.max(1, Math.min(500, Math.trunc(value)))
}

/**
 * 根据通道索引和全局压力单位，返回该通道应当使用的单位字符串。
 *
 * 物理量约束：
 *  - CH1-CH16 压力通道：跟随全局压力单位（硬件 EU 系数统一管理）
 *  - CH17 大气压力：锁定 Pa（独立物理量，不归压力 EU 系数管理）
 *  - CH18 大气温度：锁定 °C
 *
 * 这样保证前端通道卡片显示的单位标签与硬件实际返回的数值语义一致，
 * 避免 CH17/CH18 被错误标注为 psi/kgf/cm² 等压力单位。
 */
function getChannelUnit(index: number, globalPressureUnit: string): string {
  if (index === 16) return 'Pa'
  if (index === 17) return '°C'
  return globalPressureUnit
}

function syncFormFromProfile(profileData: typeof profile.value) {
  if (!profileData) return
  // 保存同步中跳过表单同步，避免触发 watcher 覆盖 saveStatus
  if (syncing.value) return
  deviceName.value = profileData.name || ''
  samplingFreq.value = periodMsToHz(profileData.p1604Config?.samplingRate || 100)
  autoConnect.value = profileData.p1604Config?.autoConnect ?? false
  pressureUnit.value = profileData.p1604Config?.unit || 'psi'
  globalPrecision.value = profileData.p1604Config?.precision ?? 3
  // 时间戳开关默认开启：profile 未设置（老配置）或显式 true 均视为开启
  useDeviceTimestamp.value = profileData.p1604Config?.useDeviceTimestamp ?? true
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

/**
 * CFG-017 智能脏值比较：判断当前表单值是否与 profile 原值一致。
 *
 * 修复背景：原 watcher 仅在表单变化时单向置 hasChanges=true，
 * 用户把参数改回原值后徽章仍残留，违反"未保存更改"语义。
 *
 * 比较范围覆盖表单全部字段：
 *  - 硬件参数（采样频率/单位/全局精度/自动连接/硬件时间戳）
 *  - 通道级参数（名称/启用/颜色/精度）
 *
 * 注意：channelPrecisions 的默认值兜底为 globalPrecision，
 * 与 syncFormFromProfile 的回填逻辑保持一致，避免虚假脏值。
 */
function formEqualsProfile(p: typeof profile.value): boolean {
  if (!p) return false
  const cfg = p.p1604Config
  if (deviceName.value !== (p.name || '')) return false
  if (samplingFreq.value !== periodMsToHz(cfg?.samplingRate || 100)) return false
  if (autoConnect.value !== (cfg?.autoConnect ?? false)) return false
  if (pressureUnit.value !== (cfg?.unit || 'psi')) return false
  if (globalPrecision.value !== (cfg?.precision ?? 3)) return false
  if (useDeviceTimestamp.value !== (cfg?.useDeviceTimestamp ?? true)) return false
  const chs = p.channels
  if (channelNames.value.length !== chs.length) return false
  if (channelEnabled.value.length !== chs.length) return false
  if (channelColors.value.length !== chs.length) return false
  if (channelPrecisions.value.length !== chs.length) return false
  for (let i = 0; i < chs.length; i++) {
    const c = chs[i]
    if (!c) return false
    if ((channelNames.value[i] ?? '') !== (c.name || '')) return false
    if (channelEnabled.value[i] !== c.enabled) return false
    if ((channelColors.value[i] ?? '') !== (c.color || '')) return false
    // 精度回填规则：profile 缺省时兜底为 globalPrecision，需同等比较
    const expectedPrecision = c.precision ?? globalPrecision.value
    if ((channelPrecisions.value[i] ?? globalPrecision.value) !== expectedPrecision) return false
  }
  return true
}

watch([deviceName, samplingFreq, autoConnect, pressureUnit, globalPrecision, useDeviceTimestamp, channelNames, channelEnabled, channelColors, channelPrecisions], () => {
  // 保存同步中跳过，避免覆盖 saveStatus
  if (syncing.value) return
  // CFG-017：改回原值时自动清除徽章；只有真正存在差异时才标记未保存
  hasChanges.value = !formEqualsProfile(profile.value)
  // 任何编辑都重置 saveStatus：上次保存的 success/error 提示对新改动已失效，
  // 残留会让用户误以为当前编辑已保存。与原行为保持一致。
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
  // 时间戳开关变更需要 applyConfig（重启采集切换 content mask）
  // nil 视为 true（默认开启），与后端 UseDeviceTimestampEnabled() 语义一致
  const curTs = cur.useDeviceTimestamp ?? true
  const nxtTs = nxt.useDeviceTimestamp ?? true
  if (curTs !== nxtTs) return true
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
  // 设备名校验：仅当用户实际改动名字时才校验非空 + 全局唯一。
  // - 名字未改动（含 legacy 空名设备保存其他配置）→ 跳过校验，保持原名
  // - 改动后为空 或 与其他设备重名 → 阻止并提示
  const trimmedName = deviceName.value.trim()
  const nameChanged = trimmedName !== (profile.value.name || '')
  if (nameChanged) {
    if (!trimmedName) {
      saveStatus.value = 'error'
      saveMessage.value = i18n.t('config.deviceNameRequired')
      return
    }
    const nameTaken = deviceStore.profiles.some((p) => p.id !== props.deviceId && p.name === trimmedName)
    if (nameTaken) {
      saveStatus.value = 'error'
      saveMessage.value = i18n.t('error.duplicateName')
      return
    }
  }
  saveStatus.value = 'saving'
  saveMessage.value = ''
  syncing.value = true
  try {
    const nextProfile = {
      ...profile.value,
      name: trimmedName,
      p1604Config: {
        ...profile.value.p1604Config,
        samplingRate: hzToPeriodMs(samplingFreq.value),
        autoConnect: autoConnect.value,
        unit: pressureUnit.value,
        precision: globalPrecision.value,
        useDeviceTimestamp: useDeviceTimestamp.value,
      },
      channels: profile.value.channels.map((channel, index) => ({
        ...channel,
        name: channelNames.value[index] || '',
        enabled: channelEnabled.value[index],
        color: channelColors.value[index] || '',
        precision: channelPrecisions.value[index] ?? globalPrecision.value,
        // 同步全局压力单位到 CH1-CH16；CH17 锁 Pa，CH18 锁 °C
        unit: getChannelUnit(index, pressureUnit.value),
      })),
    }
    const hwChanged = hasHardwareConfigChanged(profile.value, nextProfile)
    await deviceStore.saveProfile(nextProfile)
    // 成功后把规范化名字回写表单，避免"尾随空格"等未 trim 值在下次编辑时
    // 触发 formEqualsProfile 永久脏值（CFG-017 回归）
    deviceName.value = trimmedName

    if (hwChanged) {
      try {
        await deviceStore.applyConfig(props.deviceId, nextProfile.p1604Config)
        saveMessage.value = i18n.t('config.savedAndApplied')
      } catch (hwErr) {
        saveMessage.value = hwErr instanceof Error ? hwErr.message : i18n.t('config.hardwareApplyFailed')
      }
    } else {
      saveMessage.value = i18n.t('config.saved')
    }

    saveStatus.value = 'success'
    hasChanges.value = false
    setTimeout(() => { saveStatus.value = 'idle' }, 2000)
  } catch (err) {
    saveStatus.value = 'error'
    saveMessage.value = err instanceof Error ? err.message : i18n.t('config.saveFailed')
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

/** 采样频率输入处理，限制 1-500 Hz 且仅允许整数 */
function onFreqInput(e: Event) {
  const target = e.target as HTMLInputElement
  samplingFreq.value = normalizeSamplingFreq(target.valueAsNumber)
  target.value = String(samplingFreq.value)
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
          <h3 class="config__header-title">{{ profile.name || i18n.t('config.deviceConfig') }}</h3>
          <p class="config__header-subtitle">
            {{ profile.address }}:{{ profile.port }}
            <span v-if="hasChanges" class="config__header-unsaved">{{ i18n.t('common.unsaved') }}</span>
          </p>
        </div>
      </div>
      <button type="button" class="config__close" :title="i18n.t('common.close')" @click.stop="emit('close')">
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
          <h4 class="config__section-title">{{ i18n.t('config.hardwareParams') }}</h4>
        </div>
        <div class="config__section-body">
          <!-- 设备名称：用户可读的唯一标识，可在配置面板修改 -->
          <div class="config__field">
            <label class="config__label">
              <Hash class="config__label-icon" />
              <span>{{ i18n.t('config.deviceName') }}</span>
            </label>
            <input
              v-model="deviceName"
              class="config__text-input"
              type="text"
              :placeholder="i18n.t('config.deviceNamePlaceholder')"
              :disabled="isAcquiring"
            />
          </div>

          <!-- 采样频率（Hz） -->
          <div class="config__field">
            <label class="config__label">
              <Clock class="config__label-icon" />
              <span>{{ i18n.t('config.samplingRate') }}</span>
            </label>
            <div class="config__rate-wrapper">
              <input
                v-model.number="samplingFreq"
                type="number"
                step="1"
                class="config__rate-input"
                :min="1"
                :max="500"
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
              <span>{{ i18n.t('config.pressureUnit') }}</span>
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
              <span>{{ i18n.t('config.globalPrecision') }}</span>
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
                :title="i18n.t('config.applyGlobalPrecisionTitle')"
                @click="applyGlobalPrecisionToAll"
              >
                {{ i18n.t('common.applyToAll') }}
              </button>
            </div>
          </div>

          <!-- 自动连接开关：启动应用时自动连接此设备 -->
          <div class="config__field">
            <label class="config__label">
              <Wifi class="config__label-icon" />
              <span>{{ i18n.t('config.autoConnect') }}</span>
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
              <span class="config__toggle-text">{{ autoConnect ? i18n.t('common.enabled') : i18n.t('common.disabled') }}</span>
            </button>
          </div>

          <!-- 设备硬件时间戳开关：开启后采集帧含硬件时间戳（更精确），关闭时回退到系统时间 -->
          <div class="config__field">
            <label class="config__label">
              <Timer class="config__label-icon" />
              <span>{{ i18n.t('config.deviceTimestamp') }}</span>
            </label>
            <button
              type="button"
              class="config__toggle"
              :class="{ 'config__toggle--on': useDeviceTimestamp }"
              :disabled="isAcquiring"
              @click="useDeviceTimestamp = !useDeviceTimestamp"
            >
              <span class="config__toggle-track">
                <span class="config__toggle-thumb"></span>
              </span>
              <span class="config__toggle-text">{{ useDeviceTimestamp ? i18n.t('common.enabled') : i18n.t('common.disabled') }}</span>
            </button>
          </div>
        </div>
        <p v-if="isAcquiring" class="config__section-hint">{{ i18n.t('config.acquireLockHint') }}</p>
      </section>

      <!-- 通道列表 -->
      <section class="config__section">
        <div class="config__section-header">
          <Activity class="config__section-icon" />
          <h4 class="config__section-title">{{ i18n.t('config.channelConfig') }}</h4>
          <span class="config__section-badge">{{ i18n.t('config.channelEnabledCount', { n: enabledCount }) }}</span>
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
                <span v-if="i === 17" class="config__channel-badge">{{ i18n.t('config.atmosphericPressure') }}</span>
                <span v-else-if="i === 18" class="config__channel-badge">{{ i18n.t('config.atmosphericTemperature') }}</span>
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
                  <span class="config__channel-precision-label">{{ i18n.t('common.precision') }}</span>
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
                  :title="channelEnabled[i - 1] ? i18n.t('config.disableChannel') : i18n.t('config.enableChannel')"
                  :disabled="isAcquiring"
                  @click="toggleChannel(i - 1)"
                >
                  <span class="config__toggle-track">
                    <span class="config__toggle-thumb"></span>
                  </span>
                  <span class="config__toggle-text">{{ channelEnabled[i - 1] ? i18n.t('common.on') : i18n.t('common.off') }}</span>
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
        <span>{{ saveMessage || (saveStatus === 'saving' ? i18n.t('common.saving') : '') }}</span>
      </div>
      <div v-else class="config__status config__status--idle">
        <span v-if="isAcquiring">{{ i18n.t('config.acquireLock') }}</span>
        <span v-else-if="hasChanges">{{ i18n.t('common.unsavedChanges') }}</span>
        <span v-else>{{ i18n.t('common.saved') }}</span>
      </div>
      <div class="config__actions">
        <button type="button" class="config__btn config__btn--secondary" :disabled="isAcquiring || saveStatus === 'saving'" @click.stop="resetConfig">
          <RotateCcw class="config__btn-icon" />
          <span>{{ i18n.t('common.reset') }}</span>
        </button>
        <button
          type="button"
          class="config__btn config__btn--primary"
          :disabled="saveStatus === 'saving'"
          @click.stop="saveConfig"
        >
          <Save class="config__btn-icon" />
          <span>{{ i18n.t('common.save') }}</span>
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

/* 文本输入（设备名称等） */
.config__text-input {
  width: 100%;
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: var(--bg-input, var(--bg-panel));
  color: var(--text-primary);
  font-size: var(--font-size-sm);
  font-weight: 700;
  outline: none;
  transition: border-color var(--motion-fast) var(--easing-standard);
}

.config__text-input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-muted);
}

.config__text-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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
