<script setup lang="ts">
import type { CalibrationType } from '@shared/types/calibration'
import { ArrowRight, CheckCircle2, Info } from '@lucide/vue'
import IconCalibrationFiveHole from '@components/icons/IconCalibrationFiveHole.vue'
import IconCalibrationThreeHole from '@components/icons/IconCalibrationThreeHole.vue'
import IconCalibrationTotalPressure from '@components/icons/IconCalibrationTotalPressure.vue'
import IconCalibrationTotalTemperature from '@components/icons/IconCalibrationTotalTemperature.vue'

const emit = defineEmits<{
  selectCalibration: [type: CalibrationType]
}>()

const calibrationTypes = [
  {
    type: 'five-hole' as CalibrationType,
    name: '五孔探针校准',
    subtitle: 'Five-Hole Probe',
    description: '三维空间流动测量，支持攻角(α)与侧滑角(β)双轴联动标定。',
    features: ['Kα-Kβ系数空间计算', 'CPT/CPS气动系数', '81点自动矩阵扫描'],
    colors: {
      primary: '#3b82f6',
      primaryLight: '#60a5fa',
      primaryDark: '#1d4ed8',
      bg: '#eff6ff',
      bgHover: '#dbeafe',
      border: '#bfdbfe',
      borderHover: '#3b82f6',
      text: '#1e40af',
      textLight: '#3b82f6',
      accent: '#2563eb',
      shadow: 'rgba(59, 130, 246, 0.15)',
      gradient: 'linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%)',
      gradientSoft: 'linear-gradient(135deg, #60a5fa 0%, #3b82f6 100%)',
    }
  },
  {
    type: 'three-hole' as CalibrationType,
    name: '三孔探针校准',
    subtitle: 'Three-Hole Probe',
    description: '二维平面流动测量，专注于偏航角(θ)的精准方向标定。',
    features: ['方向系数K线性拟合', '速度系数Cv修正', '13点标准角度标定'],
    colors: {
      primary: '#10b981',
      primaryLight: '#34d399',
      primaryDark: '#047857',
      bg: '#ecfdf5',
      bgHover: '#d1fae5',
      border: '#a7f3d0',
      borderHover: '#10b981',
      text: '#065f46',
      textLight: '#10b981',
      accent: '#059669',
      shadow: 'rgba(16, 185, 129, 0.15)',
      gradient: 'linear-gradient(135deg, #10b981 0%, #047857 100%)',
      gradientSoft: 'linear-gradient(135deg, #34d399 0%, #10b981 100%)',
    }
  },
  {
    type: 'total-pressure' as CalibrationType,
    name: '总压探针校准',
    subtitle: 'Total Pressure',
    description: '总压测量精度验证，自动分析不同攻角下的压力恢复系数。',
    features: ['CPT恢复系数分析', '角度敏感度误差评估', '31点高精度步进'],
    colors: {
      primary: '#f59e0b',
      primaryLight: '#fbbf24',
      primaryDark: '#b45309',
      bg: '#fffbeb',
      bgHover: '#fef3c7',
      border: '#fde68a',
      borderHover: '#f59e0b',
      text: '#92400e',
      textLight: '#f59e0b',
      accent: '#d97706',
      shadow: 'rgba(245, 158, 11, 0.15)',
      gradient: 'linear-gradient(135deg, #f59e0b 0%, #b45309 100%)',
      gradientSoft: 'linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%)',
    }
  },
  {
    type: 'total-temperature' as CalibrationType,
    name: '总温探针校准',
    subtitle: 'Total Temperature',
    description: '总温传感器特性研究，分析不同马赫数下的恢复系数r。',
    features: ['恢复系数r-Ma曲线', '多工况点温度平衡测试', '滞止温度对比修正'],
    colors: {
      primary: '#f43f5e',
      primaryLight: '#fb7185',
      primaryDark: '#be123c',
      bg: '#fff1f2',
      bgHover: '#ffe4e6',
      border: '#fecdd3',
      borderHover: '#f43f5e',
      text: '#9f1239',
      textLight: '#f43f5e',
      accent: '#e11d48',
      shadow: 'rgba(244, 63, 94, 0.15)',
      gradient: 'linear-gradient(135deg, #f43f5e 0%, #be123c 100%)',
      gradientSoft: 'linear-gradient(135deg, #fb7185 0%, #f43f5e 100%)',
    }
  }
]

function selectType(type: CalibrationType) {
  emit('selectCalibration', type)
}

function getIconComponent(type: CalibrationType) {
  switch (type) {
    case 'five-hole': return IconCalibrationFiveHole
    case 'three-hole': return IconCalibrationThreeHole
    case 'total-pressure': return IconCalibrationTotalPressure
    case 'total-temperature': return IconCalibrationTotalTemperature
    default: return IconCalibrationFiveHole
  }
}
</script>

<template>
  <div class="calibration-home">
    <!-- 头部 -->
    <div class="header">
      <div class="header-content">
        <div class="header-icon">
          <IconCalibrationFiveHole :size="22" />
        </div>
        <div>
          <h1 class="header-title">探针校准控制中心</h1>
          <p class="header-subtitle">Probe Calibration & Aerodynamic Analysis System</p>
        </div>
      </div>
    </div>

    <!-- 校准类型网格 -->
    <div class="grid-container">
      <div class="grid">
        <div
          v-for="item in calibrationTypes"
          :key="item.type"
          class="card"
          :style="{
            '--card-primary': item.colors.primary,
            '--card-primary-light': item.colors.primaryLight,
            '--card-primary-dark': item.colors.primaryDark,
            '--card-bg': item.colors.bg,
            '--card-bg-hover': item.colors.bgHover,
            '--card-border': item.colors.border,
            '--card-border-hover': item.colors.borderHover,
            '--card-text': item.colors.text,
            '--card-text-light': item.colors.textLight,
            '--card-accent': item.colors.accent,
            '--card-shadow': item.colors.shadow,
            '--card-gradient': item.colors.gradient,
            '--card-gradient-soft': item.colors.gradientSoft,
          }"
          @click="selectType(item.type)"
        >
          <!-- 顶部渐变装饰条 -->
          <div class="card-accent" />

          <!-- 背景装饰圆 -->
          <div class="card-bg-decoration" />

          <div class="card-header">
            <div class="card-title-row">
              <!-- 渐变图标背景 -->
              <div class="card-icon">
                <component :is="getIconComponent(item.type)" :size="32" />
              </div>
              <div>
                <h3 class="card-name">{{ item.name }}</h3>
                <p class="card-subtitle">{{ item.subtitle }}</p>
              </div>
            </div>
            <!-- 箭头按钮 -->
            <div class="card-arrow">
              <ArrowRight class="arrow-icon" />
            </div>
          </div>

          <p class="card-description">{{ item.description }}</p>

          <!-- 特性标签 -->
          <div class="card-features">
            <div
              v-for="(feature, idx) in item.features"
              :key="idx"
              class="feature-tag"
            >
              <CheckCircle2 class="feature-icon" />
              <span class="feature-text">{{ feature }}</span>
            </div>
          </div>

          <!-- 底部装饰线 -->
          <div class="card-footer-line" />
        </div>
      </div>
    </div>

    <!-- 底部状态 -->
    <div class="footer">
      <div class="footer-content">
        <div class="footer-icon">
          <Info class="info-icon" />
        </div>
        <p class="footer-text">
          <span class="footer-label">System Ready:</span>
          <span class="footer-status">✓ HW Check Passed</span>
          <span class="footer-separator">|</span>
          <span class="footer-hint">请在开始自动化校准前，手动验证各压力通道零位及运动控制器归零状态。</span>
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.calibration-home {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  height: 100%;
  /* 主体背景跟随主题 token：浅色为浅灰渐变，暗色为深蓝渐变，无需媒体查询 */
  background: linear-gradient(180deg, var(--bg-canvas) 0%, var(--bg-app) 100%);
}

/* 头部 */
.header {
  flex-shrink: 0;
  border-bottom: 1px solid var(--border-default);
  background: linear-gradient(90deg, var(--bg-panel) 0%, var(--bg-canvas) 100%);
  padding: 1rem 1.5rem;
}

.header-content {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.header-icon {
  display: flex;
  height: 2.5rem;
  width: 2.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.75rem;
  /* 品牌 icon 渐变保留为品牌色，主题切换不应改变品牌视觉语言 */
  background: linear-gradient(135deg, #3b82f6, #4f46e5);
  color: white;
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.3);
}

.header-title {
  font-size: var(--font-size-lg);
  font-weight: 700;
  letter-spacing: -0.025em;
  color: var(--text-primary);
  margin: 0;
}

.header-subtitle {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  margin: 0;
  font-weight: 500;
}

/* 网格容器 */
.grid-container {
  flex: 1;
  min-height: 0;
  padding: 1.25rem;
  overflow: auto;
  display: flex;
}

.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  grid-template-rows: repeat(2, 1fr);
  gap: 1.25rem;
  width: 100%;
  min-height: 100%;
}

/* 卡片：基础背景使用 panel token，--card-bg 仅作为品牌色叠加层（半透明） */
.card {
  position: relative;
  display: flex;
  flex-direction: column;
  cursor: pointer;
  border-radius: 1rem;
  border: 2px solid var(--card-border);
  background: linear-gradient(145deg, var(--bg-panel) 0%, var(--bg-canvas) 100%);
  padding: 1.25rem 1.5rem 1.5rem;
  overflow: hidden;
  transition: border-color 200ms ease, box-shadow 240ms ease, transform 240ms ease, background 240ms ease;
  min-height: 0;
}

.card:hover {
  border-color: var(--card-border-hover);
  box-shadow: 0 20px 40px -12px var(--card-shadow), 0 8px 16px -8px rgba(0, 0, 0, 0.08);
  transform: translateY(-2px);
  background: linear-gradient(145deg, var(--bg-panel-strong) 0%, var(--bg-panel) 100%);
}

/* 顶部渐变装饰条 */
.card-accent {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 4px;
  background: var(--card-gradient);
}

/* 背景装饰圆 */
.card-bg-decoration {
  position: absolute;
  top: -60px;
  right: -60px;
  width: 180px;
  height: 180px;
  border-radius: 50%;
  background: var(--card-gradient-soft);
  opacity: 0.06;
  transition: all 0.5s ease;
  pointer-events: none;
}

.card:hover .card-bg-decoration {
  opacity: 0.12;
  transform: scale(1.1);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 1rem;
  position: relative;
  z-index: 1;
  flex-shrink: 0;
}

.card-title-row {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.card-icon {
  display: flex;
  height: 4rem;
  width: 4rem;
  align-items: center;
  justify-content: center;
  border-radius: 1rem;
  background: var(--card-gradient);
  color: white;
  box-shadow: 0 8px 20px -4px var(--card-shadow);
  transition: all 0.35s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}

.card-icon::after {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 1.1rem;
  background: var(--card-gradient-soft);
  opacity: 0;
  z-index: -1;
  transition: opacity 0.35s ease;
}

.card:hover .card-icon {
  transform: scale(1.08) rotate(-3deg);
  box-shadow: 0 12px 28px -6px var(--card-shadow);
}

.card:hover .card-icon::after {
  opacity: 0.4;
}

.card-name {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
  transition: color 0.3s ease;
}

.card:hover .card-name {
  color: var(--card-text);
}

.card-subtitle {
  font-size: var(--font-size-sm);
  font-weight: 600;
  margin: 0.25rem 0 0;
  color: var(--card-text-light);
  letter-spacing: 0.02em;
}

.card-arrow {
  display: flex;
  height: 2.5rem;
  width: 2.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--bg-panel-strong);
  color: var(--card-text-light);
  transition: all 0.35s cubic-bezier(0.4, 0, 0.2, 1);
  border: 2px solid var(--card-border);
}

.card:hover .card-arrow {
  background: var(--card-gradient);
  color: white;
  border-color: transparent;
  box-shadow: 0 4px 12px -2px var(--card-shadow);
}

.arrow-icon {
  width: 1.25rem;
  height: 1.25rem;
  transition: transform 0.35s cubic-bezier(0.4, 0, 0.2, 1);
}

.card:hover .arrow-icon {
  transform: translateX(3px);
}

.card-description {
  margin: 0 0 1rem 0;
  font-size: var(--font-size-sm);
  line-height: 1.55;
  color: var(--text-secondary);
  position: relative;
  z-index: 1;
  flex: 1;
  min-height: 0;
}

.card-features {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  position: relative;
  z-index: 1;
  flex-shrink: 0;
}

.feature-tag {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  border-radius: 0.5rem;
  padding: 0.375rem 0.625rem;
  background: var(--bg-panel-strong);
  border: 1.5px solid var(--card-border);
  transition: background 200ms ease, border-color 200ms ease;
}

.card:hover .feature-tag {
  background: var(--card-bg-hover);
  border-color: var(--card-border-hover);
}

.feature-icon {
  width: 0.875rem;
  height: 0.875rem;
  color: var(--card-text-light);
  flex-shrink: 0;
}

.feature-text {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--card-text);
}

/* 底部装饰线 */
.card-footer-line {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: var(--card-gradient);
  opacity: 0;
  transition: opacity 0.35s ease;
}

.card:hover .card-footer-line {
  opacity: 0.6;
}

/* 底部 */
.footer {
  flex-shrink: 0;
  border-top: 1px solid var(--border-default);
  background: linear-gradient(90deg, var(--bg-panel) 0%, var(--bg-canvas) 100%);
  padding: 0.75rem 1.25rem;
}

.footer-content {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.footer-icon {
  display: flex;
  height: 1.75rem;
  width: 1.75rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  /* 警告色品牌渐变，保留跨主题一致 */
  background: linear-gradient(135deg, #fbbf24, #f97316);
  color: white;
  flex-shrink: 0;
  box-shadow: 0 2px 8px rgba(251, 191, 36, 0.3);
}

.info-icon {
  width: 0.875rem;
  height: 0.875rem;
}

.footer-text {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  margin: 0;
}

.footer-label {
  font-weight: 600;
  color: var(--text-secondary);
}

.footer-status {
  margin-left: 0.25rem;
  font-weight: 600;
  /* 成功色渐变保留为品牌色，跨主题一致 */
  background: linear-gradient(90deg, #059669, #10b981);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.footer-separator {
  margin: 0 0.5rem;
  color: var(--text-muted);
}

.footer-hint {
  color: var(--text-tertiary);
}
</style>
