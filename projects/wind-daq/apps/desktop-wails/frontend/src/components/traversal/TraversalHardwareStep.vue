<script setup lang="ts">
import type { ProbeChannelConfig, TraversalMotionAxisConfig } from '@shared/types/traversal'
import { isTraversalRequiredProbeChannel } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'

const probeChannels = defineModel<ProbeChannelConfig[]>('probeChannels', { required: true })
const motionAxes = defineModel<TraversalMotionAxisConfig[]>('motionAxes', { required: true })

const props = defineProps<{
  t: Record<string, string>
  isLoading: boolean
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()

function isRequired(c: ProbeChannelConfig) { return isTraversalRequiredProbeChannel(c.role, c.name) }

const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i}`, value: i }))
const axisOptions = ['X', 'Y', 'Z', 'U'].map(a => ({ label: `${a} 轴`, value: a }))
const mappingOptions = [{ label: 'alpha', value: 'alpha' }, { label: 'beta', value: 'beta' }]
</script>

<template>
  <div class="step-content">
    <UiPanel class="section-card">
      <div class="hw-head"><span class="hdr-enabled">{{ t.channelEnabled }}</span><span class="hdr-name">{{ t.channelProbeName }}</span><span class="hdr-device">{{ t.channelDataSource }}</span><span class="hdr-w80">{{ t.channelIndexLabel }}</span></div>
      <div v-for="ch in probeChannels" :key="ch.name" class="hw-row">
        <div class="row-check"><UiCheckbox v-model:checked="ch.enabled" :disabled="isRequired(ch)" /></div>
        <div class="row-content">
          <div class="flex items-center gap-2">
            <span class="chan-name">{{ ch.name }}</span>
            <UiStatusBadge v-if="isRequired(ch)" status="connected">Required</UiStatusBadge>
          </div>
        </div>
        <UiSelect v-model="ch.channel.deviceId" :options="deviceStore.profiles.map(d => ({ label: d.name, value: d.id }))" placeholder="选择设备" class="sel-w150" :disabled="!ch.enabled || isLoading" />
        <UiSelect :model-value="ch.channel.channelIndex != null ? String(ch.channel.channelIndex) : ''" @update:model-value="ch.channel.channelIndex = Number($event)" :options="channelIndexOptions.map(o => ({ label: o.label, value: String(o.value) }))" placeholder="Unassigned" class="sel-w80" :disabled="!ch.enabled" />
      </div>
    </UiPanel>

    <UiPanel class="section-card">
      <div class="hw-head"><span class="hdr-w50">{{ t.coordinateAxis }}</span><span class="hdr-name">{{ t.motionControllerLabel }}</span><span class="hdr-w80">{{ t.physicalAxis }}</span><span class="hdr-w90">Mapping</span></div>
      <div v-for="ax in motionAxes" :key="ax.name" class="hw-row">
        <span class="axis-name">{{ ax.name }}</span>
        <UiSelect v-model="ax.controllerId" :options="motionStore.profiles.map(c => ({ label: c.name, value: c.id }))" placeholder="选择控制器" class="sel-flex" :disabled="isLoading" />
        <UiSelect v-model="ax.axis" :options="axisOptions" class="sel-w80" />
        <UiSelect v-model="ax.angleMapping!.type" :options="mappingOptions" class="sel-w90" />
      </div>
    </UiPanel>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-3) }
.section-card { font-size:var(--text-sm) }
.hw-head { display:flex; align-items:center; gap:var(--space-2); padding-bottom:var(--space-2); border-bottom:1px solid var(--border-default) }
.hw-row { display:flex; align-items:center; gap:var(--space-2); padding:6px 0 }
.hw-row:hover { background:var(--bg-panel-strong); border-radius:var(--radius-md) }
.hdr-enabled { font-size:var(--text-xs);flex:0 0 32px;color:var(--text-muted) }
.hdr-name { font-size:var(--text-xs);flex:1;color:var(--text-muted) }
.hdr-device { font-size:var(--text-xs);width:150px;color:var(--text-muted) }
.hdr-w80 { font-size:var(--text-xs);width:80px;color:var(--text-muted) }
.hdr-w50 { font-size:var(--text-xs);width:50px;color:var(--text-muted) }
.hdr-w90 { font-size:var(--text-xs);width:90px;color:var(--text-muted) }
.row-check { flex:0 0 32px }
.row-content { flex:1;min-width:0 }
.chan-name { font-size:var(--text-sm);color:var(--text-primary) }
.axis-name { font-size:var(--text-sm);font-weight:600;width:50px;color:var(--text-primary) }
.sel-w150 { width:150px }
.sel-w80 { width:80px }
.sel-w90 { width:90px }
.sel-flex { flex:1 }
</style>