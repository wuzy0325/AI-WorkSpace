<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { deviceApi } from '@api/deviceApi'
import type { DeviceProfile } from '@api/types'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const editing = ref<DeviceProfile | null>(null)
const draftName = ref('')
const draftType = ref<'SIMULATED' | 'DAQ_T_1603'>('SIMULATED')
const saving = ref(false)
const scanning = ref(false)
const discovered = ref<Array<{ id: string; name: string; type: string }>>([])
const tab = ref<'list' | 'editor'>('list')

watch(
  () => props.open,
  (v) => {
    if (v) {
      tab.value = 'list'
      deviceStore.refreshProfiles()
    }
  },
)

async function startCreate() {
  draftName.value = 'New Device'
  draftType.value = 'SIMULATED'
  editing.value = null
  tab.value = 'editor'
}

async function startEdit(p: DeviceProfile) {
  editing.value = p
  draftName.value = p.name
  draftType.value = p.type
  tab.value = 'editor'
}

async function saveDraft() {
  if (!draftName.value.trim()) return
  saving.value = true
  try {
    const profile: DeviceProfile = {
      id: editing.value?.id ?? `device-${Date.now()}`,
      name: draftName.value.trim(),
      type: draftType.value,
      samplingRate: 20,
      channels: Array.from({ length: draftType.value === 'DAQ_T_1603' ? 16 : 4 }, (_, i) => ({
        index: i,
        name: draftType.value === 'DAQ_T_1603' ? `TC${i + 1}` : `CH${i + 1}`,
        enabled: true,
        unit: draftType.value === 'DAQ_T_1603' ? 'degC' : 'V',
        precision: draftType.value === 'DAQ_T_1603' ? 2 : 3,
      })),
    }
    await deviceApi.upsertProfile(profile)
    await deviceStore.refreshProfiles()
    feedback.pushToast(`Profile "${draftName.value}" saved`, 'success')
    tab.value = 'list'
  } catch (err) {
    feedback.pushToast(String(err), 'error')
  } finally {
    saving.value = false
  }
}

async function removeProfile(id: string) {
  const confirmed = await feedback.confirm('确认删除此设备配置？')
  if (!confirmed) return
  try {
    await deviceApi.disconnect(id).catch(() => {})
    await deviceApi.stopAcquisition(id).catch(() => {})
    await deviceApi.upsertProfile({
      id,
      name: '',
      type: 'SIMULATED',
      samplingRate: 0,
      channels: [],
    })
    await deviceStore.refreshProfiles()
    feedback.pushToast('设备配置已删除', 'info')
  } catch (err) {
    feedback.pushToast(String(err), 'error')
  }
}

async function runScan() {
  scanning.value = true
  try {
    const results = await deviceApi.getProfiles()
    discovered.value = results.map((r: any) => ({
      id: r.id,
      name: r.name ?? r.id,
      type: r.type ?? 'SIMULATED',
    }))
    feedback.pushToast(`发现 ${discovered.value.length} 个设备`, 'info')
  } catch {
    feedback.pushToast('扫描失败', 'error')
    discovered.value = []
  } finally {
    scanning.value = false
  }
}

function addFromDiscovery(d: { id: string; name: string; type: string }) {
  startCreate()
  draftName.value = d.name
}

function close() {
  emit('update:open', false)
}

const activeProfiles = ref<DeviceProfile[]>([])
onMounted(() => {
  activeProfiles.value = deviceStore.profiles
})
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="drawer-overlay" @click.self="close">
      <div class="drawer">
        <header class="drawer__head">
          <div>
            <h2>设备管理</h2>
            <p>管理设备配置、扫描和连接</p>
          </div>
          <button class="drawer__close" @click="close">✕</button>
        </header>

        <!-- List Tab -->
        <div v-if="tab === 'list'" class="drawer__body">
          <div class="drawer__toolbar">
            <button class="btn-primary" @click="startCreate">+ 新建设备</button>
            <button class="btn-secondary" :disabled="scanning" @click="runScan">
              {{ scanning ? '扫描中...' : '扫描' }}
            </button>
          </div>

          <div v-if="discovered.length" class="drawer__section">
            <h3>发现 {{ discovered.length }} 个设备</h3>
            <div v-for="d in discovered" :key="d.id" class="drawer__row">
              <div>
                <strong>{{ d.name }}</strong>
                <small>{{ d.type }}</small>
              </div>
              <button class="btn-sm" @click="addFromDiscovery(d)">添加</button>
            </div>
          </div>

          <div class="drawer__section">
            <h3>已保存配置 ({{ deviceStore.profiles.length }})</h3>
            <div v-for="p in deviceStore.profiles" :key="p.id" class="drawer__profile">
              <div class="drawer__profile-info">
                <strong>{{ p.name }}</strong>
                <small>{{ p.type }} · {{ p.channels?.length ?? 0 }} CH</small>
              </div>
              <div class="drawer__profile-actions">
                <button class="btn-sm" @click="startEdit(p)">编辑</button>
                <button class="btn-sm btn-danger" @click="removeProfile(p.id)">删除</button>
              </div>
            </div>
            <div v-if="!deviceStore.profiles.length" class="drawer__empty">
              暂无设备配置。点击"新建设备"创建。
            </div>
          </div>
        </div>

        <!-- Editor Tab -->
        <div v-else class="drawer__body">
          <div class="drawer__section">
            <h3>{{ editing ? '编辑设备' : '新建设备' }}</h3>
            <div class="drawer__field">
              <label>设备名称</label>
              <input v-model="draftName" type="text" placeholder="输入设备名称" />
            </div>
            <div class="drawer__field">
              <label>设备类型</label>
              <select v-model="draftType">
                <option value="SIMULATED">Simulated</option>
                <option value="DAQ_T_1603">DAQ-T-1603</option>
              </select>
            </div>
            <div class="drawer__field-note">
              保存后可在仪表盘中连接设备。
            </div>
            <div class="drawer__editor-actions">
              <button class="btn-secondary" @click="tab = 'list'">取消</button>
              <button class="btn-primary" :disabled="saving || !draftName.trim()" @click="saveDraft">
                {{ saving ? '保存中...' : '保存' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.drawer-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  justify-content: flex-start;
}

.drawer {
  width: 480px;
  max-width: 90vw;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border-right: 1px solid var(--border-default);
  box-shadow: 0 25px 50px rgba(0, 0, 0, 0.3);
}

.drawer__head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: var(--space-6);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.drawer__head h2 {
  margin: 0;
  font-size: 1.25rem;
}

.drawer__head p {
  margin: 0.25rem 0 0;
  color: var(--text-muted);
  font-size: 0.8rem;
}

.drawer__close {
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  border-radius: 0.5rem;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05);
  font-size: 1rem;
}

.drawer__body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-6);
}

.drawer__toolbar {
  display: flex;
  gap: 0.75rem;
  margin-bottom: var(--space-6);
}

.drawer__section {
  margin-bottom: var(--space-6);
}

.drawer__section h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.8rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.drawer__row,
.drawer__profile {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3);
  margin-bottom: var(--space-2);
  border-radius: 0.5rem;
  background: rgba(30, 41, 59, 0.4);
}

.drawer__row strong,
.drawer__profile-info strong {
  display: block;
  font-size: 0.9rem;
}

.drawer__row small,
.drawer__profile-info small {
  display: block;
  margin-top: 0.15rem;
  color: var(--text-muted);
  font-size: 0.7rem;
}

.drawer__profile-actions {
  display: flex;
  gap: 0.5rem;
}

.drawer__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.8rem;
}

.drawer__field {
  margin-bottom: var(--space-4);
}

.drawer__field label {
  display: block;
  margin-bottom: 0.35rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
}

.drawer__field input,
.drawer__field select {
  width: 100%;
  padding: 0.6rem 0.75rem;
  border-radius: 0.5rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font: inherit;
  font-size: 0.9rem;
}

.drawer__field-note {
  margin-bottom: var(--space-4);
  color: var(--text-muted);
  font-size: 0.75rem;
}

.drawer__editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  padding-top: var(--space-4);
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.btn-primary,
.btn-secondary,
.btn-sm {
  min-height: 32px;
  padding: 0 0.9rem;
  border-radius: 0.4rem;
  font-size: 0.8rem;
  font-weight: 700;
}

.btn-primary {
  background: var(--accent-success);
  color: #f8fbff;
}

.btn-secondary {
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-secondary);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.btn-sm {
  min-height: 28px;
  padding: 0 0.6rem;
  font-size: 0.72rem;
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-secondary);
}

.btn-danger {
  background: rgba(244, 63, 94, 0.12);
  color: var(--accent-danger);
  border: 1px solid rgba(244, 63, 94, 0.25);
}

button:disabled {
  opacity: 0.55;
}
</style>
