<script setup lang="ts">
// SevenHoleWorkspace.vue 是 7 孔探针工作区组件。
// 由 App.vue 在 activeProbe === 'seven' 时动态加载。
//
// 与 5 孔 / 3 孔工作区的关键差异（参考 SPEC 与 seven_hole_types.go）：
//   - 7 个压力输入 P1..P7（P7=中心孔，P1..P6=外围 60° 等分孔），无压力模式开关
//     （spec §1.1 强制所有孔压力为表压 gauge，绝压数据需在导入前由调用方转换）
//   - PRB / 校准 CSV 加载需要 7 个独立文件：
//       PRB: 7.prb（内区）+ 1.prb..6.prb（外区扇区 n）
//       CSV: 文件名含"(小角度区)"（内区）+ "(大角度N区)"（扇区 N）
//   - 支持数据源切换（PRB / 校准 CSV），切换时清空旧槽位避免混用两种解析入口
//   - ValidRange 显示（±30° Alpha/Beta 范围）替代 MachRange；MachMin/Max 恒为 0 仅供 UI 展示
//   - UI/CSV/说明书统一为 α=侧滑角（sideslip）、β=迎角（angle of attack），
//     与算法包 InterpolationResult.Alpha=sideslip、Beta=AOA 一致（SPEC §2.2）。
//     7 孔 Alpha/Beta 语义与 5 孔**反转**（5 孔 Alpha=迎角），不能复用 5 孔 UI 文案。
//   - 结果无 Vx/Vy/Vz 速度分量（5 孔有，7 孔无）
//   - 根类名 .workspace-seven，与 .workspace / .workspace-three 平级，避免 CSS 互相覆盖
//
// v0.2.0 重构：
//   - 校准文件加载逻辑抽出到 useSevenHoleCalibration composable（约 280 行）
//   - 7 个槽位行抽出到 SevenHoleSlotRow 子组件（约 150 行模板）
//   - 本文件专注于数据输入/计算/导出 UI，校准状态机由 composable 持有

import { ref, computed } from 'vue'
import {
  api,
  isWailsAvailable,
  type SevenHoleInterpolationInput,
  type SevenHoleInterpolationResult,
} from '../adapters/seven-hole'
import { useSevenHoleCalibration } from '../composables/useSevenHoleCalibration'
import { escapeCsvField } from '../utils/csv'
import { formatVal, formatInt, formatResultNum } from '../utils/format'

// emit('back') 由顶栏"返回"按钮触发，通知父组件 App.vue 切回欢迎页。
defineEmits<{
  (e: 'back'): void
}>()

// ==================== 状态管理 ====================
// 校准文件配置 + 加载状态机由 composable 持有，本组件只注入 setStatus 回调。
const statusMsg = ref('')
const statusType = ref<'info' | 'success' | 'error' | 'warning'>('info')

function setStatus(msg: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
  statusMsg.value = msg
  statusType.value = type
}

const {
  loaded,
  validRange,
  innerPointCount,
  outerPointCounts,
  isImporting,
  validRangeText,
  dataSourceText,
  batchImport,
} = useSevenHoleCalibration({ setStatus })

// 数据 / 计算状态：与 5/3 孔工作区一致。
const inputs = ref<SevenHoleInterpolationInput[]>([])
const results = ref<(SevenHoleInterpolationResult | null)[]>([])
const calculating = ref(false)
const defaultPatm = ref(101325)
const defaultTatm = ref(20)
// P1..P6=外围孔，P7=中心孔。所有压力均为表压（gauge，Pa）。
const newP1 = ref(0)
const newP2 = ref(0)
const newP3 = ref(0)
const newP4 = ref(0)
const newP5 = ref(0)
const newP6 = ref(0)
const newP7 = ref(0)
const activeTab = ref<'input' | 'results'>('input')

// ==================== 计算属性 ====================
// 状态分类：有效（在校准 PRB 角度范围内）/ 参考（外推，超出校准范围）/ 无效（计算失败）。
// 判定依据：result.isValid 表示能否算出结果；result.warning 非空（"外推点: ..."）表示
// 该结果经外推得到、角度已超出校准 PRB 范围，仅作参考。
const validResultsCount = computed(() => results.value.filter((r) => r !== null && r.isValid && !r.warning).length)
const referenceResultsCount = computed(() => results.value.filter((r) => r !== null && r.isValid && r.warning).length)
const invalidResultsCount = computed(() => results.value.filter((r) => r !== null && !r.isValid).length)
const hasResults = computed(() => results.value.length > 0 && results.value.some((r) => r !== null))

// ==================== 工具函数 ====================
// fmtNum 是 formatResultNum 的本地别名，5/3/7 孔 workspace 共享同一份泛型实现。
// 见 utils/format.ts。无效行（r=null 或 IsValid=false）统一显示 "-"。
const fmtNum = (
  r: SevenHoleInterpolationResult | null,
  sel: (r: SevenHoleInterpolationResult) => number,
): string => formatResultNum(r, sel)

// resultStatus 分类单行结果的状态：'valid'（在校准 PRB 角度范围内）/ 'reference'（外推，超出校准范围）
// / 'invalid'（计算失败）/ 'none'（未计算）。'reference' 与 'valid' 都算可显示，区别在于是否超出校准范围。
function resultStatus(r: SevenHoleInterpolationResult | null): 'valid' | 'reference' | 'invalid' | 'none' {
  if (!r) return 'none'
  if (!r.isValid) return 'invalid'
  // 算法侧仅在外推路径（角度超出校准 PRB 范围）为有效结果写入 warning，见 outer_extrapolation.go。
  return r.warning ? 'reference' : 'valid'
}

async function openHelp() {
  if (!isWailsAvailable()) {
    setStatus('当前不在 Wails 环境中运行', 'error')
    return
  }
  const resp = await api.openHelpDoc()
  if (!resp.success) {
    setStatus('打开帮助文档失败: ' + (resp.error || '未知错误'), 'error')
  }
}

// ==================== 单行输入 / 数据列表 ====================
function buildCurrentInput(): SevenHoleInterpolationInput {
  // 7 孔输入不带 PressureMode：spec §1.1 强制表压输入。
  return {
    P1: newP1.value,
    P2: newP2.value,
    P3: newP3.value,
    P4: newP4.value,
    P5: newP5.value,
    P6: newP6.value,
    P7: newP7.value,
    Patm: defaultPatm.value,
    Tatm: defaultTatm.value,
  } as SevenHoleInterpolationInput
}

function isValidInput(input: SevenHoleInterpolationInput): boolean {
  return [input.P1, input.P2, input.P3, input.P4, input.P5, input.P6, input.P7, input.Patm, input.Tatm].every(Number.isFinite)
}

function addRow() {
  if (!loaded.value) {
    setStatus('请先加载 PRB / 校准 CSV 文件', 'warning')
    return
  }
  const input = buildCurrentInput()
  if (!isValidInput(input)) {
    setStatus('请输入有效的压力和环境参数', 'warning')
    return
  }
  inputs.value.push(input)
  results.value.push(null)
  setStatus('已添加一行数据', 'success')
  newP1.value = 0
  newP2.value = 0
  newP3.value = 0
  newP4.value = 0
  newP5.value = 0
  newP6.value = 0
  newP7.value = 0
}

function removeRow(idx: number) {
  inputs.value.splice(idx, 1)
  results.value.splice(idx, 1)
  setStatus('已删除一行数据', 'info')
}

function clearAll() {
  inputs.value = []
  results.value = []
  setStatus('已清空所有数据', 'info')
}

// ==================== 数据 CSV 导入 ====================
async function importCsv() {
  if (!loaded.value) {
    setStatus('请先加载 PRB / 校准 CSV 文件', 'warning')
    return
  }
  if (!isWailsAvailable()) return
  try {
    const [resp, data] = await api.importCsvData()
    if (!resp.success) {
      setStatus('导入失败: ' + resp.error, 'error')
      return
    }
    // 7 孔 CSV 必含 P1-P7 + Patm + Tatm 共 9 列，全部必需；不存在 pressureMode 字段补齐逻辑。
    for (const d of data) {
      inputs.value.push(d)
      results.value.push(null)
    }
    setStatus(`已导入 ${data.length} 条数据`, 'success')
  } catch (e: unknown) {
    // e 类型未约束（Wails/运行时抛出），统一转 string 展示。
    setStatus('导入失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// ==================== 计算 ====================
async function calculateAll() {
  if (!loaded.value) {
    setStatus('请先加载 PRB / 校准 CSV 文件', 'warning')
    return
  }
  if (inputs.value.length === 0) {
    const input = buildCurrentInput()
    if (!isValidInput(input)) {
      setStatus('请输入有效的压力和环境参数', 'warning')
      return
    }
    inputs.value = [input]
    results.value = [null]
  }
  calculating.value = true
  setStatus('正在计算中，请稍候...', 'info')
  try {
    const [resp, res] = await api.batchCalculate(inputs.value)
    // 无论整体成功失败都更新结果：后端支持部分失败，让用户看到有效行 + 失败行的 Warning。
    results.value = res
    const valid = res.filter((r) => r && r.isValid && !r.warning).length
    const reference = res.filter((r) => r && r.isValid && r.warning).length
    const failed = res.length - valid - reference
    if (!resp.success) {
      setStatus(`部分行计算失败：有效 ${valid} 条、参考 ${reference} 条、无效 ${failed} 条，首条错误: ${resp.error}`, 'warning')
    } else if (reference > 0 || failed > 0) {
      setStatus(`计算完成：${valid} 条有效、${reference} 条参考（超出校准范围）、${failed} 条无效`, reference > 0 ? 'warning' : 'success')
    } else {
      setStatus(`计算完成！有效结果: ${valid}/${res.length} 条`, 'success')
    }
    activeTab.value = 'results'
  } catch (e: unknown) {
    // e 类型未约束（Wails/算法包抛出），统一转 string 展示。
    setStatus('计算失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  } finally {
    calculating.value = false
  }
}

// ==================== 导出结果 ====================
function exportResults() {
  if (!hasResults.value) {
    setStatus('没有可导出的结果', 'warning')
    return
  }

  // 7 孔结果字段：α(侧滑角)、β(迎角)、θ(俯仰角)、Ψ(滚转角)、Ma、V、P0(总压)、Ps(静压)。
  // 注意：与 5 孔不同，7 孔结果无 Vx/Vy/Vz 速度分量，且 α/β 语义反转（SPEC §2.2）。
  // CSV 表头明确标注 α/β/θ/Ψ 的物理含义，避免与 5 孔导出文件混淆。
  // α/β 与算法包字段直接对应：α 列绑定 result.alpha（sideslip）、β 列绑定 result.beta（AOA）。
  // θ/Ψ 是 PRB 网格原始角度坐标：内区小角度下 θ=α、Ψ=β，外区大角度下是探头坐标系角度。
  const headers = ['序号', 'α(°) 侧滑角', 'β(°) 迎角', 'θ(°) 俯仰角', 'Ψ(°) 滚转角', 'Ma', 'V(m/s)', '总压 P0(Pa)', '静压 Ps(Pa)', '状态']
  const rows = results.value.map((r, idx) => [
    idx + 1,
    fmtNum(r, (x) => x.alpha),
    fmtNum(r, (x) => x.beta),
    fmtNum(r, (x) => x.theta),
    fmtNum(r, (x) => x.phi),
    fmtNum(r, (x) => x.machNumber),
    fmtNum(r, (x) => x.velocity),
    fmtNum(r, (x) => x.P0),
    fmtNum(r, (x) => x.Ps),
    r ? (r.isValid ? (r.warning ? '参考' : '有效') : '无效: ' + r.warning) : '-',
  ].map(escapeCsvField))

  const csvContent = [headers.map(escapeCsvField).join(','), ...rows.map((row) => row.join(','))].join('\n')
  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `七孔探针计算结果_${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`
  link.click()
  URL.revokeObjectURL(link.href)
  setStatus('结果已导出', 'success')
}

</script>

<template>
  <div class="workspace-seven">
    <!-- 顶部标题栏 -->
    <header class="app-header">
      <div class="header-brand">
        <div class="logo">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <!-- 7 孔探针示意：中心 1 孔 + 外围 6 孔按 60° 等分 -->
            <circle cx="12" cy="12" r="2"/>
            <circle cx="12" cy="5" r="1.5"/>
            <circle cx="18.06" cy="8.5" r="1.5"/>
            <circle cx="18.06" cy="15.5" r="1.5"/>
            <circle cx="12" cy="19" r="1.5"/>
            <circle cx="5.94" cy="15.5" r="1.5"/>
            <circle cx="5.94" cy="8.5" r="1.5"/>
          </svg>
        </div>
        <div class="brand-text">
          <h1>七孔探针插值计算</h1>
          <span class="subtitle">Seven-Hole Probe Interpolation</span>
        </div>
      </div>
      <div class="header-actions">
        <button class="btn btn-back" @click="$emit('back')" title="返回探针选择页">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="19" y1="12" x2="5" y2="12"/>
            <polyline points="12 19 5 12 12 5"/>
          </svg>
          返回
        </button>
        <button class="btn btn-help" @click="openHelp" title="打开用户说明书">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          帮助
        </button>
        <button
          class="btn btn-primary"
          :class="{ 'btn-loading': isImporting }"
          :disabled="calculating || isImporting"
          @click="batchImport"
        >
          <svg v-if="!isImporting" class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7 10 12 15 17 10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          <svg v-else class="icon spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
          </svg>
          {{ isImporting ? '加载中...' : '加载 PRB/CSV 文件' }}
        </button>
      </div>
    </header>

    <Transition name="slide-down">
      <div v-if="loaded" class="info-card">
        <div class="info-item">
          <span class="info-label">已加载文件</span>
          <span class="info-value file-name">7 个 {{ dataSourceText }}</span>
        </div>
        <div class="info-divider"></div>
        <div v-if="validRangeText" class="info-item">
          <span class="info-label">角度范围</span>
          <span class="info-value">{{ validRangeText }}</span>
        </div>
        <div class="info-divider"></div>
        <div class="info-item">
          <span class="info-label">网格点</span>
          <span class="info-value">{{ innerPointCount + outerPointCounts.reduce((sum, count) => sum + count, 0) }}</span>
        </div>
        <div class="info-status active">
          <span class="status-dot"></span>
          就绪
        </div>
      </div>
    </Transition>

    <!-- 主内容区 -->
    <main class="main-content">
      <!-- 标签页切换 -->
      <div class="tabs">
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'input' }"
          @click="activeTab = 'input'"
        >
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
          </svg>
          数据输入
          <span v-if="inputs.length > 0" class="badge">{{ inputs.length }}</span>
        </button>
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'results' }"
          @click="activeTab = 'results'"
          :disabled="!hasResults"
        >
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
          </svg>
          计算结果
          <span v-if="hasResults" class="badge" :class="{ success: validResultsCount > 0, error: invalidResultsCount > 0 }">
            {{ validResultsCount + referenceResultsCount }}/{{ results.length }}
          </span>
        </button>
      </div>

      <!-- 数据输入面板 -->
      <Transition name="fade" mode="out-in">
        <div v-if="activeTab === 'input'" key="input" class="panel">
          <!-- 输入区域 -->
          <div class="input-section">
            <div class="section-header">
              <h3>压力参数输入（表压）</h3>
              <div class="section-actions">
                <button class="btn btn-secondary" @click="importCsv" :disabled="calculating">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                    <polyline points="17 8 12 3 7 8"/>
                    <line x1="12" y1="3" x2="12" y2="15"/>
                  </svg>
                  导入 CSV
                </button>
                <button class="btn btn-secondary" @click="clearAll" :disabled="calculating || inputs.length === 0">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                  </svg>
                  清空
                </button>
              </div>
            </div>

            <!-- 单行输入布局：7 孔需要 P1..P7 + Patm + Tatm + 操作 共 10 列。
                 宽屏一行铺满，窄屏通过媒体查询自动换行（参见 .input-row 响应式规则）。 -->
            <div class="input-row">
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P1</span>
                  <span class="label-hint">1号孔(表压)</span>
                </label>
                <input v-model.number="newP1" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P2</span>
                  <span class="label-hint">2号孔(表压)</span>
                </label>
                <input v-model.number="newP2" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P3</span>
                  <span class="label-hint">3号孔(表压)</span>
                </label>
                <input v-model.number="newP3" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P4</span>
                  <span class="label-hint">4号孔(表压)</span>
                </label>
                <input v-model.number="newP4" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P5</span>
                  <span class="label-hint">5号孔(表压)</span>
                </label>
                <input v-model.number="newP5" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P6</span>
                  <span class="label-hint">6号孔(表压)</span>
                </label>
                <input v-model.number="newP6" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P7</span>
                  <span class="label-hint">中心孔(表压)</span>
                </label>
                <input v-model.number="newP7" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">Patm</span>
                  <span class="label-hint">大气压</span>
                </label>
                <input v-model.number="defaultPatm" type="number" step="any" class="input-field" placeholder="101325" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">Tatm</span>
                  <span class="label-hint">大气温</span>
                </label>
                <input v-model.number="defaultTatm" type="number" step="any" class="input-field" placeholder="20" />
              </div>
              <div class="input-group action-group">
                <label class="input-label">
                  <span class="label-text">操作</span>
                  <span class="label-hint">添加数据</span>
                </label>
                <button class="btn btn-primary btn-add" @click="addRow" :disabled="calculating">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="12" y1="5" x2="12" y2="19"/>
                    <line x1="5" y1="12" x2="19" y2="12"/>
                  </svg>
                  添加
                </button>
              </div>
            </div>

            <!-- 计算按钮区域 -->
            <div class="calculate-bar">
              <button
                class="btn btn-primary btn-calculate"
                :class="{ 'btn-loading': calculating }"
                @click="calculateAll"
                :disabled="calculating"
              >
                <svg v-if="!calculating" class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <polygon points="5 3 19 12 5 21 5 3"/>
                </svg>
                <svg v-else class="icon spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 12a9 9 0 1 1-6.219-8.56"/>
                </svg>
                {{ calculating ? '计算中...' : '执行插值计算' }}
              </button>
            </div>
          </div>

          <!-- 数据列表 -->
          <Transition name="slide-up">
            <div v-if="inputs.length > 0" class="data-table-section">
              <div class="section-header">
                <h3>数据列表</h3>
                <span class="count-badge">{{ inputs.length }} 条记录</span>
              </div>
              <div class="table-container">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th class="col-num">#</th>
                      <th>P1</th>
                      <th>P2</th>
                      <th>P3</th>
                      <th>P4</th>
                      <th>P5</th>
                      <th>P6</th>
                      <th>P7</th>
                      <th>Patm</th>
                      <th>Tatm</th>
                      <th class="col-action">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(row, idx) in inputs" :key="idx" class="data-row">
                      <td class="col-num">{{ idx + 1 }}</td>
                      <td>{{ formatInt(row.P1) }}</td>
                      <td>{{ formatInt(row.P2) }}</td>
                      <td>{{ formatInt(row.P3) }}</td>
                      <td>{{ formatInt(row.P4) }}</td>
                      <td>{{ formatInt(row.P5) }}</td>
                      <td>{{ formatInt(row.P6) }}</td>
                      <td>{{ formatInt(row.P7) }}</td>
                      <td>{{ formatInt(row.Patm) }}</td>
                      <td>{{ row.Tatm }}</td>
                      <td class="col-action">
                        <button class="btn-icon danger" @click="removeRow(idx)" :disabled="calculating" title="删除">
                          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="3 6 5 6 21 6"/>
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                          </svg>
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </Transition>

          <!-- 空状态 -->
          <Transition name="fade">
            <div v-if="inputs.length === 0" class="empty-state empty-state--panel">
              <div class="empty-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                  <polyline points="14 2 14 8 20 8"/>
                  <line x1="16" y1="13" x2="8" y2="13"/>
                  <line x1="16" y1="17" x2="8" y2="17"/>
                  <polyline points="10 9 9 9 8 9"/>
                </svg>
              </div>
              <p class="empty-title">暂无数据</p>
              <p class="empty-desc">输入 7 个探针孔压力参数后可直接执行插值计算，也可以先点击"添加"保存到列表</p>
            </div>
          </Transition>
        </div>

        <!-- 计算结果面板 -->
        <div v-else key="results" class="panel">
          <div class="result-header">
            <div class="result-stats">
              <div class="stat-card">
                <span class="stat-value">{{ results.length }}</span>
                <span class="stat-label">总记录</span>
              </div>
              <div class="stat-card success">
                <span class="stat-value">{{ validResultsCount }}</span>
                <span class="stat-label">有效</span>
              </div>
              <div class="stat-card" v-if="referenceResultsCount > 0">
                <span class="stat-value">{{ referenceResultsCount }}</span>
                <span class="stat-label">参考</span>
              </div>
              <div class="stat-card error" v-if="invalidResultsCount > 0">
                <span class="stat-value">{{ invalidResultsCount }}</span>
                <span class="stat-label">无效</span>
              </div>
            </div>
            <button class="btn btn-secondary" @click="exportResults">
              <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                <polyline points="7 10 12 15 17 10"/>
                <line x1="12" y1="15" x2="12" y2="3"/>
              </svg>
              导出 CSV
            </button>
          </div>

          <div class="table-container results-table">
            <table class="data-table">
              <thead>
                <tr>
                  <th class="col-num">#</th>
                  <th>α(°) 侧滑角</th>
                  <th>β(°) 迎角</th>
                  <th>θ(°) 俯仰角</th>
                  <th>Ψ(°) 滚转角</th>
                  <th>Ma</th>
                  <th>V(m/s)</th>
                  <th>总压 P0(Pa)</th>
                  <th>静压 Ps(Pa)</th>
                  <th class="col-status">状态</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(r, idx) in results" :key="idx" class="data-row" :class="{ invalid: resultStatus(r) === 'invalid', reference: resultStatus(r) === 'reference' }">
                  <td class="col-num">{{ idx + 1 }}</td>
                  <td>{{ fmtNum(r, x => x.alpha) }}</td>
                  <td>{{ fmtNum(r, x => x.beta) }}</td>
                  <td>{{ fmtNum(r, x => x.theta) }}</td>
                  <td>{{ fmtNum(r, x => x.phi) }}</td>
                  <td>{{ fmtNum(r, x => x.machNumber) }}</td>
                  <td>{{ fmtNum(r, x => x.velocity) }}</td>
                  <td>{{ fmtNum(r, x => x.P0) }}</td>
                  <td>{{ fmtNum(r, x => x.Ps) }}</td>
                  <td class="col-status">
                    <span v-if="resultStatus(r) === 'valid'" class="status-badge success">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                      有效
                    </span>
                    <span v-else-if="resultStatus(r) === 'reference'" class="status-badge reference" :title="r?.warning">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="9" y1="12" x2="15" y2="12"/></svg>
                      参考
                    </span>
                    <span v-else-if="resultStatus(r) === 'invalid'" class="status-badge error" :title="r?.warning">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                      无效
                    </span>
                    <span v-else class="status-badge">-</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </Transition>
    </main>

    <!-- 状态栏 -->
    <footer class="status-bar">
      <div class="status-content">
        <span class="status-indicator" :class="statusType"></span>
        <span class="status-text">{{ statusMsg || '就绪' }}</span>
      </div>
      <div class="status-meta">
        <span>七孔探针工作区 v0.2.0</span>
      </div>
    </footer>
  </div>
</template>

<style src="../styles/seven-hole-workspace.css"></style>
