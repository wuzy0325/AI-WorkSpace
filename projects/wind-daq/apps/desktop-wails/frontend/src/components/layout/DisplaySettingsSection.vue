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
        ? i18nStore.t.set_refreshRateRangeError
          .replace('{min}', String(REFRESH_RATE_MIN))
          .replace('{max}', String(REFRESH_RATE_MAX)) : ''
    case 'waveformBufferSize':
      return waveformBufferSize.value < WAVEFORM_BUFFER_MIN || waveformBufferSize.value > WAVEFORM_BUFFER_MAX
        ? i18nStore.t.set_waveformBufferRangeError
          .replace('{min}', String(WAVEFORM_BUFFER_MIN))
          .replace('{max}', String(WAVEFORM_BUFFER_MAX)) : ''
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
          <span class="card-head__title">{{ i18nStore.t.set_appearanceLanguage }}</span>
        </div>
      </template>
      <div class="form-fields">
        <!-- 主题切换 -->
        <UiFormField :label="i18nStore.t.set_themeMode">
          <div class="theme-switch">
            <UiButton
              size="md"
              :variant="theme === 'light' ? 'primary' : 'ghost'"
              :aria-label="i18nStore.t.set_toggleToLightTheme"
              data-test="settings-theme-light"
              @click="themeStore.setTheme('light')"
            >
              <template #icon><Sun :size="14" /></template>{{ i18nStore.t.set_light }}
            </UiButton>
            <UiButton
              size="md"
              :variant="theme === 'dark' ? 'primary' : 'ghost'"
              :aria-label="i18nStore.t.set_toggleToDarkTheme"
              data-test="settings-theme-dark"
              @click="themeStore.setTheme('dark')"
            >
              <template #icon><Moon :size="14" /></template>{{ i18nStore.t.set_dark }}
            </UiButton>
          </div>
        </UiFormField>
        <!-- 语言切换 -->
        <UiFormField :label="i18nStore.t.set_interfaceLanguage">
          <div class="locale-switch">
            <button
              class="locale-btn"
              :class="{ 'locale-btn--active': locale === 'zh' }"
              :aria-label="i18nStore.t.set_switchToChinese"
              data-test="settings-locale-zh"
              @click="i18nStore.setLocale('zh')"
            >
              <Globe :size="12" />{{ i18nStore.t.set_chinese }}
            </button>
            <button
              class="locale-btn"
              :class="{ 'locale-btn--active': locale === 'en' }"
              :aria-label="i18nStore.t.set_switchToEnglish"
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
          <span class="card-head__title">{{ i18nStore.t.set_refreshRate }}</span>
        </div>
      </template>
      <div class="form-fields">
        <UiFormField
          :label="i18nStore.t.set_refreshFrequency"
          :error="validationErrors.refreshRate"
          :hint="i18nStore.t.set_refreshHint"
        >
          <div class="refresh-row">
            <div class="refresh-slider">
              <UiSlider v-model="refreshRate" :min="1" :max="20" :step="1" :aria-label="i18nStore.t.set_refreshFrequency" />
              <div class="refresh-labels">
                <span class="refresh-label">1 Hz</span>
                <span
                  class="refresh-label refresh-label--highlight"
                  :class="{ 'refresh-label--active': refreshRate >= 5 && refreshRate <= 15 }"
                >{{ i18nStore.t.set_recommendedRefresh }}</span>
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
          <span class="card-head__title">{{ i18nStore.t.set_waveform }}</span>
        </div>
      </template>
      <div class="form-fields">
        <UiFormField
          :label="i18nStore.t.waveformBufferSizeLabel"
          :error="validationErrors.waveformBufferSize"
          :hint="i18nStore.t.waveformBufferSizeHint"
        >
          <div class="refresh-row">
            <div class="refresh-slider">
              <UiSlider
                v-model="waveformBufferSize"
                :min="WAVEFORM_BUFFER_MIN"
                :max="WAVEFORM_BUFFER_MAX"
                :step="WAVEFORM_BUFFER_STEP"
                :aria-label="i18nStore.t.waveformBufferSizeLabel"
              />
              <div class="refresh-labels">
                <span class="refresh-label">{{ WAVEFORM_BUFFER_MIN }} {{ i18nStore.t.pts }}</span>
                <span
                  class="refresh-label refresh-label--highlight"
                  :class="{ 'refresh-label--active': waveformBufferSize >= 100 && waveformBufferSize <= 500 }"
                >{{ i18nStore.t.set_recommendedBuffer }}</span>
                <span class="refresh-label">{{ WAVEFORM_BUFFER_MAX }} {{ i18nStore.t.pts }}</span>
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
              <span class="input-unit">{{ i18nStore.t.pts }}</span>
            </div>
          </div>
        </UiFormField>
      </div>
    </UiPanel>
  </section>
</template>
