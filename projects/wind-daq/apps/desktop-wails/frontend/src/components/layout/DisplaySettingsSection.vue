<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18nStore } from '@stores/i18nStore'
import {
  type StorageSettings,
  DEFAULT_SETTINGS,
  DEFAULT_REFRESH_RATE_HZ,
  REFRESH_RATE_MIN,
  REFRESH_RATE_MAX,
  clampRefreshHz,
  WAVEFORM_BUFFER_MIN,
  WAVEFORM_BUFFER_MAX,
  WAVEFORM_BUFFER_STEP,
} from '@stores/storageStore'
import { useThemeStore } from '@stores/themeStore'
import { deviceApi } from '@api/deviceApi'
import UiButton from '@components/ui/UiButton.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSlider from '@components/ui/UiSlider.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import {
  Activity,
  Globe,
  Monitor,
  Moon,
  RefreshCw,
  Sun,
} from '@lucide/vue'

const themeStore = useThemeStore()
const i18nStore = useI18nStore()
const { theme } = storeToRefs(themeStore)
const { locale } = storeToRefs(i18nStore)

const refreshRate = ref(DEFAULT_REFRESH_RATE_HZ)
const waveformBufferSize = ref(DEFAULT_SETTINGS.waveformBufferSize)

/** 字段级校验错误记录 */
const validationErrors = ref<Record<string, string>>({})

/** 从业务级 StorageSettings 载入界面相关字段。
 *  刷新率优先用持久化值（settings.refreshRateHz），不再 fallback 到后端 getPublishRate()——
 *  后端 AcquisitionHub 重启后恒为默认 20Hz，若以它为准会覆盖用户保存的值。持久化值才是真相。 */
async function load(settings: StorageSettings): Promise<void> {
  validationErrors.value = {}
  // 对从 store 读入的值做边界限制，确保 UI 展示合法
  waveformBufferSize.value = Math.max(WAVEFORM_BUFFER_MIN, Math.min(WAVEFORM_BUFFER_MAX, settings.waveformBufferSize ?? DEFAULT_SETTINGS.waveformBufferSize))
  const hz = clampRefreshHz(settings.refreshRateHz ?? DEFAULT_REFRESH_RATE_HZ)
  refreshRate.value = hz
}

/** 校验指定字段，返回错误信息或空字符串 */
function validateField(field: string): string {
  switch (field) {
    case 'refreshRate':
      return refreshRate.value < REFRESH_RATE_MIN || refreshRate.value > REFRESH_RATE_MAX
        ? `刷新率范围为 ${REFRESH_RATE_MIN} 到 ${REFRESH_RATE_MAX} Hz` : ''
    case 'waveformBufferSize':
      return waveformBufferSize.value < WAVEFORM_BUFFER_MIN || waveformBufferSize.value > WAVEFORM_BUFFER_MAX
        ? `波形图缓冲区点数范围为 ${WAVEFORM_BUFFER_MIN} 到 ${WAVEFORM_BUFFER_MAX}` : ''
    default:
      return ''
  }
}

/** 更新单个字段的校验状态 */
function updateFieldError(field: string): void {
  const error = validateField(field)
  if (error) {
    validationErrors.value = { ...validationErrors.value, [field]: error }
  } else {
    const { [field]: _, ...rest } = validationErrors.value
    validationErrors.value = rest as Record<string, string>
  }
}

/** 全量校验，返回错误映射（空对象表示通过） */
function validate(): Record<string, string> {
  const errs: Record<string, string> = {}
  for (const field of ['refreshRate', 'waveformBufferSize']) {
    const error = validateField(field)
    if (error) errs[field] = error
  }
  validationErrors.value = errs
  return errs
}

/** 恢复默认设置 */
function reset(): void {
  refreshRate.value = DEFAULT_REFRESH_RATE_HZ
  waveformBufferSize.value = DEFAULT_SETTINGS.waveformBufferSize
  validationErrors.value = {}
}

/** 保存刷新率：始终下发后端，即时生效。刷新率的持久化由父组件合入 StorageSettings 统一保存。
 *  不做"仅变化时下发"的跳过优化——若启动时 setPublishRate 失败（后端停留在默认 20Hz），
 *  后续保存（即便用户只改了波形点数）若跳过下发，会导致后端与持久化值长期不一致且无法自愈。
 *  始终下发代价很低（一次本地 binding 调用），换来后端状态始终对齐持久化值。 */
async function save(): Promise<void> {
  await deviceApi.setPublishRate(refreshRate.value)
}

defineExpose({ load, save, reset, validate, waveformBufferSize, refreshRate })
</script>

<template>
  <section
    id="settings-panel-display"
    role="tabpanel"
    aria-labelledby="settings-tab-display"
    class="settings-section"
  >
    <UiPanel :segmented="false" class="form-card">
      <template #header>
        <div class="card-head">
          <Monitor :size="15" />
          <span class="card-head__title">外观与语言</span>
        </div>
      </template>
      <div class="form-fields">
        <!-- 主题切换 -->
        <UiFormField label="主题模式">
          <div class="theme-switch">
            <UiButton
              size="md"
              :variant="theme === 'light' ? 'primary' : 'ghost'"
              aria-label="切换为浅色主题"
              data-test="settings-theme-light"
              @click="themeStore.setTheme('light')"
            >
              <template #icon><Sun :size="14" /></template>浅色
            </UiButton>
            <UiButton
              size="md"
              :variant="theme === 'dark' ? 'primary' : 'ghost'"
              aria-label="切换为深色主题"
              data-test="settings-theme-dark"
              @click="themeStore.setTheme('dark')"
            >
              <template #icon><Moon :size="14" /></template>深色
            </UiButton>
          </div>
        </UiFormField>
        <!-- 语言切换 -->
        <UiFormField label="界面语言">
          <div class="locale-switch">
            <button
              class="locale-btn"
              :class="{ 'locale-btn--active': locale === 'zh' }"
              aria-label="切换为中文界面"
              data-test="settings-locale-zh"
              @click="i18nStore.setLocale('zh')"
            >
              <Globe :size="12" />中文
            </button>
            <button
              class="locale-btn"
              :class="{ 'locale-btn--active': locale === 'en' }"
              aria-label="Switch interface language to English"
              data-test="settings-locale-en"
              @click="i18nStore.setLocale('en')"
            >
              <Globe :size="12" />English
            </button>
          </div>
        </UiFormField>
      </div>
    </UiPanel>

    <!-- 刷新率 -->
    <UiPanel class="form-card">
      <template #header>
        <div class="card-head">
          <RefreshCw :size="15" />
          <span class="card-head__title">刷新率</span>
        </div>
      </template>
      <div class="form-fields">
        <UiFormField
          label="实时数据刷新频率"
          :error="validationErrors.refreshRate"
          hint="较高的刷新率会占用更多系统资源"
        >
          <div class="refresh-row">
            <div class="refresh-slider">
              <UiSlider v-model="refreshRate" :min="1" :max="20" :step="1" aria-label="实时数据刷新频率" />
              <div class="refresh-labels">
                <span class="refresh-label">1 Hz</span>
                <span
                  class="refresh-label refresh-label--highlight"
                  :class="{ 'refresh-label--active': refreshRate >= 5 && refreshRate <= 15 }"
                >推荐 5–15 Hz</span>
                <span class="refresh-label">20 Hz</span>
              </div>
            </div>
            <div class="refresh-value">
              <UiInputNumber
                v-model="refreshRate"
                :min="1"
                :max="20"
                size="small"
                @blur="updateFieldError('refreshRate')"
              />
              <span class="input-unit">Hz</span>
            </div>
          </div>
        </UiFormField>
      </div>
    </UiPanel>

    <!-- 波形图缓冲区 -->
    <UiPanel class="form-card">
      <template #header>
        <div class="card-head">
          <Activity :size="15" />
          <span class="card-head__title">波形图</span>
        </div>
      </template>
      <div class="form-fields">
        <UiFormField
          label="波形图缓冲区点数"
          :error="validationErrors.waveformBufferSize"
          hint="较大的缓冲区可显示更长时间趋势，但会占用更多内存"
        >
          <div class="refresh-row">
            <div class="refresh-slider">
              <UiSlider
                v-model="waveformBufferSize"
                :min="WAVEFORM_BUFFER_MIN"
                :max="WAVEFORM_BUFFER_MAX"
                :step="WAVEFORM_BUFFER_STEP"
                aria-label="波形图缓冲区点数"
              />
              <div class="refresh-labels">
                <span class="refresh-label">{{ WAVEFORM_BUFFER_MIN }} 点</span>
                <span
                  class="refresh-label refresh-label--highlight"
                  :class="{ 'refresh-label--active': waveformBufferSize >= 100 && waveformBufferSize <= 500 }"
                >推荐 100–500 点</span>
                <span class="refresh-label">{{ WAVEFORM_BUFFER_MAX }} 点</span>
              </div>
            </div>
            <div class="refresh-value">
              <UiInputNumber
                v-model="waveformBufferSize"
                :min="WAVEFORM_BUFFER_MIN"
                :max="WAVEFORM_BUFFER_MAX"
                :step="WAVEFORM_BUFFER_STEP"
                size="small"
                @blur="updateFieldError('waveformBufferSize')"
              />
              <span class="input-unit">点</span>
            </div>
          </div>
        </UiFormField>
      </div>
    </UiPanel>
  </section>
</template>
