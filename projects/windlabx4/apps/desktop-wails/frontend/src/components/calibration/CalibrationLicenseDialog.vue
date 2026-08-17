<script setup lang="ts">
/**
 * 探针校准模块许可证解锁对话框。
 *
 * 触发时机：用户点击侧边栏「探针校准」入口且尚未解锁时弹出。
 * 交互流程：
 *   1. 提示此为付费模块，请输入验证码；
 *   2. 用户输入验证码并确认——正确则持久化解锁状态并放行；
 *   3. 错误或取消——不进入探针校准画面。
 *
 * 设计要点：
 *   - 对话框内部不直接跳转页面，只 emit「unlocked / cancel」两个语义事件，
 *     由父组件决定后续导航行为，保持组件职责单一。
 *   - 输入框使用 text type 让用户能确认输入内容（验证码含 @ 等特殊字符，
 *     遮蔽反而易输错）；错误信息内联显示，不弹 toast，避免遮挡连续输入。
 *   - 关闭（点遮罩/右上角 ✕/Esc）统一归为 cancel 语义。
 */
import { ref, watch } from 'vue'
import { Lock, ShieldCheck, AlertCircle } from '@lucide/vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import { useCalibrationLicenseStore } from '@stores/calibrationLicenseStore'

const props = defineProps<{
  show: boolean
  /** i18n 文案表，由父组件透传 */
  t: Record<string, string>
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  /** 验证码正确，已解锁——父组件可据此放行进入校准画面 */
  (e: 'unlocked'): void
  /** 用户取消或关闭——父组件应保持原页面，不跳转 */
  (e: 'cancel'): void
}>()

const licenseStore = useCalibrationLicenseStore()

// 输入的验证码——每次打开对话框时清空，避免残留上次输入
const code = ref('')
// 错误提示——仅在验证失败时显示，关闭对话框时清空
const errorMsg = ref('')

// 对话框打开/关闭时都重置输入与错误状态：
// - 打开时清空，保证每次进入都是干净状态；
// - 关闭时清空，避免半截输入残留——当前组件始终挂载（无 v-if），
//   但若以后改为 v-if 卸载，watch 不会再触发，故此处主动清理更稳健。
watch(
  () => props.show,
  () => {
    code.value = ''
    errorMsg.value = ''
  },
)

/** 提交验证码：成功则 emit unlocked，失败则显示错误 */
function handleSubmit(): void {
  const trimmed = code.value.trim()
  if (!trimmed) {
    errorMsg.value = props.t.calLicensePleaseInput || '请输入验证码'
    return
  }
  const ok = licenseStore.unlock(trimmed)
  if (ok) {
    errorMsg.value = ''
    code.value = ''
    emit('update:show', false)
    emit('unlocked')
  } else {
    errorMsg.value = props.t.calLicenseInvalidCode || '验证码不正确，请重新输入'
  }
}

/** 取消：清空状态并通知父组件 */
function handleCancel(): void {
  code.value = ''
  errorMsg.value = ''
  emit('update:show', false)
  emit('cancel')
}

/** 遮罩点击 / 右上角 ✕ / Esc：UiDialog 通过 update:show=false 通知，统一归为 cancel */
function handleShowChange(v: boolean): void {
  if (!v) {
    handleCancel()
  }
}

/** 回车键提交，提升输入效率 */
function handleKeydown(e: KeyboardEvent): void {
  if (e.key === 'Enter') {
    e.preventDefault()
    handleSubmit()
  }
}
</script>

<template>
  <UiDialog
    :show="show"
    :title="t.calLicenseTitle || '探针校准模块'"
    width="440px"
    :closable="true"
    @update:show="handleShowChange"
  >
    <div class="cal-license">
      <!-- 顶部图标 + 付费提示 -->
      <div class="cal-license__hero">
        <div class="cal-license__icon">
          <Lock :size="22" />
        </div>
        <div class="cal-license__hero-text">
          <p class="cal-license__heading">
            {{ t.calLicensePaidModule || '此为付费模块' }}
          </p>
          <p class="cal-license__hint">
            {{ t.calLicenseHint || '请输入验证码以解锁探针校准功能。解锁后此设备可永久使用。' }}
          </p>
        </div>
      </div>

      <!-- 验证码输入区 -->
      <div class="cal-license__field">
        <label class="cal-license__label" for="cal-license-code">
          <ShieldCheck :size="14" />
          <span>{{ t.calLicenseCodeLabel || '验证码' }}</span>
        </label>
        <UiInput
          id="cal-license-code"
          v-model="code"
          type="text"
          :placeholder="t.calLicensePlaceholder || '请输入验证码'"
          :aria-label="t.calLicenseCodeLabel || '验证码'"
          autocomplete="off"
          @keydown="handleKeydown"
        />
        <!-- 错误提示内联显示，避免 toast 遮挡输入 -->
        <p v-if="errorMsg" class="cal-license__error">
          <AlertCircle :size="13" />
          <span>{{ errorMsg }}</span>
        </p>
      </div>
    </div>

    <template #footer>
      <div class="cal-license__actions">
        <UiButton quaternary size="sm" @click="handleCancel">
          {{ t.calLicenseCancel || '取消' }}
        </UiButton>
        <UiButton variant="primary" size="sm" @click="handleSubmit">
          {{ t.calLicenseConfirm || '确认解锁' }}
        </UiButton>
      </div>
    </template>
  </UiDialog>
</template>

<style scoped>
.cal-license {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 0.25rem 0;
}

.cal-license__hero {
  display: flex;
  gap: 0.75rem;
  align-items: flex-start;
}

.cal-license__icon {
  flex-shrink: 0;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 使用 accent 色族，传达「受保护」语义而非「警告」语义 */
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
  color: var(--accent-primary);
}

.cal-license__hero-text {
  flex: 1;
  min-width: 0;
}

.cal-license__heading {
  margin: 0;
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.4;
}

.cal-license__hint {
  margin: 0.375rem 0 0;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  line-height: 1.5;
}

.cal-license__field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.cal-license__label {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-secondary);
}

.cal-license__error {
  margin: 0.25rem 0 0;
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.75rem;
  color: var(--accent-danger);
}

.cal-license__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}
</style>
