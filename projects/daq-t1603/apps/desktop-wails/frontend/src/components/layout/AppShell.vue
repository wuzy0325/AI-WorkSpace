<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import MainTopBar from './MainTopBar.vue'
import MainBottomBar from './MainBottomBar.vue'
import DeviceSidebar from './DeviceSidebar.vue'
import DaqT1603Config from '@components/device/DaqT1603Config.vue'
import ScanResultList from '@components/device/ScanResultList.vue'
import type { ScanResult } from '@bridge/deviceBridge'
import { useDeviceStore } from '@stores/deviceStore'

const deviceStore = useDeviceStore()

const showAddDialog = ref(false)
const showConfig = ref(false)
const showScanDialog = ref(false)

const newName = ref('')
const newAddress = ref('192.168.3.101')
const newPort = ref(9000)
const addError = ref<string | null>(null)

const isAcquiring = computed(() =>
  deviceStore.profiles.some((p) => deviceStore.acquiringFor(p.id))
)

const canConfigure = computed(() => !!deviceStore.selectedId)

function toggleAcquisition() {
  if (isAcquiring.value) {
    for (const p of deviceStore.profiles) {
      if (deviceStore.acquiringFor(p.id)) {
        void deviceStore.stopAcquisition(p.id)
      }
    }
  } else {
    for (const p of deviceStore.profiles) {
      if (deviceStore.statusFor(p.id) === 'Connected') {
        void deviceStore.startAcquisition(p.id)
      }
    }
  }
}

function openAddDevice(prefill?: { address: string; port: number }) {
  newName.value = ''
  newAddress.value = prefill?.address ?? '192.168.3.101'
  newPort.value = prefill?.port ?? 9000
  addError.value = null
  showAddDialog.value = true
}

function openScanDialog() {
  deviceStore.clearScanResults()
  showScanDialog.value = true
  void deviceStore.scanDevices()
}

function addFromScanResult(result: ScanResult) {
  showScanDialog.value = false
  openAddDevice({ address: result.address, port: result.port })
}

function openConfig() {
  if (canConfigure.value) {
    showConfig.value = true
  } else {
    openAddDevice()
  }
}

function requestConfig() {
  if (canConfigure.value) {
    showConfig.value = true
  }
}

provide('shell:openConfig', requestConfig)

async function confirmAddDevice() {
  if (!newName.value.trim()) {
    addError.value = '请输入设备名称'
    return
  }
  if (!newAddress.value.trim()) {
    addError.value = '请输入 IP 地址'
    return
  }
  addError.value = null
  try {
    await deviceStore.addProfile(newName.value.trim(), newAddress.value.trim(), newPort.value)
    showAddDialog.value = false
  } catch (err) {
    addError.value = err instanceof Error ? err.message : '添加设备失败'
  }
}
</script>

<template>
  <div class="shell">
    <MainTopBar
      version="0.1.0"
      @add-device="openAddDevice"
      @open-config="openConfig"
    />
    <div class="shell__body">
      <DeviceSidebar @scan="openScanDialog" />
      <main class="shell__main">
        <slot />
      </main>
    </div>
    <MainBottomBar @toggle-acquisition="toggleAcquisition" />

    <!-- 配置模态框 -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showConfig && canConfigure" class="modal-overlay" @click.self="showConfig = false">
          <div class="modal-panel">
            <DaqT1603Config
              :device-id="deviceStore.selectedId!"
              @close="showConfig = false"
            />
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 扫描设备模态框 -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showScanDialog" class="modal-overlay" @click.self="showScanDialog = false">
          <div class="modal-panel modal-panel--narrow">
            <div class="dialog">
              <div class="dialog__header">
                <h3 class="dialog__title">扫描设备</h3>
                <p class="dialog__subtitle">局域网中发现 DAQ-T-1603 设备</p>
              </div>
              <div class="dialog__body">
                <ScanResultList
                  :results="deviceStore.scanResults"
                  :scanning="deviceStore.isScanning"
                  @add="addFromScanResult"
                />
              </div>
              <div class="dialog__actions">
                <button class="dialog__btn dialog__btn--secondary" @click="showScanDialog = false">关闭</button>
                <button
                  v-if="!deviceStore.isScanning"
                  class="dialog__btn dialog__btn--primary"
                  @click="void deviceStore.scanDevices()"
                >
                  重新扫描
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 添加设备模态框 -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showAddDialog" class="modal-overlay" @click.self="showAddDialog = false">
          <div class="modal-panel modal-panel--narrow">
            <div class="dialog">
              <div class="dialog__header">
                <h3 class="dialog__title">添加 T1603 设备</h3>
                <p class="dialog__subtitle">通过 IP 端口接入温度采集器</p>
              </div>
              <div class="dialog__body">
                <div class="dialog__field">
                  <label>设备名称</label>
                  <input v-model="newName" placeholder="例如: 温度采集器 1" autofocus @keyup.enter="confirmAddDevice" />
                </div>
                <div class="dialog__row">
                  <div class="dialog__field">
                    <label>IP 地址</label>
                    <input v-model="newAddress" placeholder="192.168.3.101" @keyup.enter="confirmAddDevice" />
                  </div>
                  <div class="dialog__field dialog__field--narrow">
                    <label>端口</label>
                    <input v-model.number="newPort" type="number" min="1" max="65535" @keyup.enter="confirmAddDevice" />
                  </div>
                </div>
                <p v-if="addError" class="dialog__error">{{ addError }}</p>
              </div>
              <div class="dialog__actions">
                <button class="dialog__btn dialog__btn--secondary" @click="showAddDialog = false">取消</button>
                <button class="dialog__btn dialog__btn--primary" @click="confirmAddDevice">添加</button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  color: var(--text-primary);
}

.shell__body {
  flex: 1;
  display: flex;
  min-height: 0;
  overflow: hidden;
}

.shell__main {
  flex: 1;
  min-width: 0;
  padding: var(--layout-content-padding);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--layout-content-gap);
}

/* 模态框过渡动画 */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-overlay);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.modal-panel {
  width: 100%;
  max-width: 38rem;
  max-height: 86vh;
  display: flex;
  flex-direction: column;
}

.modal-panel--narrow {
  max-width: 26rem;
}

/* Vue Transition 类 */
.modal-enter-active {
  transition: opacity var(--motion-base) var(--easing-standard);
}

.modal-leave-active {
  transition: opacity var(--motion-fast) var(--easing-exit);
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-active .modal-panel,
.modal-leave-active .modal-panel {
  transition: transform var(--motion-base) var(--easing-emphasis),
              opacity var(--motion-base) var(--easing-standard);
}

.modal-enter-from .modal-panel {
  opacity: 0;
  transform: scale(0.96) translateY(12px);
}

.modal-leave-to .modal-panel {
  opacity: 0;
  transform: scale(0.98) translateY(4px);
}

.dialog {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  max-height: inherit;
}

.dialog__header {
  padding: 1.25rem 1.5rem 0.85rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border-bottom: 1px solid var(--divider-color);
}

.dialog__title {
  font-size: var(--font-size-md);
  font-weight: 800;
  color: var(--text-primary);
}

.dialog__subtitle {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.dialog__body {
  padding: 1.25rem 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.dialog__row {
  display: flex;
  gap: 0.75rem;
}

.dialog__field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  flex: 1;
}

.dialog__field--narrow {
  max-width: 7rem;
}

.dialog__field label {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-secondary);
}

.dialog__error {
  font-size: var(--font-size-xs);
  color: var(--danger);
  margin: 0;
  padding: 0.5rem 0.65rem;
  background: var(--danger-muted);
  border-radius: var(--radius-md);
  border: 1px solid var(--danger-border);
}

.dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  padding: 0.85rem 1.5rem;
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--divider-color);
}

.dialog__btn {
  padding: 0.5rem 1rem;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 700;
  transition: all var(--motion-fast) var(--easing-standard);
}

.dialog__btn--secondary {
  background: var(--btn-bg);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
}

.dialog__btn--secondary:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
}

.dialog__btn--primary {
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  color: #ffffff;
  border: 1px solid var(--accent-border);
  box-shadow: 0 4px 12px var(--accent-glow);
}

.dialog__btn--primary:hover {
  box-shadow: 0 6px 16px var(--accent-glow);
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .modal-enter-active,
  .modal-leave-active,
  .modal-enter-active .modal-panel,
  .modal-leave-active .modal-panel {
    transition: none;
  }
}
</style>
