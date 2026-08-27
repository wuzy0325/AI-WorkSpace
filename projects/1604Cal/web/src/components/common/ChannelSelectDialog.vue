<template>
  <el-dialog
    :model-value="visible"
    title="通道选择"
    width="380px"
    class="channel-select-dialog"
    append-to-body
    :close-on-click-modal="false"
    @update:model-value="$emit('update:visible', $event)"
  >
    <ChannelMatrix
      :selected-channels="localChannels"
      @update:selected-channels="localChannels = $event"
    />
    <template #footer>
      <el-button @click="$emit('update:visible', false)">
        取消
      </el-button>
      <el-button
        type="primary"
        @click="handleConfirm"
      >
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import ChannelMatrix from '@/components/common/ChannelMatrix.vue'

const props = defineProps<{
  visible: boolean
  selectedChannels: number[]
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: [channels: number[]]
}>()

const localChannels = ref<number[]>([])

watch(() => props.visible, (val) => {
  if (val) {
    localChannels.value = [...props.selectedChannels]
  }
})

function handleConfirm() {
  emit('confirm', [...localChannels.value])
  emit('update:visible', false)
}
</script>


