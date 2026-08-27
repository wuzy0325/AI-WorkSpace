<template>
  <el-dialog
    :model-value="props.visible"
    :title="mode === 'create' ? '新增设备配置' : '编辑设备配置'"
    width="520px"
    :close-on-click-modal="false"
    destroy-on-close
    @update:model-value="(val: boolean) => val ? null : emit('update:visible', false)"
    @closed="handleClosed"
  >
    <div class="form-grid">
      <label v-if="mode === 'edit'">
        <span>设备ID</span>
        <input
          :value="form.id"
          data-test="form-id"
          type="text"
          disabled
        >
      </label>

      <label>
        <span>名称</span>
        <input
          v-model.trim="form.name"
          data-test="form-name"
          type="text"
          placeholder="输入设备名称"
        >
      </label>

      <label>
        <span>类型</span>
        <select
          v-model="form.type"
          data-test="form-type"
        >
          <option value="measure">计量设备</option>
          <option value="pressure">打压设备</option>
        </select>
      </label>

      <label>
        <span>型号</span>
        <select
          v-model="form.model"
          data-test="form-model"
        >
          <option
            value=""
            disabled
          >选择型号</option>
          <option
            v-for="m in modelOptions"
            :key="m.value"
            :value="m.value"
          >
            {{ m.label }}
          </option>
        </select>
      </label>

      <label>
        <span>IP地址</span>
        <input
          v-model.trim="form.host"
          data-test="form-host"
          type="text"
          placeholder="192.168.1.xxx"
        >
      </label>

      <label>
        <span>端口</span>
        <input
          v-model.number="form.port"
          data-test="form-port"
          type="number"
          placeholder="9000"
        >
      </label>

      <label>
        <span>绑定本地IP</span>
        <input
          v-model.trim="form.localAddr"
          data-test="form-localAddr"
          type="text"
          placeholder="留空自动选择 / 多网卡时指定"
        >
      </label>
    </div>

    <!-- DAQ-P-1603：每通道量程/单位配置（4-20mA 电流环必须配量程才能输出正确工程量） -->
    <div
      v-if="isP1603Model"
      class="channel-config"
    >
      <div class="channel-config-header">
        <span>通道量程配置（4mA → rangeMin，20mA → rangeMax）</span>
      </div>
      <div class="bulk-bar">
        <span class="bulk-label">总设置：</span>
        <select
          v-model="bulkUnit"
          data-test="bulk-unit"
        >
          <option
            v-for="u in p1603UnitOptions"
            :key="u"
            :value="u"
          >
            {{ u }}
          </option>
        </select>
        <input
          v-model.number="bulkRangeMin"
          data-test="bulk-rangeMin"
          type="number"
          step="any"
          placeholder="量程下限"
        >
        <input
          v-model.number="bulkRangeMax"
          data-test="bulk-rangeMax"
          type="number"
          step="any"
          placeholder="量程上限"
        >
        <el-button
          size="small"
          type="primary"
          data-test="bulk-apply"
          @click="applyBulkConfig"
        >
          <el-icon><Check /></el-icon>
          应用到全部
        </el-button>
        <el-button
          size="small"
          type="success"
          data-test="bulk-enable-all"
          @click="setAllEnabled(true)"
        >
          全部启用
        </el-button>
        <el-button
          size="small"
          type="warning"
          data-test="bulk-disable-all"
          @click="setAllEnabled(false)"
        >
          全部禁用
        </el-button>
      </div>
      <div class="channel-config-body">
        <table class="channel-table">
          <thead>
            <tr>
              <th>通道</th>
              <th>启用</th>
              <th>单位</th>
              <th>量程下限</th>
              <th>量程上限</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="ch in form.channels"
              :key="ch.index"
            >
              <td>{{ ch.name }}</td>
              <td>
                <input
                  v-model="ch.enabled"
                  type="checkbox"
                >
              </td>
              <td>
                <select
                  v-model="ch.unit"
                  data-test="ch-unit"
                >
                  <option
                    v-for="u in p1603UnitOptions"
                    :key="u"
                    :value="u"
                  >
                    {{ u }}
                  </option>
                </select>
              </td>
              <td>
                <input
                  v-model.number="ch.rangeMin"
                  data-test="ch-rangeMin"
                  type="number"
                  step="any"
                >
              </td>
              <td>
                <input
                  v-model.number="ch.rangeMax"
                  data-test="ch-rangeMax"
                  type="number"
                  step="any"
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <p
      v-if="errorMessage"
      class="form-error"
    >
      <el-icon><Warning /></el-icon>
      {{ errorMessage }}
    </p>

    <template #footer>
      <el-button @click="handleCancel">
        取消
      </el-button>
      <el-button
        data-test="submit-form"
        type="primary"
        @click="handleSubmit"
      >
        <el-icon><Check /></el-icon>
        保存
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Warning, Check } from '@element-plus/icons-vue'
import type { ChannelConfigDTO, DeviceDTO } from '@/types/device'
import { isValvelessModel } from '@/utils/deviceModels'

// ---- Props & Emits ----

const props = defineProps<{
  /** 对话框是否可见 */
  visible: boolean
  /** 创建/编辑模式 */
  mode: 'create' | 'edit'
  /** 已有设备 ID 列表（创建模式下用于 ID 去重） */
  existingIds: string[]
  /** 编辑模式下的初始设备数据 */
  initialDevice?: DeviceDTO | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  /** 保存设备配置 */
  (e: 'save', device: DeviceDTO): void
  /** 取消关闭 */
  (e: 'cancel'): void
}>()

// ---- 表单状态 ----

type DeviceFormState = {
  id: string
  name: string
  type: DeviceDTO['type']
  model: string
  host: string
  port: number
  localAddr: string
  status: DeviceDTO['status']
  /** 每通道采集配置（P1603 使用，需配置量程才能输出正确工程量） */
  channels: ChannelConfigDTO[]
}

const form = reactive<DeviceFormState>({
  id: '',
  name: '',
  type: 'measure',
  model: '',
  host: '',
  port: 9000,
  localAddr: '',
  status: 'disconnected',
  channels: []
})

const errorMessage = ref('')

// P1603 通道单位选项（与设备面板单位下拉一致；4-20mA 传感器工程量单位）
const p1603UnitOptions = ['Pa', 'kPa', 'MPa', 'psi', 'kgf/cm2', 'bar', 'mbar']

// ---- P1603 总设置（一键应用到全部 16 通道）----
// 背景：16 通道逐一填写量程/单位易出错且繁琐，批量设置后仍可逐行微调。
const bulkUnit = ref('Pa')
const bulkRangeMin = ref<number | undefined>(-5000)
const bulkRangeMax = ref<number | undefined>(5000)

/** 将单位/量程批量写入全部通道（仅 P1603 通道配置使用） */
function applyBulkConfig() {
  if (form.channels.length === 0) return
  for (const ch of form.channels) {
    ch.unit = bulkUnit.value
    if (typeof bulkRangeMin.value === 'number' && !Number.isNaN(bulkRangeMin.value)) {
      ch.rangeMin = bulkRangeMin.value
    }
    if (typeof bulkRangeMax.value === 'number' && !Number.isNaN(bulkRangeMax.value)) {
      ch.rangeMax = bulkRangeMax.value
    }
  }
}

/** 批量启用/禁用全部通道 */
function setAllEnabled(enabled: boolean) {
  for (const ch of form.channels) {
    ch.enabled = enabled
  }
}

// P1603 固定 16 通道；判型逻辑统一走 deviceModels 工具。
const isP1603Model = computed(() => isValvelessModel(form.model))

// 生成长度 16 的默认通道配置（对齐后端 domain.DefaultP1603Channels）
function defaultP1603Channels(): ChannelConfigDTO[] {
  return Array.from({ length: 16 }, (_, i) => ({
    index: i + 1,
    name: `CH${i + 1}`,
    enabled: true,
    unit: 'Pa',
    rangeMin: -5000,
    rangeMax: 5000,
    precision: 3
  }))
}

// 设备型号选项，与设备类型联动
const modelOptions = computed(() => {
  if (form.type === 'measure') {
    return [
      { value: 'WTN1604', label: 'WTN1604' },
      { value: 'DAQ-P-1603', label: 'DAQ-P-1603' }
    ]
  }
  return [
    { value: 'ConST811A', label: 'ConST811A' },
    { value: 'ConST820', label: 'ConST820' },
    { value: 'ConST860', label: 'ConST860' },
    { value: 'SPC4000', label: 'SPC4000' }
  ]
})

// 切换设备类型时自动设置型号：仅在创建模式下生效
watch(() => form.type, () => {
  if (props.mode === 'create') {
    if (form.type === 'measure') {
      form.model = 'WTN1604'
    } else {
      form.model = ''
    }
  }
})

// 创建模式下选择 P1603 时初始化 16 通道默认量程配置（用户需按传感器量程修改）
watch(() => form.model, (model) => {
  if (props.mode !== 'create' || !model) return
  if (isValvelessModel(model) && form.channels.length === 0) {
    form.channels = defaultP1603Channels()
  }
})

// ---- 对话框打开时初始化表单 ----

watch(() => props.visible, (isVisible) => {
  if (!isVisible) return
  if (props.mode === 'create') {
    initCreate()
  } else if (props.initialDevice) {
    initEdit(props.initialDevice)
  }
})

/** 重置为创建模式表单 */
function initCreate() {
  form.id = generateDeviceId()
  form.name = ''
  form.type = 'measure'
  form.model = ''
  form.host = ''
  form.port = 9000
  form.localAddr = ''
  form.status = 'disconnected'
  form.channels = []
  errorMessage.value = ''
}

/** 使用已有设备数据填充编辑模式表单 */
function initEdit(device: DeviceDTO) {
  form.id = device.id
  form.name = device.name
  form.type = device.type
  form.model = device.model
  form.host = device.host
  form.port = device.port
  form.localAddr = device.localAddr ?? ''
  form.status = device.status
  // 编辑模式：历史配置无 channels 时，P1603 回退默认（避免空表）
  form.channels = device.channels?.length
    ? device.channels.map(c => ({ ...c }))
    : (isP1603Model.value ? defaultP1603Channels() : [])
  // 总设置栏用当前通道实际值填充（取首个启用通道），避免显示误导性的默认值。
  syncBulkFromChannels()
  errorMessage.value = ''
}

/** 用首个启用通道的实际单位/量程回填总设置栏，使批量工具与通道表一致 */
function syncBulkFromChannels() {
  const ref = form.channels.find(c => c.enabled) ?? form.channels[0]
  if (!ref) return
  if (ref.unit) bulkUnit.value = ref.unit
  if (typeof ref.rangeMin === 'number') bulkRangeMin.value = ref.rangeMin
  if (typeof ref.rangeMax === 'number') bulkRangeMax.value = ref.rangeMax
}

// ---- 内部方法 ----

/** 生成唯一设备ID，格式 dev-{timestamp后6位}-{随机4位} */
function generateDeviceId(): string {
  const ts = Date.now().toString().slice(-6)
  const rand = Math.random().toString(36).slice(2, 6)
  return `dev-${ts}-${rand}`
}

function isValidIPv4(value: string): boolean {
  const segments = value.split('.')
  if (segments.length !== 4) return false
  return segments.every((segment) => {
    if (!/^\d+$/.test(segment)) return false
    const num = Number(segment)
    return num >= 0 && num <= 255
  })
}

function validateForm(): string {
  if (!form.host) return '请填写IP地址。'
  if (!isValidIPv4(form.host)) return 'IP地址格式不正确'
  if (!Number.isInteger(form.port) || form.port < 1 || form.port > 65535) return '端口必须在1-65535之间'
  if (!form.model) return '请选择设备型号'
  return ''
}

function handleCancel() {
  errorMessage.value = ''
  emit('cancel')
  emit('update:visible', false)
}

function handleSubmit() {
  errorMessage.value = ''
  const msg = validateForm()
  if (msg) {
    errorMessage.value = msg
    return
  }

  // 创建模式下，如果 ID 已存在则重新生成
  let id = form.id
  if (props.mode === 'create' && props.existingIds.includes(id)) {
    id = generateDeviceId()
  }

  emit('save', {
    id,
    name: form.name,
    type: form.type,
    model: form.model,
    host: form.host,
    port: form.port,
    localAddr: form.localAddr || undefined,
    status: 'disconnected',
    channels: isP1603Model.value ? form.channels : undefined
  })
}

function handleClosed() {
  errorMessage.value = ''
}
</script>

<style scoped lang="scss">
.form-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, 1fr);
}

.form-full {
  grid-column: 1 / -1;
}

.form-grid label {
  color: $slate-500;
  display: flex;
  flex-direction: column;
  font-size: 12px;
  gap: 4px;
  font-weight: 500;
}

.form-grid input,
.form-grid select {
  background: #ffffff;
  border: 1px solid $slate-300;
  border-radius: 6px;
  color: $slate-700;
  padding: 6px 8px;
  font-size: 12px;
  font-family: $font-sans;
  outline: none;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.12);
  }
}

.form-error {
  color: $red;
  margin: 8px 0 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;

  .el-icon {
    font-size: 14px;
  }
}

/* ---- P1603 通道量程配置表 ---- */
.channel-config {
  margin-top: 12px;
  border: 1px solid $slate-300;
  border-radius: 6px;
  overflow: hidden;
}

.channel-config-header {
  background: #f8fafc;
  border-bottom: 1px solid $slate-300;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 600;
  color: $slate-600;
}

/* ---- P1603 总设置栏：批量填写单位/量程 ---- */
.bulk-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 6px 10px;
  border-bottom: 1px solid $slate-300;
  background: #fff;

  .bulk-label {
    color: $slate-500;
    font-size: 12px;
    font-weight: 600;
    white-space: nowrap;
  }

  select,
  input[type='number'] {
    background: #fff;
    border: 1px solid $slate-300;
    border-radius: 4px;
    padding: 3px 6px;
    font-size: 12px;
    font-family: $font-sans;
    color: $slate-700;
    outline: none;

    &:focus {
      border-color: $mint;
    }
  }

  input[type='number'] {
    width: 90px;
  }

  .el-button {
    font-size: 12px;
  }
}

.channel-config-body {
  max-height: 260px;
  overflow-y: auto;
}

.channel-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;

  th,
  td {
    padding: 4px 6px;
    text-align: center;
    border-bottom: 1px solid #f1f5f9;
  }

  th {
    color: $slate-500;
    font-weight: 500;
    background: #fbfdfd;
    position: sticky;
    top: 0;
  }

  input[type='number'],
  select {
    width: 100%;
    min-width: 64px;
    box-sizing: border-box;
    background: #fff;
    border: 1px solid $slate-300;
    border-radius: 4px;
    padding: 3px 4px;
    font-size: 12px;
  }
}

@media (max-width: 900px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>