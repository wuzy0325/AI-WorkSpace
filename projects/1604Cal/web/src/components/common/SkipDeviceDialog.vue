<template>
  <el-dialog
    v-model="visible"
    title="跳过计量设备"
    width="440px"
    :close-on-click-modal="false"
    :show-close="false"
    draggable
  >
    <div class="skip-body">
      <el-alert
        type="warning"
        :closable="false"
        show-icon
      >
        <template #title>
          <span>确认永久跳过该计量设备？</span>
        </template>
        <template #default>
          <div class="skip-hint">
            <span v-if="deviceName">
              设备：{{ deviceName }}
            </span>
            <span>
              后续压力点将不再采集该设备，已完成数据将保留。
            </span>
          </div>
        </template>
      </el-alert>

      <div class="skip-form">
        <div class="form-label">
          跳过原因
        </div>
        <el-radio-group
          v-model="reason"
          class="reason-group"
        >
          <el-radio
            v-for="opt in presetReasons"
            :key="opt"
            :value="opt"
          >
            {{ opt }}
          </el-radio>
        </el-radio-group>
        <!-- 备注（可选）：填写后拼接为「预设原因 - 备注」，符合 spec 场景 -->
        <el-input
          v-model="note"
          type="textarea"
          :rows="2"
          placeholder="可选备注，如：线缆接触不良"
          maxlength="100"
          show-word-limit
        />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="visible = false">
          取消
        </el-button>
        <el-button
          type="danger"
          :disabled="!finalReason"
          @click="confirm"
        >
          跳过该设备
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'

const visible = defineModel<boolean>('visible', { default: false })

defineProps<{
  /** 设备显示名（用于提示文案） */
  deviceName?: string
}>()

const emit = defineEmits<{
  confirm: [reason: string]
}>()

/** 预设跳过原因，与后端语义对齐；「其他」时备注必填作为原因 */
const presetReasons = ['设备故障', '采集超时', '人工放弃', '其他']

const reason = ref('人工放弃')
const note = ref('')

// 每次打开弹窗时重置为默认原因，避免上次选择残留。
watch(visible, (v) => {
  if (v) {
    reason.value = '人工放弃'
    note.value = ''
  }
})

// 最终原因：「其他」时取备注本身；预设项时若填了备注拼接为「预设 - 备注」（spec 场景）。
const finalReason = computed(() => {
  const trimmedNote = note.value.trim()
  if (reason.value === '其他') return trimmedNote
  return trimmedNote ? `${reason.value} - ${trimmedNote}` : reason.value
})

function confirm() {
  if (!finalReason.value) return
  emit('confirm', finalReason.value)
  visible.value = false
}
</script>

<style scoped lang="scss">
.skip-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.skip-hint {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.skip-form {
  display: flex;
  flex-direction: column;
  gap: 10px;

  .form-label {
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 500;
  }

  .reason-group {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
