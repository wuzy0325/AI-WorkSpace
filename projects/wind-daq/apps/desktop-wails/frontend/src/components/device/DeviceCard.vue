<script setup lang="ts">
/**
 * DeviceCard
 *
 * 设备列表卡片：被 DeviceManagementDrawer 在"已连接 / 连接中 / 等待连接"
 * 三组中复用，消除原模板中约 100 行的重复。
 *
 * 设计要点：
 *   - 子组件不直接访问 deviceStore，所有状态通过 props 显式传入，便于单测。
 *   - 按钮事件通过 emit 上抛给父组件，由父组件统一编排（连接/断开/采集/删除）。
 *   - 按"连接分组"（connected / connecting / pending）决定按钮可见性与禁用态，
 *     保持与原模板完全等价的交互。
 */
import { Plug, Zap, LayoutGrid, AlertCircle } from '@lucide/vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiButton from '@components/ui/UiButton.vue'
import type { DeviceProfile } from '@api/types'

type ConnectionGroup = 'connected' | 'connecting' | 'pending'

const props = defineProps<{
  // 设备配置
  profile: DeviceProfile
  // 当前所在分组（决定按钮组合）
  group: ConnectionGroup
  // 后端连接状态字符串：'Connected' | 'Connecting' | 'Error' | 'Disconnected' | 'Idle'
  // 使用 string 而非接口 DeviceStatus，是因为 deviceStore.statusFor() 返回的就是
  // connection 子字段（字符串字面值），保持与父组件传值类型一致。
  status: string | undefined
  // 是否正在采集
  acquiring: boolean
  // 当前是否被勾选
  selected: boolean
  // 状态条纹的 CSS 类（沿用父组件 statusClass(p) 计算）
  statusClass: string
  // 连接按钮显示文字（"连接" / "断开" / "重试"...）
  connectLabel: string
}>()

const emit = defineEmits<{
  (e: 'toggle-selected'): void
  (e: 'edit'): void
  (e: 'connect-toggle'): void
  (e: 'toggle-acquisition'): void
  (e: 'remove'): void
}>()

// 仅在"已连接"分组中显示采集按钮，避免连接中/等待连接时误操作
const showAcquireBtn = (): boolean => props.group === 'connected' && props.status === 'Connected'

// 错误徽章只在 pending 分组且后端报 Error 时显示
const showErrorBadge = (): boolean => props.group === 'pending' && props.status === 'Error'
</script>

<template>
  <div class="device-card" :class="[statusClass]">
    <div class="device-card-stripe" :class="[statusClass]" />
    <div class="device-card-body">
      <div class="device-card-left">
        <div class="device-card-row">
          <UiCheckbox :checked="selected" @update:checked="emit('toggle-selected')" />
          <h3 class="device-card-name">{{ profile.name }}</h3>
          <span class="device-card-type-badge">{{ profile.type }}</span>
        </div>
        <div class="device-card-meta">
          <span><Plug class="meta-icon" /> {{ profile.transport === 'serial' ? (profile.serialPort || 'COM?') : `${profile.address || '-'}:${profile.port || '-'}` }}</span>
          <span><Zap class="meta-icon" /> {{ profile.samplingRate ?? 20 }}Hz</span>
          <span><LayoutGrid class="meta-icon" /> {{ profile.channels?.length ?? 0 }} 通道</span>
        </div>
      </div>
      <div class="device-card-right">
        <UiButton secondary size="md" @click="emit('edit')">编辑</UiButton>
        <!-- 连接按钮：connecting 分组禁用并显示 spinner -->
        <UiButton
          size="md"
          :disabled="group === 'connecting'"
          @click="emit('connect-toggle')"
        >
          <span v-if="group === 'connecting'" class="inline-spinner" aria-hidden="true"></span>
          {{ connectLabel }}
        </UiButton>
        <UiButton v-if="showAcquireBtn()" size="md" @click="emit('toggle-acquisition')">
          {{ acquiring ? '停止' : '采集' }}
        </UiButton>
        <UiButton variant="danger" size="md" secondary @click="emit('remove')">删除</UiButton>
      </div>
    </div>
    <div v-if="showErrorBadge()" class="device-card-error">
      <AlertCircle class="error-icon" :size="14" />
      设备通信错误
    </div>
  </div>
</template>
