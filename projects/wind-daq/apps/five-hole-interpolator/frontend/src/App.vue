<script setup lang="ts">
import { ref, computed } from 'vue'
import { api, isWailsAvailable, type PrbFileInfo, type LoadPrbResult, type InterpolationInput, type InterpolationResult } from './wails-adapter'
import { OpenHelpDoc } from '../wailsjs/go/backend/App'

// ==================== 状态管理 ====================
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
const newP4 = ref(0)
const newP5 = ref(0)
const activeTab = ref<'input' | 'results'>('input')

// ==================== 计算属性 ====================
const validResultsCount = computed(() => results.value.filter(r => r !== null && r.isValid).length)
const invalidResultsCount = computed(() => results.value.filter(r => r !== null && !r.isValid).length)
const hasResults = computed(() => results.value.length > 0 && results.value.some(r => r !== null))

// ==================== 工具函数 ====================
function setStatus(msg: string, type: 'info' | 'success' | 'error' | 'warning' = 'info') {
  statusMsg.value = msg
  statusType.value = type
}

async function openHelp() {
  if (!isWailsAvailable()) {
    setStatus('当前不在 Wails 环境中运行', 'error')
    return
  }
  try {
    await OpenHelpDoc()
  } catch (e: any) {
    setStatus('打开帮助文档失败: ' + (e.message || e), 'error')
  }
}

function formatVal(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toFixed(4)
}

function formatInt(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toLocaleString('zh-CN')
}

// ==================== 数据操作 ====================
function addRow() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
    return
  }
  inputs.value.push({
    P1: newP1.value,
    P2: newP2.value,
    P3: newP3.value,
    P4: newP4.value,
    P5: newP5.value,
    Patm: defaultPatm.value,
    Tatm: defaultTatm.value,
  })
  results.value.push(null)
  setStatus('已添加一行数据', 'success')
  newP1.value = 0
  newP2.value = 0
  newP3.value = 0
  newP4.value = 0
  newP5.value = 0
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

// ==================== 文件操作 ====================
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
    loaded.value = true
    prbFiles.value = result!.files
    machRange.value = result!.machRange
    const names = result!.files.map(f => f.fileName).join(', ')
    setStatus(`已加载 ${result!.files.length} 个 PRB 文件: ${names}`, 'success')
  } catch (e: any) {
    setStatus('加载失败: ' + (e.message || e), 'error')
  }
}

async function importCsv() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
    return
  }
  if (!isWailsAvailable()) return
  try {
    const [resp, data] = await api.importCsvData()
    if (!resp.success) {
      setStatus('导入失败: ' + resp.error, 'error')
      return
    }
    for (const d of data) {
      inputs.value.push(d)
      results.value.push(null)
    }
    setStatus(`已导入 ${data.length} 条数据`, 'success')
  } catch (e: any) {
    setStatus('导入失败: ' + (e.message || e), 'error')
  }
}

// ==================== 计算 ====================
async function calculateAll() {
  if (!loaded.value) {
    setStatus('请先加载 PRB 文件', 'warning')
    return
  }
  if (inputs.value.length === 0) {
    setStatus('请先添加数据', 'warning')
    return
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
    setStatus(`计算完成！有效结果: ${valid}/${res.length} 条`, 'success')
    activeTab.value = 'results'
  } catch (e: any) {
    setStatus('计算失败: ' + (e.message || e), 'error')
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

  const headers = ['序号', 'α(°)', 'β(°)', 'Ma', 'TAS(m/s)', 'CAS(m/s)', 'SAT(K)', 'Qc(Pa)', 'ρ(kg/m³)', 'Pt(Pa)', 'Ps(Pa)', '状态']
  const rows = results.value.map((r, idx) => [
    idx + 1,
    r ? formatVal(r.alpha) : '-',
    r ? formatVal(r.beta) : '-',
    r ? formatVal(r.machNumber) : '-',
    r ? formatVal(r.velocity) : '-',
    r ? formatVal(r.cas) : '-',
    r ? formatVal(r.sat) : '-',
    r ? formatVal(r.dynamicPressure) : '-',
    r ? formatVal(r.density) : '-',
    r ? formatVal(r.P0) : '-',
    r ? formatVal(r.Ps) : '-',
    r ? (r.isValid ? '有效' : '无效: ' + r.warning) : '-',
  ])

  const csvContent = [headers.join(','), ...rows.map(row => row.join(','))].join('\n')
  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `五孔探针计算结果_${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.csv`
  link.click()
  URL.revokeObjectURL(link.href)
  setStatus('结果已导出', 'success')
}
</script>

<template>
  <div class="app">
    <!-- 顶部标题栏 -->
    <header class="app-header">
      <div class="header-brand">
        <div class="logo">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="3"/>
            <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/>
          </svg>
        </div>
        <div class="brand-text">
          <h1>五孔探针插值计算</h1>
          <span class="subtitle">Five-Hole Probe Interpolation System</span>
        </div>
      </div>
      <div class="header-actions">
        <button
          class="btn btn-help"
          @click="openHelp"
          title="打开用户说明书"
        >
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          帮助
        </button>
        <button
          class="btn btn-primary"
          @click="loadPrb"
          :disabled="calculating"
        >
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7 10 12 15 17 10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          加载 PRB 文件
        </button>
      </div>
    </header>

    <!-- 文件信息卡片 -->
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
          <span class="prb-tag" v-for="f in prbFiles" :key="f.fileName">
            {{ f.fileName }} <em>Ma={{ f.machNumber.toFixed(3) }}</em>
          </span>
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
            {{ validResultsCount }}/{{ results.length }}
          </span>
        </button>
      </div>

      <!-- 数据输入面板 -->
      <Transition name="fade" mode="out-in">
        <div v-if="activeTab === 'input'" key="input" class="panel">
          <!-- 输入区域 -->
          <div class="input-section">
            <div class="section-header">
              <h3>压力参数输入</h3>
              <div class="section-actions">
                <button class="btn btn-secondary" @click="importCsv">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                    <polyline points="17 8 12 3 7 8"/>
                    <line x1="12" y1="3" x2="12" y2="15"/>
                  </svg>
                  导入 CSV
                </button>
                <button class="btn btn-secondary" @click="clearAll" :disabled="inputs.length === 0">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="3 6 5 6 21 6"/>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                  </svg>
                  清空
                </button>
              </div>
            </div>

            <!-- 单行输入布局 -->
            <div class="input-row">
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P1</span>
                  <span class="label-hint">下孔</span>
                </label>
                <input v-model.number="newP1" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P2</span>
                  <span class="label-hint">中心</span>
                </label>
                <input v-model.number="newP2" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P3</span>
                  <span class="label-hint">上孔</span>
                </label>
                <input v-model.number="newP3" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P4</span>
                  <span class="label-hint">左孔</span>
                </label>
                <input v-model.number="newP4" type="number" step="any" class="input-field" placeholder="0" />
              </div>
              <div class="input-group">
                <label class="input-label">
                  <span class="label-text">P5</span>
                  <span class="label-hint">右孔</span>
                </label>
                <input v-model.number="newP5" type="number" step="any" class="input-field" placeholder="0" />
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
                <button class="btn btn-primary btn-add" @click="addRow">
                  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="12" y1="5" x2="12" y2="19"/>
                    <line x1="5" y1="12" x2="19" y2="12"/>
                  </svg>
                  添加
                </button>
              </div>
            </div>

            <!-- 计算按钮区域 - 始终显示 -->
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
                      <td>{{ formatInt(row.Patm) }}</td>
                      <td>{{ row.Tatm }}</td>
                      <td class="col-action">
                        <button class="btn-icon danger" @click="removeRow(idx)" title="删除">
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
            <div v-if="inputs.length === 0" class="empty-state">
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
              <p class="empty-desc">输入压力参数后点击"添加"按钮，或导入 CSV 文件</p>
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
                  <th>α (°)</th>
                  <th>β (°)</th>
                  <th>Ma</th>
                  <th>TAS (m/s)</th>
                  <th>CAS (m/s)</th>
                  <th>SAT (K)</th>
                  <th>Qc (Pa)</th>
                  <th>ρ (kg/m³)</th>
                  <th>Pt (Pa)</th>
                  <th>Ps (Pa)</th>
                  <th class="col-status">状态</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(r, idx) in results" :key="idx" class="data-row" :class="{ invalid: r && !r.isValid }">
                  <td class="col-num">{{ idx + 1 }}</td>
                  <td>{{ r ? formatVal(r.alpha) : '-' }}</td>
                  <td>{{ r ? formatVal(r.beta) : '-' }}</td>
                  <td>{{ r ? formatVal(r.machNumber) : '-' }}</td>
                  <td>{{ r ? formatVal(r.velocity) : '-' }}</td>
                  <td>{{ r ? formatVal(r.cas) : '-' }}</td>
                  <td>{{ r ? formatVal(r.sat) : '-' }}</td>
                  <td>{{ r ? formatVal(r.dynamicPressure) : '-' }}</td>
                  <td>{{ r ? formatVal(r.density) : '-' }}</td>
                  <td>{{ r ? formatVal(r.P0) : '-' }}</td>
                  <td>{{ r ? formatVal(r.Ps) : '-' }}</td>
                  <td class="col-status">
                    <span v-if="r && r.isValid" class="status-badge success">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
                      有效
                    </span>
                    <span v-else-if="r && !r.isValid" class="status-badge error" :title="r.warning">
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
        <span>五孔探针插值计算系统 v1.0</span>
      </div>
    </footer>
  </div>
</template>

<style>
:root {
  --primary-50: #eef2ff;
  --primary-100: #e0e7ff;
  --primary-200: #c7d2fe;
  --primary-300: #a5b4fc;
  --primary-400: #818cf8;
  --primary-500: #6366f1;
  --primary-600: #4f46e5;
  --primary-700: #4338ca;
  --primary-800: #3730a3;
  --primary-900: #312e81;
  --gray-50: #f8fafc;
  --gray-100: #f1f5f9;
  --gray-200: #e2e8f0;
  --gray-300: #cbd5e1;
  --gray-400: #94a3b8;
  --gray-500: #64748b;
  --gray-600: #475569;
  --gray-700: #334155;
  --gray-800: #1e293b;
  --gray-900: #0f172a;
  --success-50: #f0fdf4;
  --success-100: #dcfce7;
  --success-500: #22c55e;
  --success-600: #16a34a;
  --error-50: #fef2f2;
  --error-100: #fee2e2;
  --error-500: #ef4444;
  --error-600: #dc2626;
  --warning-50: #fffbeb;
  --warning-500: #f59e0b;
  --warning-600: #d97706;
  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1);
  --radius-sm: 6px;
  --radius-md: 10px;
  --radius-lg: 14px;
  --transition-fast: 150ms ease;
  --transition-base: 250ms ease;
  --transition-slow: 350ms ease;
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', -apple-system, BlinkMacSystemFont, sans-serif;
  background: var(--gray-100);
  color: var(--gray-800);
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

.app { max-width: 1440px; margin: 0 auto; padding: 24px; min-height: 100vh; display: flex; flex-direction: column; gap: 20px; }

.app-header {
  background: linear-gradient(135deg, var(--gray-900) 0%, var(--gray-800) 100%);
  border-radius: var(--radius-lg);
  padding: 24px 32px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: var(--shadow-lg);
  position: relative;
  overflow: hidden;
}

.app-header::before {
  content: '';
  position: absolute;
  top: 0; right: 0;
  width: 300px; height: 100%;
  background: linear-gradient(135deg, transparent 0%, rgba(99, 102, 241, 0.1) 100%);
  pointer-events: none;
}

.header-brand { display: flex; align-items: center; gap: 16px; }

.logo {
  width: 48px; height: 48px;
  background: linear-gradient(135deg, var(--primary-500), var(--primary-700));
  border-radius: var(--radius-md);
  display: flex; align-items: center; justify-content: center;
  color: white;
  box-shadow: var(--shadow-md);
}

.logo svg { width: 28px; height: 28px; }

.brand-text h1 { font-size: 22px; font-weight: 700; color: white; letter-spacing: -0.5px; }
.brand-text .subtitle { font-size: 12px; color: var(--gray-400); font-weight: 500; letter-spacing: 0.5px; text-transform: uppercase; }

.header-actions { display: flex; gap: 12px; }

.info-card {
  background: white;
  border-radius: var(--radius-lg);
  padding: 16px 24px;
  display: flex;
  align-items: center;
  gap: 20px;
  box-shadow: var(--shadow-md);
  border: 1px solid var(--gray-200);
}

.info-item { display: flex; flex-direction: column; gap: 2px; }
.info-label { font-size: 11px; color: var(--gray-500); font-weight: 500; text-transform: uppercase; letter-spacing: 0.5px; }
.info-value { font-size: 14px; font-weight: 600; color: var(--gray-800); }
.info-value.file-name { color: var(--primary-600); }
.info-divider { width: 1px; height: 32px; background: var(--gray-200); }

.prb-tags { display: flex; gap: 6px; flex-wrap: wrap; }
.prb-tag { background: var(--primary-50); color: var(--primary-700); padding: 3px 10px; border-radius: var(--radius-sm); font-size: 12px; font-weight: 600; }
.prb-tag em { font-style: normal; color: var(--primary-500); margin-left: 4px; }

.info-status {
  margin-left: auto;
  display: flex; align-items: center; gap: 8px;
  padding: 6px 14px;
  background: var(--gray-100);
  border-radius: var(--radius-sm);
  font-size: 13px; font-weight: 600; color: var(--gray-500);
  transition: all var(--transition-base);
}

.info-status.active { background: var(--success-50); color: var(--success-600); }

.status-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--gray-400);
  transition: background var(--transition-base);
}

.info-status.active .status-dot { background: var(--success-500); box-shadow: 0 0 0 3px var(--success-100); }

.main-content { flex: 1; display: flex; flex-direction: column; gap: 16px; }

.tabs { display: flex; gap: 4px; background: var(--gray-200); padding: 4px; border-radius: var(--radius-md); width: fit-content; }

.tab-btn {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 20px;
  border: none; background: transparent;
  color: var(--gray-600); font-size: 14px; font-weight: 600;
  border-radius: var(--radius-sm); cursor: pointer;
  transition: all var(--transition-base);
}

.tab-btn:hover:not(:disabled) { color: var(--gray-800); background: rgba(255, 255, 255, 0.5); }
.tab-btn.active { background: white; color: var(--primary-600); box-shadow: var(--shadow-sm); }
.tab-btn:disabled { opacity: 0.5; cursor: not-allowed; }

.tab-btn .badge { padding: 2px 8px; background: var(--gray-200); color: var(--gray-600); border-radius: 10px; font-size: 11px; font-weight: 700; }
.tab-btn .badge.success { background: var(--success-100); color: var(--success-600); }
.tab-btn .badge.error { background: var(--error-100); color: var(--error-600); }

.panel { background: white; border-radius: var(--radius-lg); padding: 24px; box-shadow: var(--shadow-md); border: 1px solid var(--gray-200); }

.section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.section-header h3 { font-size: 16px; font-weight: 700; color: var(--gray-800); }
.section-actions { display: flex; gap: 8px; }
.count-badge { padding: 4px 12px; background: var(--primary-50); color: var(--primary-600); border-radius: 20px; font-size: 12px; font-weight: 700; }

.input-row { display: flex; flex-direction: row; gap: 12px; align-items: flex-end; flex-wrap: nowrap; }

.input-group { display: flex; flex-direction: column; gap: 6px; flex: 1; min-width: 0; }
.input-group.action-group { flex: 0 0 auto; min-width: 100px; }

.input-label { display: flex; flex-direction: column; gap: 2px; }
.label-text { font-size: 13px; font-weight: 600; color: var(--gray-700); }
.label-hint { font-size: 11px; color: var(--gray-500); }

.input-field {
  padding: 10px 12px;
  border: 2px solid var(--gray-200);
  border-radius: var(--radius-md);
  font-size: 14px; font-weight: 500; color: var(--gray-800);
  background: white;
  transition: all var(--transition-fast);
  width: 100%; min-width: 0;
}

.input-field:hover { border-color: var(--gray-300); }
.input-field:focus { outline: none; border-color: var(--primary-500); box-shadow: 0 0 0 3px var(--primary-100); }
.input-field::placeholder { color: var(--gray-400); }

.btn-add { width: 100%; justify-content: center; height: 42px; white-space: nowrap; }

.calculate-bar { display: flex; justify-content: center; margin-top: 20px; padding-top: 20px; border-top: 1px solid var(--gray-200); }

.btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  padding: 10px 18px;
  border: none; border-radius: var(--radius-md);
  font-size: 14px; font-weight: 600; cursor: pointer;
  transition: all var(--transition-fast); white-space: nowrap;
}

.btn:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-primary {
  background: linear-gradient(135deg, var(--primary-500), var(--primary-700));
  color: white;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.3);
}

.btn-primary:hover:not(:disabled) { background: linear-gradient(135deg, var(--primary-600), var(--primary-800)); box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4); transform: translateY(-1px); }
.btn-primary:active:not(:disabled) { transform: translateY(0); }

.btn-secondary { background: var(--gray-100); color: var(--gray-700); border: 1px solid var(--gray-200); }
.btn-secondary:hover:not(:disabled) { background: var(--gray-200); border-color: var(--gray-300); }

.btn-help {
  background: transparent;
  color: var(--gray-400);
  border: 1px solid var(--gray-600);
}
.btn-help:hover:not(:disabled) {
  background: rgba(255,255,255,0.1);
  color: white;
  border-color: var(--gray-400);
}

.btn-calculate { padding: 14px 48px; font-size: 16px; border-radius: var(--radius-lg); }
.btn-loading { position: relative; pointer-events: none; }

.btn-icon {
  display: flex; align-items: center; justify-content: center;
  width: 32px; height: 32px;
  border: none; border-radius: var(--radius-sm);
  background: transparent; color: var(--gray-500); cursor: pointer;
  transition: all var(--transition-fast);
}

.btn-icon:hover { background: var(--gray-100); color: var(--gray-700); }
.btn-icon.danger:hover { background: var(--error-50); color: var(--error-600); }
.btn-icon svg { width: 16px; height: 16px; }

.icon { width: 18px; height: 18px; flex-shrink: 0; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

.data-table-section { margin-top: 24px; padding-top: 24px; border-top: 1px solid var(--gray-200); }

.table-container { overflow-x: auto; border-radius: var(--radius-md); border: 1px solid var(--gray-200); scrollbar-width: thin; scrollbar-color: var(--gray-300) transparent; }
.table-container::-webkit-scrollbar { height: 6px; width: 6px; }
.table-container::-webkit-scrollbar-track { background: transparent; }
.table-container::-webkit-scrollbar-thumb { background: var(--gray-300); border-radius: 3px; }
.table-container::-webkit-scrollbar-thumb:hover { background: var(--gray-400); }

.data-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.data-table thead { background: var(--gray-50); }
.data-table th { padding: 12px 16px; text-align: center; font-weight: 700; color: var(--gray-600); font-size: 12px; text-transform: uppercase; letter-spacing: 0.5px; border-bottom: 2px solid var(--gray-200); white-space: nowrap; }
.data-table td { padding: 12px 16px; text-align: center; border-bottom: 1px solid var(--gray-100); color: var(--gray-700); font-weight: 500; white-space: nowrap; }

.data-row { transition: background var(--transition-fast); }
.data-row:hover { background: var(--primary-50); }
.data-row.invalid { background: var(--error-50); }
.data-row.invalid:hover { background: var(--error-100); }

.col-num { width: 50px; color: var(--gray-500); font-weight: 600; }
.col-action { width: 60px; }
.col-status { width: 90px; }

.status-badge { display: inline-flex; align-items: center; gap: 4px; padding: 4px 10px; border-radius: 20px; font-size: 12px; font-weight: 600; }
.status-badge.success { background: var(--success-100); color: var(--success-600); }
.status-badge.error { background: var(--error-100); color: var(--error-600); cursor: help; }
.status-badge svg { width: 14px; height: 14px; }

.result-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.result-stats { display: flex; gap: 12px; }

.stat-card { display: flex; flex-direction: column; align-items: center; padding: 12px 20px; background: var(--gray-50); border-radius: var(--radius-md); border: 1px solid var(--gray-200); min-width: 80px; }
.stat-card.success { background: var(--success-50); border-color: var(--success-100); }
.stat-card.error { background: var(--error-50); border-color: var(--error-100); }

.stat-value { font-size: 24px; font-weight: 800; color: var(--gray-800); line-height: 1; }
.stat-card.success .stat-value { color: var(--success-600); }
.stat-card.error .stat-value { color: var(--error-600); }
.stat-label { font-size: 11px; color: var(--gray-500); font-weight: 600; margin-top: 4px; text-transform: uppercase; }

.empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 60px 20px; text-align: center; }
.empty-icon { width: 80px; height: 80px; color: var(--gray-300); margin-bottom: 16px; }
.empty-icon svg { width: 100%; height: 100%; }
.empty-title { font-size: 18px; font-weight: 700; color: var(--gray-700); margin-bottom: 8px; }
.empty-desc { font-size: 14px; color: var(--gray-500); max-width: 400px; }

.status-bar { background: white; border-radius: var(--radius-lg); padding: 12px 24px; display: flex; justify-content: space-between; align-items: center; box-shadow: var(--shadow-sm); border: 1px solid var(--gray-200); }
.status-content { display: flex; align-items: center; gap: 10px; }

.status-indicator { width: 8px; height: 8px; border-radius: 50%; background: var(--gray-400); transition: all var(--transition-base); }
.status-indicator.info { background: var(--primary-500); box-shadow: 0 0 0 3px var(--primary-100); }
.status-indicator.success { background: var(--success-500); box-shadow: 0 0 0 3px var(--success-100); }
.status-indicator.error { background: var(--error-500); box-shadow: 0 0 0 3px var(--error-100); }
.status-indicator.warning { background: var(--warning-500); box-shadow: 0 0 0 3px var(--warning-50); }

.status-text { font-size: 13px; font-weight: 600; color: var(--gray-600); }
.status-meta { font-size: 12px; color: var(--gray-400); font-weight: 500; }

.fade-enter-active, .fade-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
.fade-enter-from, .fade-leave-to { opacity: 0; transform: translateY(8px); }

.slide-down-enter-active, .slide-down-leave-active { transition: all var(--transition-slow); }
.slide-down-enter-from, .slide-down-leave-to { opacity: 0; transform: translateY(-16px); }

.slide-up-enter-active, .slide-up-leave-active { transition: all var(--transition-slow); }
.slide-up-enter-from, .slide-up-leave-to { opacity: 0; transform: translateY(16px); }

@media (max-width: 1024px) {
  .input-row { flex-wrap: wrap; }
  .input-group { flex: 1 1 calc(25% - 12px); min-width: 120px; }
  .input-group.action-group { flex: 0 0 100px; }
}

@media (max-width: 768px) {
  .app { padding: 12px; }
  .app-header { flex-direction: column; gap: 16px; text-align: center; }
  .info-card { flex-wrap: wrap; gap: 12px; }
  .info-divider { display: none; }
  .input-group { flex: 1 1 calc(50% - 12px); }
  .result-header { flex-direction: column; gap: 16px; }
  .result-stats { width: 100%; justify-content: center; }
}
</style>
