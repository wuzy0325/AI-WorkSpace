<template>
  <!-- 核对码弹窗：不可关闭绕过（无 X 按钮、无遮罩关闭） -->
  <div
    v-if="visible"
    class="verification-overlay"
  >
    <div class="verification-dialog">
      <header class="dialog-header">
        <span class="warning-icon">⚠</span>
        <h3>物理切换确认</h3>
      </header>

      <div class="dialog-body">
        <p class="hint-text">
          请确保已将标准器切换为（满量程上限）：
        </p>

        <!-- 大字显示量程上限（与标准器物理标识一致），方便操作员比对 -->
        <div class="range-display">
          <span class="range-value">{{ batch.rangeMax }}</span>
          <span class="range-unit">{{ batch.rangeUnit }}</span>
        </div>

        <p class="hint-text">
          请输入标准器量程值以确认切换：
        </p>

        <input
          ref="codeInputRef"
          v-model="code"
          type="text"
          class="code-input"
          placeholder="请输入量程值"
          :class="{ error: !!errorMessage }"
          @keyup.enter="handleConfirm"
          @input="errorMessage = ''"
        >

        <p
          v-if="errorMessage"
          class="error-message"
        >
          {{ errorMessage }}
        </p>
      </div>

      <footer class="dialog-footer">
        <button
          class="cancel-btn"
          @click="handleCancel"
        >
          取消
        </button>
        <button
          class="confirm-btn"
          :disabled="!code.trim()"
          @click="handleConfirm"
        >
          确认切换
        </button>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { type BatchGroup } from '@/types/batch'

const props = defineProps<{
  /** 是否显示弹窗 */
  visible: boolean
  /** 当前批次信息（显示量程值） */
  batch: BatchGroup
}>()

const emit = defineEmits<{
  /** 校验通过时触发，携带 batchId 和输入的 code */
  verified: [batchId: string, code: string]
  /** 取消时触发（留在当前状态） */
  cancel: []
}>()

const code = ref<string>('')
const errorMessage = ref<string>('')
const codeInputRef = ref<HTMLInputElement | null>(null)

// 弹窗显示时自动聚焦输入框
watch(
  () => props.visible,
  async (visible) => {
    if (visible) {
      code.value = ''
      errorMessage.value = ''
      await nextTick()
      codeInputRef.value?.focus()
    }
  }
)

// 确认：前端只做格式校验（非空 + 合法数值），数值匹配交给后端做权威校验。
// 这样后端调整容差策略或匹配规则时，前端无需同步修改，且 setError 路径可真正被触发。
const handleConfirm = async (): Promise<void> => {
  const trimmed = code.value.trim()
  if (!trimmed) {
    errorMessage.value = '请输入量程值'
    return
  }

  // 前端预校验：必须是有效数值（格式层面）
  const inputValue = parseFloat(trimmed)
  if (isNaN(inputValue)) {
    errorMessage.value = '请输入有效的量程数值'
    return
  }

  // 前端格式校验通过，通知父组件调用后端做最终数值匹配校验
  emit('verified', props.batch.batchId, trimmed)
}

const handleCancel = (): void => {
  emit('cancel')
}

// 暴露给父组件：设置错误信息（后端校验失败时调用）
defineExpose({
  setError: (msg: string): void => {
    errorMessage.value = msg
  },
  reset: (): void => {
    code.value = ''
    errorMessage.value = ''
  }
})
</script>

<style scoped lang="scss">
.verification-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.verification-dialog {
  background: #fff;
  border-radius: 12px;
  padding: 24px;
  width: 420px;
  max-width: 90vw;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
}

.dialog-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.warning-icon {
  font-size: 24px;
  color: $warning-500;
}

.dialog-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: $slate-800;
}

.dialog-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.hint-text {
  margin: 0;
  color: $slate-700;
  font-size: 14px;
}

.range-display {
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: 6px;
  padding: 16px;
  background: $slate-50;
  border-radius: 8px;
  margin: 4px 0 12px;
}

.range-value {
  font-size: 36px;
  font-weight: 700;
  color: $mint-dark;
  font-family: 'Consolas', 'Monaco', monospace;
}

.range-unit {
  font-size: 18px;
  color: $slate-500;
  font-weight: 600;
}

.code-input {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  font-size: 16px;
  box-sizing: border-box;
  transition: all 0.15s ease;

  &:focus {
    outline: none;
    border-color: $mint;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  }
}

.code-input.error {
  border-color: $danger-500;
  background: $danger-50;
}

.error-message {
  margin: 0;
  color: $danger-500;
  font-size: 13px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

/* 取消按钮：slate 半透明次按钮 */
.cancel-btn {
  padding: 8px 16px;
  background: rgba(55, 65, 81, 0.08);
  border: 1px solid $slate-200;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  color: $slate-700;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }
}

/* 确认按钮：mint 渐变主按钮 */
.confirm-btn {
  padding: 8px 16px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  min-width: 100px;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
  transition: all 0.15s ease;

  &:disabled {
    background: $slate-300;
    cursor: not-allowed;
    box-shadow: none;
  }

  &:not(:disabled):hover {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.4);
    transform: translateY(-1px);
  }
}
</style>
