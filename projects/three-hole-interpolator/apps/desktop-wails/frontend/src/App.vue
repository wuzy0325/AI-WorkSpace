<script setup lang="ts">
import { ref, computed } from 'vue'
import { api, isWailsAvailable, type PrbFileInfo, type LoadPrbResult, type InterpolationInput, type InterpolationResult } from './wails-adapter'

const loaded = ref(false)
const prbFiles = ref<PrbFileInfo[]>([])
const machRange = ref<number[]>([])
const inputs = ref<InterpolationInput[]>([])
const results = ref<(InterpolationResult | null)[]>([])
const calculating = ref(false)
const statusMsg = ref('')
const statusType = ref<'info' | 'success' | 'error' | 'warning'>('info')
const defaultPatm = ref(101325)
const defaultTatm = ref(20)
const newP1 = ref(0)
const newP2 = ref(0)
const newP3 = ref(0)
const activeTab = ref<'input' | 'results'>('input')
const pressureMode = ref<'gauge' | 'absolute'>('gauge')

const hasResults = computed(() => results.value.length > 0 && results.value.some(r => r !== null))

function setStatus(msg: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
  statusMsg.value = msg
  statusType.value = type
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

function formatVal(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toFixed(4)
}

function formatNum(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toLocaleString('zh-CN')
}

function buildCurrentInput(): InterpolationInput {
  return {
    P1: newP1.value, P2: newP2.value, P3: newP3.value,
    Patm: defaultPatm.value, Tatm: defaultTatm.value,
    pressureMode: pressureMode.value,
  }
}

function isValidInput(input: InterpolationInput): boolean {
  return [input.P1, input.P2, input.P3, input.Patm, input.Tatm].every(Number.isFinite)
}

function addRow() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
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

async function loadPrb() {
  if (!isWailsAvailable()) {
    setStatus('当前不在 Wails 环境中运行', 'error')
    return
  }
  try {
    const [resp, result] = await api.loadPrbFiles()
    if (!resp.success) {
      setStatus('加载失败: ' + resp.error, 'error')
      return
    }
    if (!result || !Array.isArray(result.files)) {
      setStatus('加载失败: PRB 文件信息为空', 'error')
      return
    }
    loaded.value = true
    prbFiles.value = result.files
    machRange.value = result.machRange ?? []
    const names = result.files.map(f => f.fileName).join(', ')
    setStatus(`已加载 ${result.files.length} 个 PRB 文件: ${names}`, 'success')
  } catch (e: any) {
    setStatus('加载失败: ' + (e.message || e), 'error')
  }
}

async function importCsv() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
    return
  }
  if (!isWailsAvailable()) {
    setStatus('当前不在 Wails 环境中运行', 'error')
    return
  }
  try {
    const [resp, data] = await api.importCsvData()
    if (!resp.success) {
      setStatus('导入失败: ' + resp.error, 'error')
      return
    }
    for (const d of data) {
      d.pressureMode = pressureMode.value
      inputs.value.push(d)
      results.value.push(null)
    }
    setStatus(`已导入 ${data.length} 条数据`, 'success')
  } catch (e: any) {
    setStatus('导入失败: ' + (e.message || e), 'error')
  }
}

async function calculateAll() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
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
    if (!resp.success) {
      setStatus('计算失败: ' + resp.error, 'error')
      return
    }
    results.value = res
    const valid = res.filter(r => r && r.isValid).length
    setStatus(`计算完成，共 ${res.length} 条`, 'success')
    activeTab.value = 'results'
  } catch (e: any) {
    setStatus('计算失败: ' + (e.message || e), 'error')
  } finally {
    calculating.value = false
  }
}

function escapeCsvField(value: string | number): string {
  const s = String(value)
  if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}

function exportResults() {
  if (!hasResults.value) {
    setStatus('没有可导出的结果', 'warning')
    return
  }

  const headers = ['序号', 'α(°)', 'Ma', 'Pt(Pa)', 'Ps(Pa)']
  const rows = results.value.map((r, idx) => [
    idx + 1,
    r ? formatVal(r.alpha) : '-',
    r ? formatVal(r.machNumber) : '-',
    r ? formatVal(r.P0) : '-',
    r ? formatVal(r.Ps) : '-',
  ].map(escapeCsvField))

  const csvContent = [headers.map(escapeCsvField).join(','), ...rows.map(row => row.join(','))].join('\n')
  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `三孔探针计算结果_${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`
  link.click()
  URL.revokeObjectURL(link.href)
  setStatus('结果已导出', 'success')
}
</script>

<template>
  <div class="app">
    <header class="app-header">
      <div class="header-brand">
        <div class="logo">
          <svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"/>
            <circle cx="12" cy="12" r="3"/>
            <line x1="12" y1="2" x2="12" y2="6"/>
            <line x1="12" y1="18" x2="12" y2="22"/>
            <line x1="2" y1="12" x2="6" y2="12"/>
            <line x1="18" y1="12" x2="22" y2="12"/>
          </svg>
        </div>
        <div class="brand-text">
          <h1>三孔探针插值计算</h1>
          <span class="subtitle">Three-Hole Probe Interpolation System</span>
        </div>
      </div>
      <div class="header-actions">
        <button class="btn btn-help" @click="openHelp" title="打开用户说明书">帮助</button>
        <button class="btn btn-primary" @click="loadPrb" :disabled="calculating">加载 PRB 文件</button>
      </div>
    </header>

    <Transition name="slide-down">
      <div v-if="loaded && prbFiles.length > 0" class="info-card">
        <div class="info-item">
          <span class="info-label">已加载文件</span>
          <span class="info-value file-name">{{ prbFiles.length }} 个 PRB 文件</span>
        </div>
        <div class="info-divider"></div>
        <div class="info-item" v-if="machRange.length === 2">
          <span class="info-label">Ma 范围</span>
          <span class="info-value">{{ machRange[0].toFixed(3) }} ~ {{ machRange[1].toFixed(3) }}</span>
        </div>
        <div class="info-divider" v-if="machRange.length === 2"></div>
        <div class="prb-tags">
          <span class="prb-tag" v-for="f in prbFiles" :key="f.fileName">{{ f.fileName }} <em>Ma={{ f.machNumber.toFixed(3) }}</em></span>
        </div>
        <div class="info-status active"><span class="status-dot"></span>就绪</div>
      </div>
    </Transition>

    <main class="main-content">
      <div class="tabs">
        <button class="tab-btn" :class="{ active: activeTab === 'input' }" @click="activeTab = 'input'">数据输入</button>
        <button class="tab-btn" :class="{ active: activeTab === 'results' }" @click="activeTab = 'results'" :disabled="!hasResults">计算结果</button>
      </div>

      <Transition name="fade" mode="out-in">
        <div v-if="activeTab === 'input'" key="input" class="panel">
          <div class="input-section">
            <div class="section-header">
              <h3>压力参数输入</h3>
              <div class="section-actions">
                <div class="pressure-mode-switch">
                  <button class="mode-btn" :class="{ active: pressureMode === 'gauge' }" @click="pressureMode = 'gauge'">表压</button>
                  <button class="mode-btn" :class="{ active: pressureMode === 'absolute' }" @click="pressureMode = 'absolute'">绝压</button>
                </div>
                <button class="btn btn-secondary" @click="importCsv">导入数据</button>
                <button class="btn btn-secondary" @click="clearAll" :disabled="inputs.length === 0">清空</button>
              </div>
            </div>

            <div class="input-row">
              <div class="input-group">
                <label class="input-label"><span class="label-text">P1</span><span class="label-hint">1号孔(表压/绝压)</span></label>
                <input v-model.number="newP1" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label"><span class="label-text">P2</span><span class="label-hint">中心孔(表压/绝压)</span></label>
                <input v-model.number="newP2" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label"><span class="label-text">P3</span><span class="label-hint">3号孔(表压/绝压)</span></label>
                <input v-model.number="newP3" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label"><span class="label-text">Patm</span><span class="label-hint">大气压</span></label>
                <input v-model.number="defaultPatm" type="number" step="any" class="input-field" placeholder="101325" />
              </div>
              <div class="input-group">
                <label class="input-label"><span class="label-text">Tatm</span><span class="label-hint">大气温</span></label>
                <input v-model.number="defaultTatm" type="number" step="any" class="input-field" placeholder="20" />
              </div>
              <div class="input-group action-group">
                <label class="input-label"><span class="label-text">操作</span><span class="label-hint">添加数据</span></label>
                <button class="btn btn-primary btn-add" @click="addRow">添加</button>
              </div>
            </div>

            <div class="calculate-bar">
              <button class="btn btn-primary btn-calculate" :class="{ 'btn-loading': calculating }" @click="calculateAll" :disabled="calculating">
                {{ calculating ? '计算中...' : '执行插值计算' }}
              </button>
            </div>
          </div>

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
                      <th class="col-num">#</th><th>P1</th><th>P2</th><th>P3</th><th>Patm</th><th>Tatm</th><th>模式</th><th class="col-action">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(row, idx) in inputs" :key="idx" class="data-row">
                      <td class="col-num">{{ idx + 1 }}</td>
                      <td>{{ formatNum(row.P1) }}</td><td>{{ formatNum(row.P2) }}</td>
                      <td>{{ formatNum(row.P3) }}</td><td>{{ formatNum(row.Patm) }}</td>
                      <td>{{ row.Tatm }}</td>
                      <td class="col-mode">{{ row.pressureMode === 'absolute' ? '绝压' : '表压' }}</td>
                      <td class="col-action"><button class="btn-icon danger" @click="removeRow(idx)" title="删除">[X]</button></td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </Transition>

          <Transition name="fade">
            <div v-if="inputs.length === 0" class="empty-state">
              <p class="empty-title">暂无数据</p>
              <p class="empty-desc">输入三个探针孔压力参数后可直接执行插值计算，也可以先点击"添加"保存到列表</p>
            </div>
          </Transition>
        </div>

        <div v-else key="results" class="panel">
          <div class="result-header">
            <div class="result-stats">
              <div class="stat-card"><span class="stat-value">{{ results.length }}</span><span class="stat-label">总记录</span></div>
            </div>
            <button class="btn btn-secondary" @click="exportResults">导出 CSV</button>
          </div>
          <div class="table-container results-table">
            <table class="data-table">
              <thead>
                <tr>
                  <th class="col-num">#</th><th>α (°)</th><th>Ma</th><th>Pt (Pa)</th><th>Ps (Pa)</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(r, idx) in results" :key="idx">
                  <td class="col-num">{{ idx + 1 }}</td>
                  <td>{{ r ? formatVal(r.alpha) : '-' }}</td>
                  <td>{{ r ? formatVal(r.machNumber) : '-' }}</td>
                  <td>{{ r ? formatVal(r.P0) : '-' }}</td>
                  <td>{{ r ? formatVal(r.Ps) : '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </Transition>
    </main>

    <footer class="status-bar">
      <div class="status-content">
        <span class="status-indicator" :class="statusType"></span>
        <span class="status-text">{{ statusMsg || '就绪' }}</span>
      </div>
      <div class="status-meta"><span>三孔探针插值计算系统 v1.0</span></div>
    </footer>
  </div>
</template>

<style>
:root {
  --primary-50: #eef2ff;  --primary-100: #e0e7ff;  --primary-200: #c7d2fe;
  --primary-300: #a5b4fc;  --primary-400: #818cf8;  --primary-500: #6366f1;
  --primary-600: #4f46e5;  --primary-700: #4338ca;  --primary-800: #3730a3;
  --primary-900: #312e81;
  --gray-50: #f8fafc;   --gray-100: #f1f5f9;   --gray-200: #e2e8f0;
  --gray-300: #cbd5e1;  --gray-400: #94a3b8;   --gray-500: #64748b;
  --gray-600: #475569;  --gray-700: #334155;   --gray-800: #1e293b;
  --gray-900: #0f172a;
  --success-50: #f0fdf4;   --success-100: #dcfce7;   --success-500: #22c55e;   --success-600: #16a34a;
  --error-50: #fef2f2;     --error-100: #fee2e2;      --error-500: #ef4444;      --error-600: #dc2626;
  --warning-50: #fffbeb;   --warning-500: #f59e0b;    --warning-600: #d97706;
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1);
  --radius-sm: 6px;   --radius-md: 10px;   --radius-lg: 14px;
  --transition-fast: 150ms ease;   --transition-base: 250ms ease;   --transition-slow: 350ms ease;
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', -apple-system, BlinkMacSystemFont, sans-serif;
  background: var(--gray-100); color: var(--gray-800); line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

.app { max-width: 1440px; margin: 0 auto; padding: 12px; min-height: 100vh; display: flex; flex-direction: column; gap: 10px; }

/* ===== Header ===== */
.app-header {
  background: white; border-radius: var(--radius-lg); padding: 14px 20px;
  display: flex; justify-content: space-between; align-items: center;
  box-shadow: var(--shadow-sm);
}

.header-brand { display: flex; align-items: center; gap: 12px; }
.logo { color: var(--primary-600); display: flex; align-items: center; }
.brand-text h1 { font-size: 18px; font-weight: 600; color: var(--gray-900); letter-spacing: -0.3px; }
.subtitle { font-size: 12px; color: var(--gray-400); }

.header-actions { display: flex; align-items: center; gap: 8px; }

/* ===== Buttons ===== */
.btn {
  display: inline-flex; align-items: center; justify-content: center;
  height: 32px; padding: 0 14px; border: none; border-radius: var(--radius-sm);
  font-size: 13px; font-weight: 500; cursor: pointer;
  transition: all var(--transition-fast); white-space: nowrap;
}

.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: var(--primary-600); color: white; }
.btn-primary:hover:not(:disabled) { background: var(--primary-700); }
.btn-secondary { background: white; color: var(--gray-700); border: 1px solid var(--gray-200); }
.btn-secondary:hover:not(:disabled) { background: var(--gray-50); border-color: var(--gray-300); }
.btn-help { background: transparent; color: var(--gray-500); }
.btn-help:hover { background: var(--gray-100); }

/* ===== Info Card ===== */
.info-card {
  background: white; border-radius: var(--radius-md); padding: 12px 18px;
  display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  box-shadow: var(--shadow-sm); border: 1px solid var(--primary-100);
}

.info-item { display: flex; align-items: center; gap: 6px; }
.info-label { font-size: 12px; color: var(--gray-400); }
.info-value { font-size: 13px; font-weight: 500; color: var(--gray-700); }
.info-divider { width: 1px; height: 20px; background: var(--gray-200); }
.prb-tags { display: flex; gap: 4px; flex-wrap: wrap; flex: 1; }
.prb-tag {
  font-size: 11px; padding: 2px 8px; background: var(--primary-50);
  color: var(--primary-700); border-radius: 4px; white-space: nowrap;
}
.prb-tag em { font-style: normal; color: var(--primary-500); margin-left: 2px; }

.info-status { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--gray-400); }
.status-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--gray-300); }
.info-status.active .status-dot { background: var(--success-500); }
.info-status.active { color: var(--success-600); }

/* ===== Tabs ===== */
.tabs { display: flex; gap: 0; background: white; border-radius: var(--radius-md); overflow: hidden; box-shadow: var(--shadow-sm); }
.tab-btn {
  flex: 1; padding: 10px; border: none; background: transparent; font-size: 14px; font-weight: 500;
  color: var(--gray-500); cursor: pointer; transition: all var(--transition-fast);
  border-bottom: 2px solid transparent;
}
.tab-btn.active { color: var(--primary-600); border-bottom-color: var(--primary-600); }
.tab-btn:hover:not(:disabled) { color: var(--primary-500); background: var(--gray-50); }
.tab-btn:disabled { cursor: not-allowed; }

/* ===== Panels ===== */
.panel { background: white; border-radius: var(--radius-md); padding: 20px; box-shadow: var(--shadow-sm); }
.section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.section-header h3 { font-size: 15px; font-weight: 600; color: var(--gray-800); }
.section-actions { display: flex; align-items: center; gap: 8px; }

/* ===== Pressure Mode Switch ===== */
.pressure-mode-switch { display: flex; border: 1px solid var(--gray-200); border-radius: var(--radius-sm); overflow: hidden; }
.mode-btn { padding: 4px 12px; border: none; background: transparent; font-size: 12px; cursor: pointer; color: var(--gray-500); }
.mode-btn.active { background: var(--primary-600); color: white; }
.mode-btn:not(.active):hover { background: var(--gray-50); }

/* ===== Input Row ===== */
.input-row { display: flex; gap: 10px; flex-wrap: wrap; align-items: flex-end; }
.input-group { flex: 1; min-width: 100px; }
.input-group.action-group { flex: 0 0 auto; min-width: auto; }
.input-label { display: flex; flex-direction: column; gap: 2px; margin-bottom: 4px; }
.label-text { font-size: 12px; font-weight: 500; color: var(--gray-700); }
.label-hint { font-size: 10px; color: var(--gray-400); }
.input-field {
  width: 100%; height: 36px; padding: 0 10px; border: 1px solid var(--gray-200);
  border-radius: var(--radius-sm); font-size: 13px; outline: none;
  transition: border-color var(--transition-fast);
}
.input-field:focus { border-color: var(--primary-400); box-shadow: 0 0 0 3px var(--primary-100); }
.btn-add { height: 36px; }

/* ===== Calculate Bar ===== */
.calculate-bar { margin-top: 14px; }
.btn-calculate { width: 100%; height: 40px; font-size: 15px; font-weight: 600; }
.btn-loading { opacity: 0.8; }

/* ===== Tables ===== */
.data-table-section { margin-top: 18px; }
.table-container { overflow-x: auto; border: 1px solid var(--gray-200); border-radius: var(--radius-sm); }
.data-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.data-table th {
  background: var(--gray-50); padding: 8px 10px; text-align: right; font-weight: 600;
  color: var(--gray-600); border-bottom: 1px solid var(--gray-200); white-space: nowrap;
}
.data-table th.col-num, .data-table td.col-num { text-align: center; width: 40px; }
.data-table th.col-action, .data-table td.col-action { text-align: center; width: 50px; }
.data-table th.col-mode, .data-table td.col-mode { text-align: center; }
.data-table td { padding: 6px 10px; text-align: right; border-bottom: 1px solid var(--gray-100); color: var(--gray-700); }
.data-table tbody tr:hover { background: var(--gray-50); }

.count-badge {
  font-size: 11px; padding: 2px 10px; background: var(--primary-50);
  color: var(--primary-600); border-radius: 10px;
}

.btn-icon { background: none; border: none; cursor: pointer; font-size: 11px; padding: 2px 4px; color: var(--gray-400); }
.btn-icon.danger:hover { color: var(--error-500); }

/* ===== Results ===== */
.result-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.result-stats { display: flex; gap: 10px; }
.stat-card {
  display: flex; flex-direction: column; align-items: center; padding: 8px 16px;
  background: var(--gray-50); border-radius: var(--radius-sm); border: 1px solid var(--gray-200);
}
.stat-value { font-size: 22px; font-weight: 700; color: var(--gray-800); }
.stat-card.success .stat-value { color: var(--success-600); }
.stat-card.error .stat-value { color: var(--error-500); }
.stat-label { font-size: 11px; color: var(--gray-400); margin-top: 2px; }

/* ===== Empty State ===== */
.empty-state { text-align: center; padding: 40px 20px; }
.empty-title { font-size: 16px; font-weight: 500; color: var(--gray-400); }
.empty-desc { font-size: 13px; color: var(--gray-300); margin-top: 8px; }

/* ===== Status Bar ===== */
.status-bar {
  background: white; border-radius: var(--radius-md); padding: 8px 16px;
  display: flex; justify-content: space-between; align-items: center;
  box-shadow: var(--shadow-sm); font-size: 12px; color: var(--gray-400);
}
.status-content { display: flex; align-items: center; gap: 6px; }
.status-text { color: var(--gray-600); }
.status-indicator { width: 6px; height: 6px; border-radius: 50%; background: var(--gray-300); }
.status-indicator.success { background: var(--success-500); }
.status-indicator.error { background: var(--error-500); }
.status-indicator.warning { background: var(--warning-500); }
.status-indicator.info { background: var(--primary-500); }

/* ===== Transitions ===== */
.fade-enter-active, .fade-leave-active { transition: opacity var(--transition-base); }
.fade-enter-from, .fade-leave-to { opacity: 0; }
.slide-down-enter-active, .slide-down-leave-active { transition: all var(--transition-base); overflow: hidden; }
.slide-down-enter-from, .slide-down-leave-to { opacity: 0; max-height: 0; margin: 0; padding: 0; }
.slide-up-enter-active, .slide-up-leave-active { transition: all var(--transition-base); overflow: hidden; }
.slide-up-enter-from, .slide-up-leave-to { opacity: 0; transform: translateY(10px); }

/* ===== Responsive ===== */
@media (max-width: 1024px) {
  .input-row { gap: 8px; }
  .input-group { min-width: 80px; }
}
@media (max-width: 768px) {
  .app-header { flex-direction: column; gap: 10px; }
  .header-actions { width: 100%; justify-content: flex-end; }
  .brand-text h1 { font-size: 16px; }
  .info-card { flex-direction: column; align-items: flex-start; }
  .info-divider { display: none; }
}
</style>
