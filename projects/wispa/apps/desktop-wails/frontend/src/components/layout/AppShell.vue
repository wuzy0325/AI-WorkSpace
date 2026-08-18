<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import MainTopBar from './MainTopBar.vue'
import MainBottomBar from './MainBottomBar.vue'
import LogPanel from './LogPanel.vue'
import DeviceSidebar from './DeviceSidebar.vue'
import DaqP1604Config from '@components/device/DaqP1604Config.vue'
import ScanResultList, { type ScanSelectionItem } from '@components/device/ScanResultList.vue'
import { type ScannedDeviceInput } from '@stores/deviceStoreHelpers'
import { useDeviceStore } from '@stores/deviceStore'
import { useLogStore } from '@stores/logStore'
import { useI18nStore } from '@stores/i18nStore'
import pkg from '../../../package.json'
const appVersion = pkg.version

const deviceStore = useDeviceStore()
const logStore = useLogStore()
const i18n = useI18nStore()

const showAddDialog = ref(false)
const showConfig = ref(false)
const showScanDialog = ref(false)

const newName = ref('')
const newAddress = ref('192.168.3.101')
const newLocalAddress = ref('')
const newPort = ref(9000)
const addError = ref<string | null>(null)

/** 扫描弹窗中当前勾选项（由 ScanResultList v-model 维护） */
const scanSelection = ref<ScanSelectionItem[]>([])
/** 顶部批量默认开关：新增设备默认启用自动连接（Q2=A：默认开启） */
const defaultAutoConnectOnAdd = ref(true)
/** 批量添加操作锁：防止用户在提交过程中重复点击 */
const isAddingScanned = ref(false)

/** 操作锁：防止快速重复点击导致并发请求 */
const isToggling = ref(false)
const isZeroing = ref(false)

const isAcquiring = computed(() =>
  deviceStore.profiles.some((p) => deviceStore.acquiringFor(p.id))
)

const canConfigure = computed(() => !!deviceStore.selectedId)

/**
 * 切换采集状态（带操作锁保护）
 *
 * 优化点：
 * 1. 使用 isToggling 锁防止并发执行
 * 2. 在操作期间禁止重复触发
 */
async function toggleAcquisition() {
  // 操作锁检查：如果正在进行采集切换操作，忽略后续点击
  if (isToggling.value || isZeroing.value) {
    return
  }

  isToggling.value = true
  try {
    if (isAcquiring.value) {
      // 停止所有正在采集的设备
      const acquiringIds = deviceStore.profiles
        .filter((p) => deviceStore.acquiringFor(p.id))
        .map((p) => p.id)
      await Promise.allSettled(acquiringIds.map((id) => deviceStore.stopAcquisition(id)))
    } else {
      // 仅对已连接的设备启动采集（排除正在转换状态的设备）
      const connectedIds = deviceStore.profiles
        .filter((p) => {
          const status = deviceStore.statusFor(p.id)
          // ✅ 修复：只允许 Connected 状态的设备启动采集，排除 Starting/Stopping
          return status === 'Connected'
        })
        .map((p) => p.id)
      await Promise.allSettled(connectedIds.map((id) => deviceStore.startAcquisition(id)))
    }
  } finally {
    // 无论成功失败，都释放操作锁
    isToggling.value = false
  }
}

async function zeroCalibration() {
  const id = deviceStore.selectedId
  if (!id || isZeroing.value || isToggling.value) return

  const status = deviceStore.statusFor(id)
  if (status !== 'Connected' && status !== 'Acquiring') return

  isZeroing.value = true
  try {
    await deviceStore.zeroCalibration(id)
  } finally {
    isZeroing.value = false
  }
}

provide('shell:openConfig', requestConfig)
provide('shell:zeroCalibration', zeroCalibration)
provide('shell:zeroing', isZeroing)

function openAddDevice(prefill?: { address: string; port: number }) {
  newName.value = ''
  newAddress.value = prefill?.address ?? '192.168.3.101'
  newLocalAddress.value = ''
  newPort.value = prefill?.port ?? 9000
  addError.value = null
  showAddDialog.value = true
}

function openScanDialog() {
  deviceStore.clearScanResults()
  scanSelection.value = []
  showScanDialog.value = true
  void deviceStore.scanDevices()
}

function closeScanDialog() {
  deviceStore.cancelScan()
  showScanDialog.value = false
}

function openConfig() {
  // 防御性检查：按钮在无设备时已 disabled，理论上不会进入此分支；
  // 仍保留判断以防外部直接调用（例如 MonitorView 通过 inject 触发）。
  if (canConfigure.value) {
    showConfig.value = true
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
    addError.value = i18n.t('dialog.inputDeviceName')
    return
  }
  if (!newAddress.value.trim()) {
    addError.value = i18n.t('dialog.inputIpAddress')
    return
  }
  addError.value = null
  try {
    await deviceStore.addProfile(newName.value.trim(), newAddress.value.trim(), newPort.value, newLocalAddress.value.trim())
    showAddDialog.value = false
  } catch (err) {
    addError.value = err instanceof Error ? err.message : i18n.t('dialog.addDeviceFailed')
  }
}

/**
 * 批量添加扫描弹窗中已勾选的设备。
 *
 * 交互流程（Q1=B）：
 * 1. 校验非空 + 操作锁
 * 2. 委托 deviceStore.addScannedProfiles 完成去重/命名/落库
 * 3. 成功后关闭弹窗、清空选择状态
 * 4. 失败/跳过条目通过 logStore 汇报
 */
async function confirmAddScanned() {
  if (scanSelection.value.length === 0 || isAddingScanned.value) return

  isAddingScanned.value = true
  try {
    // 把扫描弹窗的选择项映射为 ScannedDeviceInput，其中 name 作为 overrideName 传给规划逻辑。
    // 修复点：此前仅传 ScanSelectionItem，planScannedAdditions 读取的是 overrideName，
    // 用户在内联输入框改的名字从未生效（永远落到默认名）。此处显式映射后改名真正落地。
    const inputs: ScannedDeviceInput[] = scanSelection.value.map((s) => ({
      address: s.address,
      port: s.port,
      macAddress: s.macAddress,
      serialNumber: s.serialNumber,
      overrideName: s.name,
    }))
    const result = await deviceStore.addScannedProfiles(inputs, {
      defaultAutoConnect: defaultAutoConnectOnAdd.value,
    })

    // 汇报被去重跳过的条目（真实调用发生前端极少触发，通常已被 checkbox 置灰拦截）
    if (result.skipped.length > 0) {
      logStore.warn(
        'device',
        i18n.t('logMessage.scanSkipped', {
          count: result.skipped.length,
          details: result.skipped.map((s) => `${s.address}:${s.port}`).join(', '),
        }),
      )
    }
    // 汇报单条落库失败
    if (result.failed.length > 0) {
      for (const f of result.failed) {
        logStore.error('device', i18n.t('logMessage.scanFailed', { name: f.input.name, error: f.error }))
      }
    }
    if (result.added.length > 0) {
      logStore.info('device', i18n.t('logMessage.scanAdded', { count: result.added.length }))
    }

    // 立即触发新加设备的自动连接（不用等应用重启）：
    // - 只连本次成功新加且 autoConnect=true 的设备
    // - 用 Promise.allSettled 并发，某台连接失败不影响其它
    // - 失败信息由 deviceStore.connect 的 errorMap + 状态栏显示；这里另记日志便于回溯
    if (result.added.length > 0 && defaultAutoConnectOnAdd.value) {
      const targets = result.added.filter((p) => p.p1604Config?.autoConnect)
      if (targets.length > 0) {
        const settled = await Promise.allSettled(
          targets.map((p) => deviceStore.connect(p.id)),
        )
        settled.forEach((res, i) => {
          if (res.status === 'rejected') {
            const target = targets[i]!
            const reason = res.reason instanceof Error ? res.reason.message : String(res.reason)
            logStore.warn('device', i18n.t('logMessage.autoConnectDeviceFailed', { name: target.name, reason }))
          }
        })
      }
    }

    // 关弹窗策略（Q1=B）：仅在有条目落库成功时关闭并清理已勾选状态；
    // 若 added=0（全部失败或全部被跳过），保持弹窗打开，让用户看到 checkbox 置灰
    // 或日志面板的错误反馈，避免弹窗一闪而过看不到问题。
    if (result.added.length > 0) {
      showScanDialog.value = false
      scanSelection.value = []
    } else if (result.failed.length > 0) {
      // 全部失败：清空选择让用户重新勾选（避免旧勾选状态遮挡视线）
      scanSelection.value = []
    }
  } finally {
    isAddingScanned.value = false
  }
}
</script>

<template>
  <div class="shell">
    <MainTopBar
      :version="appVersion"
      :is-toggling="isToggling"
      :is-zeroing="isZeroing"
      @toggle-acquisition="toggleAcquisition"
    />
    <div class="shell__body">
      <DeviceSidebar @scan="openScanDialog" @add-device="openAddDevice" />
      <main class="shell__main">
        <slot />
      </main>
      <LogPanel />
    </div>
    <MainBottomBar />

    <!-- 配置模态框 -->
    <!--
      v-if + v-show 组合（CFG-017/CFG-020 草稿保留修复）：
        - v-if="canConfigure"：仅当存在选中设备时挂载组件，避免 selectedId 为空时
          传入 undefined 触发 DaqP1604Config 内部 watch 异常。
        - v-show="showConfig"：关闭面板时仅设置 display:none，组件不销毁，
          DaqP1604Config 内部的 channelNames/channelEnabled/channelPrecisions 等
          本地 ref 草稿得以保留，重新打开面板时草稿仍在原处。
      草稿生命周期：切换设备时 :device-id 变化 → profile 变化 → watch 触发
      syncFormFromProfile 覆盖草稿（符合"切换设备时草稿重置为该设备当前配置"的语义）。
    -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="canConfigure" v-show="showConfig" class="modal-overlay" @click.self="showConfig = false">
          <div class="modal-panel">
            <DaqP1604Config
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
        <div v-if="showScanDialog" class="modal-overlay" @click.self="closeScanDialog">
          <div class="modal-panel modal-panel--scan">
            <div class="dialog dialog--scan">
              <div class="dialog__header">
                <h3 class="dialog__title">{{ i18n.t('dialog.scanTitle') }}</h3>
                <p class="dialog__subtitle">{{ i18n.t('dialog.scanSubtitle') }}</p>
              </div>
              <div class="dialog__body dialog__body--scan">
                <!-- 顶部：默认自动连接开关（Q2=A：默认开启） -->
                <label class="scan-option">
                  <input v-model="defaultAutoConnectOnAdd" type="checkbox" />
                  <span>{{ i18n.t('dialog.defaultAutoConnect') }}</span>
                </label>
                <ScanResultList
                  v-model="scanSelection"
                  :results="deviceStore.scanResults"
                  :scanning="deviceStore.isScanning"
                  :existing-keys="deviceStore.existingDeviceKeys"
                />
              </div>
              <div class="dialog__actions">
                <button
                  class="dialog__btn dialog__btn--secondary"
                  :title="i18n.t('common.cancel')"
                  @click="closeScanDialog"
                >
                  {{ i18n.t('common.cancel') }}
                </button>
                <button
                  v-if="!deviceStore.isScanning"
                  class="dialog__btn dialog__btn--secondary"
                  @click="void deviceStore.scanDevices()"
                >
                  {{ i18n.t('dialog.rescan') }}
                </button>
                <button
                  class="dialog__btn dialog__btn--primary"
                  :disabled="scanSelection.length === 0 || isAddingScanned"
                  @click="confirmAddScanned"
                >
                  {{ isAddingScanned ? i18n.t('dialog.addingDevices') : i18n.t('dialog.addSelected', { n: scanSelection.length }) }}
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
                <h3 class="dialog__title">{{ i18n.t('dialog.addDeviceTitle') }}</h3>
                <p class="dialog__subtitle">{{ i18n.t('dialog.addDeviceSubtitle') }}</p>
              </div>
              <div class="dialog__body">
                <div class="dialog__field">
                  <label>{{ i18n.t('dialog.deviceName') }}</label>
                  <input v-model="newName" :placeholder="i18n.t('dialog.deviceNamePlaceholder')" autofocus @keyup.enter="confirmAddDevice" />
                </div>
                <div class="dialog__row">
                  <div class="dialog__field">
                    <label>{{ i18n.t('dialog.ipAddress') }}</label>
                    <input v-model="newAddress" placeholder="192.168.3.101" @keyup.enter="confirmAddDevice" />
                  </div>
                  <div class="dialog__field dialog__field--narrow">
                    <label>{{ i18n.t('dialog.port') }}</label>
                    <input v-model.number="newPort" type="number" min="1" max="65535" @keyup.enter="confirmAddDevice" />
                  </div>
                </div>
                <div class="dialog__field">
                  <label>{{ i18n.t('dialog.localIpAddress') }}</label>
                  <input v-model="newLocalAddress" placeholder="192.168.3.10" @keyup.enter="confirmAddDevice" />
                </div>
                <p v-if="addError" class="dialog__error">{{ addError }}</p>
              </div>
              <div class="dialog__actions">
                <button class="dialog__btn dialog__btn--secondary" @click="showAddDialog = false">{{ i18n.t('common.cancel') }}</button>
                <button class="dialog__btn dialog__btn--primary" @click="confirmAddDevice">{{ i18n.t('common.add') }}</button>
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
  overflow: hidden;
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

/* 扫描弹窗独立尺寸：更宽更高，便于一次浏览多台设备 */
.modal-panel--scan {
  max-width: 44rem;
  max-height: 88vh;
  height: 80vh;
}

.dialog--scan {
  height: 100%;
}

.dialog__body--scan {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
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
  letter-spacing: 0.04em;
  text-transform: uppercase;
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

.dialog__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  box-shadow: none;
}

/* 扫描弹窗顶部选项条：默认自动连接开关 */
.scan-option {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--accent-soft);
  border: 1px solid var(--accent-border);
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
}

.scan-option input[type='checkbox'] {
  width: 14px;
  height: 14px;
  cursor: pointer;
  accent-color: var(--accent);
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
