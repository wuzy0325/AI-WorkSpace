<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { deviceApi } from '@api/deviceApi'
import type { DeviceProfile, ScanResult } from '@api/types'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const scanning = ref(false)
const discovered = ref<ScanResult[]>([])
const selectedIds = ref<string[]>([])

const editorOpen = ref(false)
const editingId = ref<string | null>(null)
const draftName = ref('')
const draftType = ref<DeviceProfile['type']>('SIMULATED')
const saving = ref(false)

function openCreate() {
  editingId.value = null
  draftName.value = ''
  draftType.value = 'SIMULATED'
  editorOpen.value = true
}

function openEdit(p: DeviceProfile) {
  editingId.value = p.id
  draftName.value = p.name
  draftType.value = p.type
  editorOpen.value = true
}

async function saveDraft() {
  if (!draftName.value.trim()) return
  saving.value = true
  try {
    const profile: DeviceProfile = {
      id: editingId.value ?? `device-${Date.now()}`,
      name: draftName.value.trim(),
      type: draftType.value,
      samplingRate: 20,
      channels: Array.from({ length: draftType.value === 'DAQ-T-1603' ? 16 : 4 }, (_, i) => ({
        index: i,
        name: draftType.value === 'DAQ-T-1603' ? `TC${i + 1}` : `CH${i + 1}`,
        enabled: true,
        unit: draftType.value === 'DAQ-T-1603' ? 'degC' : 'V',
        precision: draftType.value === 'DAQ-T-1603' ? 2 : 3,
      })),
    }
    await deviceApi.upsertProfile(profile)
    await deviceStore.refreshProfiles()
    feedback.pushToast(`设备 "${draftName.value}" 已保存`, 'success')
    editorOpen.value = false
  } catch (err) {
    feedback.pushToast(String(err), 'error')
  } finally {
    saving.value = false
  }
}

function cancelEditor() {
  editorOpen.value = false
}

const selectedCount = computed(() => selectedIds.value.length)

function toggleSelected(id: string) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}

function isSelected(id: string) {
  return selectedIds.value.includes(id)
}

function clearSelection() {
  selectedIds.value = []
}

watch(() => props.open, (v) => {
  if (v) {
    deviceStore.refreshProfiles()
    clearSelection()
  }
})

async function runScan() {
  scanning.value = true
  try {
    const results = await deviceApi.scanDevices()
    discovered.value = results
    if (discovered.value.length) feedback.pushToast(`发现 ${discovered.value.length} 个设备`, 'info')
    else feedback.pushToast('未发现新设备', 'info')
  } catch (err) {
    discovered.value = []
    console.error('[DeviceScan] 扫描失败:', err)
    feedback.pushToast(`扫描失败: ${err instanceof Error ? err.message : String(err)}`, 'error')
  } finally {
    scanning.value = false
  }
}

function clearDiscovered() {
  discovered.value = []
}

async function addDiscoveredDevice(d: ScanResult) {
  const existing = deviceStore.profiles.find((p) => p.id === d.id)
  if (existing) {
    feedback.pushToast(`设备 "${d.name}" 已存在于配置中`, 'warning')
    return
  }
  const profileType = mapScanTypeToProfileType(d.type)
  const profile: DeviceProfile = {
    id: d.id,
    name: d.name,
    type: profileType,
    samplingRate: 20,
    channels: Array.from({ length: profileType === 'DAQ-T-1603' ? 16 : 4 }, (_, i) => ({
      index: i,
      name: profileType === 'DAQ-T-1603' ? `TC${i + 1}` : `CH${i + 1}`,
      enabled: true,
      unit: profileType === 'DAQ-T-1603' ? 'degC' : 'V',
      precision: profileType === 'DAQ-T-1603' ? 2 : 3,
    })),
  }
  try {
    await deviceApi.upsertProfile(profile)
    await deviceStore.refreshProfiles()
    feedback.pushToast(`设备 "${d.name}" 已添加到配置`, 'success')
  } catch (e) {
    feedback.pushToast(String(e), 'error')
  }
}

function mapScanTypeToProfileType(scanType: string): DeviceProfile['type'] {
  switch (scanType) {
    case 'DAQ-P-1604': return 'DAQ-P-1604'
    case 'DAQ-T-1603': return 'DAQ-T-1603'
    case 'DAQ-P-1064Pre': return 'DAQ-P-1064Pre'
    default: return 'SIMULATED'
  }
}

async function connectToggle(p: DeviceProfile) {
  const st = deviceStore.statusFor(p.id)
  const acquiring = deviceStore.acquiringFor(p.id)
  if (acquiring || st === 'Connected') {
    try {
      await deviceApi.disconnect(p.id)
      await deviceStore.refreshStatusFor(p.id)
    } catch (e) {
      feedback.pushToast(String(e), 'error')
    }
  } else {
    try {
      await deviceApi.connect(p.id)
      await deviceStore.refreshStatusFor(p.id)
    } catch (e) {
      feedback.pushToast(String(e), 'error')
    }
  }
}

async function toggleAcquisition(p: DeviceProfile) {
  const acquiring = deviceStore.acquiringFor(p.id)
  try {
    if (acquiring) {
      await deviceApi.stopAcquisition(p.id)
    } else {
      await deviceApi.startAcquisition(p.id)
    }
    await deviceStore.refreshStatusFor(p.id)
  } catch (e) {
    feedback.pushToast(String(e), 'error')
  }
}

async function removeProfile(p: DeviceProfile) {
  const ok = await feedback.confirm('确认删除此设备配置？')
  if (!ok) return
  try {
    await deviceApi.disconnect(p.id).catch(() => {})
    await deviceApi.stopAcquisition(p.id).catch(() => {})
    const empty: DeviceProfile = { id: p.id, name: '', type: 'SIMULATED', samplingRate: 0, channels: [] }
    await deviceApi.upsertProfile(empty)
    await deviceStore.refreshProfiles()
    feedback.pushToast('设备配置已删除', 'info')
  } catch (e) {
    feedback.pushToast(String(e), 'error')
  }
}

function close() {
  emit('update:open', false)
}

function statusClass(p: DeviceProfile) {
  if (deviceStore.acquiringFor(p.id)) return 'status-acq'
  const s = deviceStore.statusFor(p.id)
  if (s === 'Connected') return 'status-online'
  if (s === 'Connecting') return 'status-connecting'
  return 'status-offline'
}

function statusLabel(p: DeviceProfile) {
  if (deviceStore.acquiringFor(p.id)) return '采集中'
  const s = deviceStore.statusFor(p.id)
  if (s === 'Connected') return '已连接'
  if (s === 'Connecting') return '连接中'
  return '已断开'
}

function connectLabel(p: DeviceProfile) {
  const acquiring = deviceStore.acquiringFor(p.id)
  const st = deviceStore.statusFor(p.id)
  if (acquiring || st === 'Connected') return '断开'
  return '连接'
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="drawer-mask" @click.self="close">
      <div class="drawer-shell">
        <header class="drawer-header">
          <div>
            <h2 class="drawer-title">设备管理</h2>
            <p class="drawer-subtitle">管理设备配置、扫描和连接</p>
          </div>
          <button class="drawer-close" @click="close">✕</button>
        </header>

        <div class="drawer-toolbar">
          <button class="btn btn-primary" @click="openCreate">
            <span class="btn-icon">+</span> 新建设备
          </button>
          <button class="btn btn-second" :disabled="scanning" @click="runScan">
            <span class="btn-icon" :class="{ spin: scanning }">⟳</span>
            {{ scanning ? '扫描中...' : '扫描' }}
          </button>
          <div class="drawer-total">
            设备: {{ deviceStore.profiles.length }}
          </div>
        </div>

        <div v-if="discovered.length" class="drawer-discovered">
          <div class="drawer-discovered-head">
            <span class="drawer-discovered-label">发现的设备</span>
            <div class="drawer-discovered-actions">
              <span class="discovered-pulse" />
              <button class="btn btn-xs btn-ghost" @click="clearDiscovered">✕</button>
            </div>
          </div>
          <div class="drawer-discovered-list">
            <div v-for="d in discovered" :key="d.id" class="discovered-card">
              <div class="discovered-card-icon">{{ d.type === 'DAQ-T-1603' ? 'T' : d.type === 'DAQ-P-1604' ? 'P' : d.type === 'DAQ-P-1064Pre' ? 'S' : 'D' }}</div>
              <div class="discovered-card-info">
                <div class="discovered-card-name">{{ d.name }}</div>
                <div class="discovered-card-type">
                  {{ d.type }}
                  <span v-if="d.address" class="discovered-card-addr"> · {{ d.address }}<template v-if="d.port">:{{ d.port }}</template></span>
                  <span v-if="d.macAddress" class="discovered-card-addr"> · MAC: {{ d.macAddress }}</span>
                </div>
              </div>
              <button class="btn btn-xs btn-green" @click="addDiscoveredDevice(d)">添加</button>
            </div>
          </div>
        </div>

        <main class="drawer-list">
          <div v-if="!deviceStore.profiles.length" class="drawer-empty">
            暂无设备配置。点击"新建设备"创建。
          </div>

          <div v-for="p in deviceStore.profiles" :key="p.id" class="device-card" :class="[statusClass(p)]">
            <div class="device-card-stripe" :class="[statusClass(p)]" />

            <div class="device-card-body">
              <div class="device-card-left">
                <div class="device-card-row">
                  <input type="checkbox" class="device-checkbox"
                    :checked="isSelected(p.id)"
                    @change="toggleSelected(p.id)" />
                  <h3 class="device-card-name">{{ p.name }}</h3>
                  <span class="device-card-type-badge">{{ p.type }}</span>
                </div>
                <div class="device-card-meta">
                  <span>{{ p.type }}</span>
                  <span>{{ p.samplingRate ?? 20 }}Hz</span>
                  <span>{{ p.channels?.length ?? 0 }} 通道</span>
                </div>
              </div>

              <div class="device-card-right">
                <button class="btn btn-sm" @click="openEdit(p)">编辑</button>
                <button class="btn btn-sm" :class="connectLabel(p) === '断开' ? 'btn-danger' : 'btn-green'" @click="connectToggle(p)">
                  {{ connectLabel(p) }}
                </button>
                <button v-if="deviceStore.statusFor(p.id) === 'Connected'" class="btn btn-sm" :class="deviceStore.acquiringFor(p.id) ? 'btn-warn' : 'btn-green'" @click="toggleAcquisition(p)">
                  {{ deviceStore.acquiringFor(p.id) ? '停止' : '采集' }}
                </button>
                <button class="btn btn-sm btn-danger ghost" @click="removeProfile(p)">删除</button>
              </div>
            </div>

            <div v-if="deviceStore.statusFor(p.id) === 'Error'" class="device-card-error">
              ⚠ 设备通信错误
            </div>
          </div>
        </main>

        <div v-if="selectedIds.length" class="drawer-bulk">
          <span>已选 <strong>{{ selectedCount }}</strong></span>
          <button class="btn btn-xs" @click="clearSelection">清除</button>
        </div>
      </div>

      <!-- editor modal -->
      <div v-if="editorOpen" class="editor-mask" @click.self="cancelEditor">
        <div class="editor-modal">
          <header class="editor-header">
            <div>
              <h3 class="editor-title">{{ editingId ? '编辑设备' : '新建设备' }}</h3>
              <p class="editor-subtitle">{{ editingId ? '修改设备名称和类型' : '配置新设备的基本信息' }}</p>
            </div>
            <button class="drawer-close" @click="cancelEditor">✕</button>
          </header>
          <div class="editor-body">
            <div class="editor-field">
              <label class="editor-label">设备名称</label>
              <input v-model="draftName" class="editor-input" type="text" placeholder="输入设备名称" />
            </div>
            <div class="editor-field">
              <label class="editor-label">设备类型</label>
              <select v-model="draftType" class="editor-input">
                <option value="SIMULATED">Simulated</option>
                <option value="DAQ-P-1604">DAQ-P-1604</option>
                <option value="DAQ-T-1603">DAQ-T-1603</option>
                <option value="DAQ-P-1064Pre">DAQ-P-1064Pre</option>
              </select>
            </div>
            <p class="editor-note">保存后可在仪表盘中连接设备并开始采集。</p>
          </div>
          <div class="editor-footer">
            <button class="btn btn-second" @click="cancelEditor">取消</button>
            <button class="btn btn-primary" :disabled="saving || !draftName.trim()" @click="saveDraft">
              {{ saving ? '保存中...' : '保存' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.drawer-mask {
  position: fixed; inset: 0; z-index: 100;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex; justify-content: flex-end;
}

.drawer-shell {
  width: 560px; max-width: 95vw; height: 100vh;
  display: flex; flex-direction: column;
  background: var(--bg-panel);
  border-left: 1px solid var(--border-default);
  box-shadow: 0 25px 50px rgba(0, 0, 0, 0.3);
}

.drawer-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}

.drawer-title {
  margin: 0; font-size: 1rem; font-weight: 800; color: var(--text-primary); letter-spacing: -0.02em;
}

.drawer-subtitle {
  margin: 0.25rem 0 0; font-size: 0.75rem; color: var(--text-muted); font-weight: 600;
}

.drawer-close {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 0.5rem; color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05); border: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 0.875rem; transition: all 0.2s;
}
.drawer-close:hover { color: var(--accent-danger); border-color: var(--accent-danger); }

.drawer-toolbar {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.drawer-total {
  margin-left: auto;
  padding: 0.375rem 0.75rem; border-radius: 999px;
  background: rgba(100, 116, 139, 0.1);
  font-size: 0.625rem; font-weight: 800; letter-spacing: 0.1em;
  color: var(--text-muted); text-transform: uppercase;
}

/* Buttons */
.btn {
  display: inline-flex; align-items: center; gap: 0.375rem;
  padding: 0.5rem 1rem; border-radius: 0.5rem;
  font-size: 0.75rem; font-weight: 700;
  transition: all 0.2s; cursor: pointer; border: 1px solid transparent;
}
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: #10b981; color: white; box-shadow: 0 4px 12px rgba(16,185,129,0.3); }
.btn-primary:hover { background: #059669; }
.btn-second { background: rgba(255,255,255,0.05); color: var(--text-secondary); border-color: rgba(255,255,255,0.1); }
.btn-second:hover { color: #10b981; border-color: #10b981; }
.btn-green { background: #10b981; color: white; }
.btn-green:hover { background: #059669; }
.btn-danger { background: rgba(244,63,94,0.1); color: #f43f5e; border-color: rgba(244,63,94,0.2); }
.btn-danger:hover { background: rgba(244,63,94,0.2); }
.btn-warn { background: rgba(245,158,11,0.1); color: #f59e0b; border-color: rgba(245,158,11,0.2); }
.btn-sm { padding: 0.375rem 0.75rem; font-size: 0.7rem; white-space: nowrap; }
.btn-xs { padding: 0.25rem 0.5rem; font-size: 0.625rem; }
.btn-icon { font-size: 1rem; line-height: 1; }
.btn-ghost { background: transparent; color: var(--text-muted); border: none; }
.spin { display: inline-block; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* Discovered */
.drawer-discovered {
  border-bottom: 1px solid var(--border-default);
  padding: 1rem 1.5rem;
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
  flex-shrink: 0;
}

.drawer-discovered-head {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 0.75rem;
}

.drawer-discovered-label {
  font-size: 0.625rem; font-weight: 800; letter-spacing: 0.15em;
  text-transform: uppercase; color: var(--text-muted);
}

.drawer-discovered-actions {
  display: flex; align-items: center; gap: 0.5rem;
}

.discovered-pulse {
  width: 6px; height: 6px; border-radius: 50%;
  background: #3b82f6; animation: pulse 1.5s infinite;
}

.drawer-discovered-list {
  display: flex; flex-direction: column; gap: 0.5rem;
  max-height: 30vh; overflow-y: auto;
}

.discovered-card {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem; border-radius: 0.75rem;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
}

.discovered-card-icon {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 50%; background: rgba(59,130,246,0.1); color: #3b82f6;
  font-size: 0.75rem; font-weight: 800; flex-shrink: 0;
}

.discovered-card-name {
  font-size: 0.8rem; font-weight: 700; color: var(--text-primary);
}

.discovered-card-type {
  font-size: 0.65rem; font-weight: 600; color: var(--text-muted); margin-top: 0.125rem;
}

.discovered-card-addr {
  color: var(--text-muted); opacity: 0.7;
}

.discovered-card .btn-green {
  margin-left: auto; flex-shrink: 0;
}

/* Device List */
.drawer-list {
  flex: 1; overflow-y: auto; padding: 1rem 1.5rem;
  display: flex; flex-direction: column; gap: 0.75rem;
}

.drawer-empty {
  padding: 2rem 1rem; text-align: center; color: var(--text-muted); font-size: 0.8rem;
}

.device-card {
  position: relative; overflow: hidden;
  border-radius: 0.75rem; border: 1px solid var(--border-default);
  background: var(--bg-panel); transition: all 0.2s;
}
.device-card:hover { border-color: var(--accent-success); }

.device-card-stripe {
  position: absolute; left: 0; top: 0; bottom: 0; width: 4px;
  background: #64748b; transition: all 0.3s;
}
.device-card-stripe.status-online { background: #10b981; box-shadow: 0 0 8px rgba(16,185,129,0.5); }
.device-card-stripe.status-acq { background: #10b981; box-shadow: 0 0 12px rgba(16,185,129,0.6); animation: pulse 1.5s infinite; }
.device-card-stripe.status-connecting { background: #f59e0b; animation: pulse 0.8s infinite; }

.device-card-body {
  padding: 1rem; display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem;
}

.device-card-left { min-width: 0; flex: 1; }

.device-card-row {
  display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem;
}

.device-checkbox {
  width: 14px; height: 14px; border-radius: 3px; flex-shrink: 0;
  accent-color: #3b82f6;
}

.device-card-name {
  margin: 0; font-size: 0.9rem; font-weight: 700; color: var(--text-primary);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}

.device-card-type-badge {
  flex-shrink: 0; padding: 0.125rem 0.5rem; border-radius: 0.25rem;
  background: var(--bg-panel-strong); font-size: 0.6rem; font-weight: 800;
  letter-spacing: 0.05em; color: var(--text-muted); text-transform: uppercase;
}

.device-card-meta {
  display: flex; flex-wrap: wrap; gap: 1rem;
  font-size: 0.65rem; font-weight: 600; color: var(--text-muted);
}
.device-card-meta span { display: inline-flex; align-items: center; gap: 0.25rem; }

.device-card-right {
  display: flex; flex-direction: column; gap: 0.375rem; flex-shrink: 0;
}

.device-card-error {
  margin: 0 1rem 0.75rem; padding: 0.5rem 0.75rem; border-radius: 0.375rem;
  background: rgba(244,63,94,0.1); border: 1px solid rgba(244,63,94,0.2);
  font-size: 0.65rem; font-weight: 600; color: #f43f5e;
}

.drawer-bulk {
  flex-shrink: 0;
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem 1.5rem; border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  font-size: 0.75rem; color: var(--text-secondary);
}

.device-card-right .btn.ghost {
  background: transparent; color: var(--text-muted); border: 1px dashed var(--border-default);
}
.device-card-right .btn.ghost:hover { color: #f43f5e; border-color: #f43f5e; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* Editor Modal */
.editor-mask {
  position: fixed; inset: 0; z-index: 110;
  background: rgba(0, 0, 0, 0.7);
  display: flex; align-items: center; justify-content: center;
  padding: 1rem;
}

.editor-modal {
  width: 480px; max-width: 98vw;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 1rem;
  box-shadow: 0 32px 64px -12px rgba(0, 0, 0, 0.5);
  display: flex; flex-direction: column;
  overflow: hidden;
}

.editor-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
}

.editor-title {
  margin: 0; font-size: 1rem; font-weight: 800; color: var(--text-primary);
}

.editor-subtitle {
  margin: 0.25rem 0 0; font-size: 0.75rem; color: var(--text-muted);
}

.editor-body {
  padding: 1.5rem;
}

.editor-field {
  margin-bottom: 1.25rem;
}

.editor-label {
  display: block; margin-bottom: 0.375rem;
  font-size: 0.75rem; font-weight: 700; color: var(--text-muted);
  letter-spacing: 0.05em;
}

.editor-input {
  width: 100%; padding: 0.625rem 0.75rem;
  border-radius: 0.5rem; border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2); color: var(--text-primary);
  font: inherit; font-size: 0.9rem;
}

:root[data-theme='light'] .editor-input {
  background: rgba(255, 255, 255, 0.8);
}

.editor-note {
  font-size: 0.75rem; color: var(--text-muted);
}

.editor-footer {
  display: flex; justify-content: flex-end; gap: 0.75rem;
  padding: 1rem 1.5rem;
  border-top: 1px solid var(--border-default);
}
</style>
