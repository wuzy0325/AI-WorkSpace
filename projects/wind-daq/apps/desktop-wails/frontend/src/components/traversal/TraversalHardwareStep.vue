<script setup lang="ts">
import type { ProbeChannelConfig, TraversalMotionAxisConfig } from '@shared/types/traversal'
import { isTraversalRequiredProbeChannel } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { NCard, NCheckbox, NSelect, NSpace, NTag, NText } from 'naive-ui'

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
    <NCard size="small" :bordered="true" class="section-card">
      <div class="hw-head"><NText depth="3" style="font-size:11px;flex:0 0 32px">{{ t.channelEnabled }}</NText><NText depth="3" style="font-size:11px;flex:1">{{ t.channelProbeName }}</NText><NText depth="3" style="font-size:11px;width:150px">{{ t.channelDataSource }}</NText><NText depth="3" style="font-size:11px;width:80px">{{ t.channelIndexLabel }}</NText></div>
      <div v-for="ch in probeChannels" :key="ch.name" class="hw-row">
        <div style="flex:0 0 32px"><NCheckbox v-model:checked="ch.enabled" :disabled="isRequired(ch)" size="small" /></div>
        <div style="flex:1;min-width:0">
          <NSpace size="small" align="center">
            <NText depth="1" style="font-size:12px;truncate">{{ ch.name }}</NText>
            <NTag v-if="isRequired(ch)" size="tiny" type="primary" :bordered="false">Required</NTag>
          </NSpace>
        </div>
        <NSelect v-model:value="ch.channel.deviceId" :options="deviceStore.profiles.map(d => ({ label: d.name, value: d.id }))" placeholder="选择设备" size="tiny" style="width:150px" :disabled="!ch.enabled || isLoading" clearable />
        <NSelect v-model:value="ch.channel.channelIndex" :options="channelIndexOptions" placeholder="Unassigned" size="tiny" style="width:80px" :disabled="!ch.enabled" />
      </div>
    </NCard>

    <NCard size="small" :bordered="true" class="section-card">
      <div class="hw-head"><NText depth="3" style="font-size:11px;width:50px">{{ t.coordinateAxis }}</NText><NText depth="3" style="font-size:11px;flex:1">{{ t.motionControllerLabel }}</NText><NText depth="3" style="font-size:11px;width:80px">{{ t.physicalAxis }}</NText><NText depth="3" style="font-size:11px;width:90px">Mapping</NText></div>
      <div v-for="ax in motionAxes" :key="ax.name" class="hw-row">
        <NText depth="1" style="font-size:12px;font-weight:600;width:50px">{{ ax.name }}</NText>
        <NSelect v-model:value="ax.controllerId" :options="motionStore.profiles.map(c => ({ label: c.name, value: c.id }))" placeholder="选择控制器" size="tiny" style="flex:1" :disabled="isLoading" clearable />
        <NSelect v-model:value="ax.axis" :options="axisOptions" size="tiny" style="width:80px" />
        <NSelect v-model:value="ax.angleMapping!.type" :options="mappingOptions" size="tiny" style="width:90px" />
      </div>
    </NCard>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:12px; }
.section-card { font-size:12px; }
.hw-head { display:flex; align-items:center; gap:8px; padding-bottom:8px; border-bottom:1px solid var(--border-default); }
.hw-row { display:flex; align-items:center; gap:8px; padding:6px 0; }
.hw-row:hover { background:var(--bg-panel-strong); border-radius:4px; }
</style>