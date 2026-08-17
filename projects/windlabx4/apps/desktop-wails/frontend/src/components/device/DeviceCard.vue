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
import { storeToRefs } from 'pinia'
import { useI18nStore } from '@stores/i18nStore'
import type { DeviceProfile } from '@api/types'

// 用 storeToRefs 保持 i18n 响应性：直接解构 useI18nStore() 会丢失 computed 响应性，
// 导致运行时切换语言（setLocale）后模板文案不更新。与 CalibrationHome/ThreeHoleMain
// 等组件保持一致写法。
const { t } = storeToRefs(useI18nStore())

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
        <!-- 操作按钮统一使用小尺寸，降低卡片视觉重量 -->
        <div class="device-card-actions">
          <UiButton variant="secondary" size="sm" @click="emit('edit')">{{ t.dev_edit }}</UiButton>
          <!-- 连接按钮：connecting 分组禁用并显示 spinner -->
          <UiButton
            size="sm"
            :disabled="group === 'connecting'"
            @click="emit('connect-toggle')"
          >
            <span v-if="group === 'connecting'" class="inline-spinner" aria-hidden="true"></span>
            {{ connectLabel }}
          </UiButton>
          <UiButton v-if="showAcquireBtn()" size="sm" @click="emit('toggle-acquisition')">
            {{ acquiring ? t.dev_stop : t.dev_acquire }}
          </UiButton>
          <UiButton variant="danger" size="sm" @click="emit('remove')">{{ t.dev_delete }}</UiButton>
        </div>
      </div>
    </div>
    <div v-if="showErrorBadge()" class="device-card-error">
      <AlertCircle class="error-icon" :size="14" />
      {{ t.dev_deviceCommError }}
    </div>
  </div>
</template>

<style scoped>
.device-card {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--border-default);
  /* 紧凑密度：卡片圆角保持，但内部padding统一收紧 */
  border-radius: var(--radius-xl);
  background: var(--bg-panel);
  transition: border-color 0.2s ease;
}

.device-card:hover {
  border-color: var(--accent-success);
}

.device-card-stripe {
  position: absolute;
  inset: 0 auto 0 0;
  width: var(--space-1);
  background: var(--text-muted);
  transition: background 0.3s ease, box-shadow 0.3s ease;
}

.device-card-stripe.status-online {
  background: var(--color-success);
  box-shadow: 0 0 var(--space-2) color-mix(in srgb, var(--color-success) 50%, transparent);
}

.device-card-stripe.status-acq {
  background: var(--color-success);
  box-shadow: 0 0 var(--space-3) color-mix(in srgb, var(--color-success) 60%, transparent);
  animation: device-card-pulse 1.5s infinite;
}

.device-card-stripe.status-connecting {
  background: var(--color-warning);
  animation: device-card-pulse 0.8s infinite;
}

.device-card-body {
  display: flex;
  /* 顶部对齐：左侧名称+元信息可能多行，右侧按钮单行，居中对齐会导致视觉错位 */
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
  /* 紧凑密度：卡片 body padding 从 1rem 收紧到 12px 16px */
  padding: var(--space-3) var(--space-4);
}

.device-card-left {
  flex: 1;
  min-width: 0;
}

.device-card-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  margin-bottom: var(--space-1-5);
}

.device-card-name {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  color: var(--text-primary);
  font-size: var(--font-size-base);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.device-card-type-badge {
  flex-shrink: 0;
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--bg-panel-strong);
  color: var(--text-muted);
  font-size: var(--font-size-micro);
  font-weight: 800;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.device-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  color: var(--text-muted);
  font-size: var(--font-size-2xs);
  font-weight: 600;
}

.device-card-meta span {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
}

.device-card-meta span:first-child {
  max-width: 100%;
  overflow-wrap: anywhere;
}

.meta-icon {
  width: var(--space-3);
  height: var(--space-3);
  flex-shrink: 0;
  opacity: 0.7;
}

.device-card-right {
  display: flex;
  flex-shrink: 0;
  /* 与左侧顶部对齐，避免按钮在卡片垂直方向漂浮 */
  align-items: flex-start;
  padding-top: var(--space-0-5);
}

/* 小按钮紧凑水平排列，避免单列按钮占用过高 */
.device-card-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-content: flex-start;
  gap: var(--space-1-5);
}

.device-card-error {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  /* 与上方 body 留出间距，避免错误横幅紧贴内容 */
  margin: var(--space-2) var(--space-4) var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid color-mix(in srgb, var(--color-danger) 20%, transparent);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--color-danger) 10%, transparent);
  color: var(--color-danger);
  font-size: var(--font-size-2xs);
  font-weight: 600;
}

.error-icon {
  flex-shrink: 0;
}

@keyframes device-card-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
