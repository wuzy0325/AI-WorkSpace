<script setup lang="ts">
import { h, ref } from 'vue'
import {
  NButton,
  NButtonGroup,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInputNumber,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  type DataTableColumns,
} from 'naive-ui'
import { NaiveThemeProvider } from '@shared-frontend/index'
import { useThemeStore } from '@stores/themeStore'

type ChannelRow = {
  channel: string
  device: string
  value: string
  trend: string
  status: 'RUN' | 'WARN' | 'OFFLINE'
}

const themeStore = useThemeStore()
const sampleRate = ref(20)
const deviceType = ref('dsa3217')

const channelRows: ChannelRow[] = [
  { channel: 'P-01', device: 'DSA3217', value: '101.328 kPa', trend: '+0.013', status: 'RUN' },
  { channel: 'P-02', device: 'DSA3217', value: '101.221 kPa', trend: '-0.008', status: 'RUN' },
  { channel: 'T-03', device: 'DAQ-T1603', value: '23.42 degC', trend: '+0.021', status: 'WARN' },
  { channel: 'M-X', device: 'B140 Axis', value: '12.500 mm', trend: '0.000', status: 'OFFLINE' },
]

const statusLabels: Record<ChannelRow['status'], string> = {
  RUN: '运行',
  WARN: '告警',
  OFFLINE: '离线',
}

const channelColumns: DataTableColumns<ChannelRow> = [
  { title: '通道', key: 'channel', width: 88 },
  { title: '设备', key: 'device', width: 124 },
  { title: '当前值', key: 'value', className: 'spike-mono', width: 132 },
  { title: '趋势', key: 'trend', className: 'spike-mono', width: 80 },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row) {
      return h(
        NTag,
        {
          size: 'small',
          type: row.status === 'RUN' ? 'success' : row.status === 'WARN' ? 'warning' : 'error',
          round: false,
        },
        { default: () => statusLabels[row.status] },
      )
    },
  },
]

const deviceOptions = [
  { label: 'DSA3217 压力扫描阀', value: 'dsa3217' },
  { label: 'DAQ-T1603 热电偶采集', value: 'daq-t1603' },
  { label: 'B140 运动控制器', value: 'b140' },
]

const scoreRows = [
  { label: '全局主题', value: '接入 themeStore，跟随 light/dark' },
  { label: '视觉来源', value: 'Naive token 由 Wind-DAQ token 映射' },
  { label: '落地策略', value: '基础组件用 Naive，业务组件项目内封装' },
  { label: '表格密度', value: 'small size + tabular numeric data' },
]
</script>

<template>
  <NaiveThemeProvider :theme="themeStore.theme">
    <div class="ui-spike-shell">
      <header class="ui-spike-header">
        <div>
          <p class="ui-spike-kicker">Wind-DAQ UI Library Spike</p>
          <h1>Naive UI Applied Theme Bridge</h1>
        </div>
        <div class="ui-spike-header__status">
          <span>开发态入口</span>
          <code>VITE_UI_SPIKE=1</code>
          <NSwitch
            :value="themeStore.theme === 'dark'"
            checked-value="dark"
            unchecked-value="light"
            @update:value="themeStore.setTheme"
          >
            <template #checked>深色</template>
            <template #unchecked>浅色</template>
          </NSwitch>
        </div>
      </header>

      <main class="ui-spike-main">
        <section class="ui-spike-scorecard" aria-label="Naive UI 应用策略">
          <div v-for="row in scoreRows" :key="row.label" class="ui-spike-scorecard__row">
            <span>{{ row.label }}</span>
            <strong>{{ row.value }}</strong>
          </div>
        </section>

        <section class="ui-spike-grid" aria-label="Naive UI 工业桌面组件样例">
          <NCard title="采集控制" size="small" :bordered="true">
            <NSpace vertical size="small" class="ui-spike-form-stack">
              <NForm label-placement="left" label-width="86px" size="small">
                <NFormItem label="设备类型">
                  <NSelect v-model:value="deviceType" :options="deviceOptions" />
                </NFormItem>
                <NFormItem label="采样频率">
                  <NInputNumber v-model:value="sampleRate" :min="1" :max="200" suffix="Hz" />
                </NFormItem>
              </NForm>
              <NButtonGroup>
                <NButton type="primary" size="small">开始采集</NButton>
                <NButton size="small">设备配置</NButton>
                <NButton type="error" size="small">急停</NButton>
              </NButtonGroup>
            </NSpace>
          </NCard>

          <NCard title="通道状态" size="small" :bordered="true">
            <NDataTable
              size="small"
              :columns="channelColumns"
              :data="channelRows"
              :pagination="false"
              :bordered="true"
              :single-line="false"
            />
          </NCard>
        </section>

        <section class="ui-spike-notes" aria-label="应用建议">
          <h2>应用建议</h2>
          <p>Naive UI 已按 Wind-DAQ 当前主题系统接入。浅色/深色切换仍以 `themeStore` 和项目 CSS token 为准。</p>
          <p>后续新增业务 UI 时，优先用 Naive 基础组件组合，再封装成设备卡、通道表、采集控制条、状态栏等业务组件。</p>
        </section>
      </main>
    </div>
  </NaiveThemeProvider>
</template>

<style scoped>
.ui-spike-shell {
  min-height: 100vh;
  background:
    radial-gradient(circle at 18% 0%, color-mix(in srgb, var(--accent-primary) 14%, transparent), transparent 32%),
    var(--bg-app);
  color: var(--text-primary);
  overflow: auto;
  padding: 20px;
}

.ui-spike-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 64px;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
  padding: 14px 18px;
}

.ui-spike-header h1,
.ui-spike-notes h2 {
  margin: 0;
}

.ui-spike-header h1 {
  font-size: 20px;
  letter-spacing: 0.01em;
}

.ui-spike-kicker {
  margin: 0 0 4px;
  color: var(--accent-info);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.ui-spike-header__status {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text-secondary);
  font-size: 12px;
}

.ui-spike-header__status code {
  border: 1px solid color-mix(in srgb, var(--accent-primary) 38%, transparent);
  border-radius: var(--radius-md);
  background: var(--accent-primary-muted);
  color: var(--text-primary);
  padding: 4px 8px;
}

.ui-spike-main {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 16px;
}

.ui-spike-scorecard {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

.ui-spike-scorecard__row {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  padding: 12px;
}

.ui-spike-scorecard__row span,
.ui-spike-scorecard__row strong {
  display: block;
}

.ui-spike-scorecard__row span {
  margin-bottom: 8px;
  color: var(--text-secondary);
  font-size: 12px;
}

.ui-spike-scorecard__row strong {
  color: var(--text-primary);
  font-size: 12px;
  line-height: 1.7;
}

.ui-spike-grid {
  display: grid;
  grid-template-columns: 0.8fr 1.2fr;
  gap: 16px;
}

.ui-spike-form-stack {
  width: 100%;
}

.ui-spike-notes {
  border: 1px solid color-mix(in srgb, var(--accent-primary) 24%, transparent);
  border-radius: var(--radius-md);
  background: var(--accent-primary-muted);
  padding: 14px 16px;
}

.ui-spike-notes h2 {
  margin-bottom: 8px;
  font-size: 15px;
}

.ui-spike-notes p {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.7;
}

:deep(.spike-mono) {
  font-family: var(--font-family-mono);
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1180px) {
  .ui-spike-grid,
  .ui-spike-scorecard {
    grid-template-columns: 1fr;
  }
}
</style>
