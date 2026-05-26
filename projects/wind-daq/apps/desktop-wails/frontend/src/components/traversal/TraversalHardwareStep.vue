<script setup lang="ts">
import type { ProbeChannelConfig, TraversalMotionAxisConfig } from '@shared/types/traversal'
import { isTraversalRequiredProbeChannel } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'

const probeChannels = defineModel<ProbeChannelConfig[]>('probeChannels', { required: true })
const motionAxes = defineModel<TraversalMotionAxisConfig[]>('motionAxes', { required: true })

const props = defineProps<{
  t: Record<string, string>
  isLoading: boolean
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()

function isRequiredProbeChannel(channel: ProbeChannelConfig): boolean {
  return isTraversalRequiredProbeChannel(channel.role, channel.name)
}
</script>

<template>
  <div class="space-y-4">
    <section class="ui-panel-surface p-4 p-3">
      <div class="flex items-center gap-2 mb-3 pb-2 border-b border-[color:var(--border-default)]">
        <div class="w-8 text-xs text-[color:var(--text-muted)]">{{ t.channelEnabled }}</div>
        <div class="flex-1 text-xs text-[color:var(--text-muted)]">{{ t.channelProbeName }}</div>
        <div class="w-40 text-xs text-[color:var(--text-muted)]">{{ t.channelDataSource }}</div>
        <div class="w-20 text-xs text-[color:var(--text-muted)]">{{ t.channelIndexLabel }}</div>
      </div>
      <div class="space-y-1">
        <div v-for="channel in probeChannels" :key="channel.name" data-test="traversal-probe-row" class="flex items-center gap-2 py-1.5 hover:bg-[color:var(--bg-panel-strong)] rounded-[var(--radius-sm)]">
          <input v-model="channel.enabled" type="checkbox" :disabled="isRequiredProbeChannel(channel)" class="h-4 w-4 rounded border-[color:var(--border-default)] text-[color:var(--accent-primary)] focus:ring-[color:var(--focus-ring-soft)]" />
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span data-test="traversal-probe-label" class="text-sm text-[color:var(--text-primary)] truncate">{{ channel.name }}</span>
              <span v-if="isRequiredProbeChannel(channel)" class="rounded-full bg-[color:var(--accent-primary)]/12 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-[color:var(--accent-primary)]">Required</span>
            </div>
          </div>
          <select v-model="channel.channel.deviceId" :disabled="!channel.enabled || props.isLoading" class="w-40 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-xs text-[color:var(--text-primary)]">
            <option value="">{{ t.selectDevice }}</option>
            <option v-for="device in deviceStore.profiles" :key="device.id" :value="device.id">{{ device.name }}</option>
          </select>
          <select v-model="channel.channel.channelIndex" :disabled="!channel.enabled" class="w-20 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-xs text-[color:var(--text-primary)]">
            <option :value="-1">Unassigned</option>
            <option v-for="index in 18" :key="index - 1" :value="index - 1">CH{{ index - 1 }}</option>
          </select>
        </div>
      </div>
    </section>

    <section class="ui-panel-surface p-4 p-3">
      <div class="flex items-center gap-2 mb-3 pb-2 border-b border-[color:var(--border-default)]">
        <div class="w-12 text-xs text-[color:var(--text-muted)]">{{ t.coordinateAxis }}</div>
        <div class="flex-1 text-xs text-[color:var(--text-muted)]">{{ t.motionControllerLabel }}</div>
        <div class="w-20 text-xs text-[color:var(--text-muted)]">{{ t.physicalAxis }}</div>
        <div class="w-24 text-xs text-[color:var(--text-muted)]">Mapping</div>
      </div>
      <div class="space-y-1">
        <div v-for="axis in motionAxes" :key="axis.name" class="flex items-center gap-2 py-1.5 hover:bg-[color:var(--bg-panel-strong)] rounded-[var(--radius-sm)]">
          <span class="w-12 text-sm font-semibold text-[color:var(--text-primary)]">{{ axis.name }}</span>
          <select v-model="axis.controllerId" :disabled="props.isLoading" class="flex-1 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-xs text-[color:var(--text-primary)]">
            <option value="">{{ t.selectController }}</option>
            <option v-for="controller in motionStore.profiles" :key="controller.id" :value="controller.id">{{ controller.name }}</option>
          </select>
          <select v-model="axis.axis" class="w-20 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-xs text-[color:var(--text-primary)]">
            <option v-for="axisName in ['X', 'Y', 'Z', 'U']" :key="axisName" :value="axisName">{{ axisName }}</option>
          </select>
          <select v-model="axis.angleMapping!.type" class="w-24 rounded-[var(--radius-sm)] border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-2 py-1 text-xs text-[color:var(--text-primary)]">
            <option value="alpha">alpha</option>
            <option value="beta">beta</option>
          </select>
        </div>
      </div>
    </section>
  </div>
</template>

