<script setup lang="ts">
import { computed, ref } from 'vue'
import UiSlider from '@components/ui/UiSlider.vue'

// 五孔探针角度示意：
// - alpha：偏航角（绕 Y 轴旋转），正值表示流向右偏
// - beta ：俯仰角（绕 X 轴旋转），正值表示流向上偏
const alpha = ref(25)
const beta = ref(20)

// SVG viewBox 内探针本体中心坐标，所有元素围绕此点组织
const PROBE_CX = 360
const PROBE_CY = 260

// alpha 弧参数：从探针底部向左下/右下弧出，弧展开角度随 alpha 绝对值缩放
const ALPHA_ARC_R = 90
const ALPHA_ARC_MAX_DEG = 45
const alphaArcAngle = computed(() =>
  Math.min(Math.abs(alpha.value) / 60, 1) * ALPHA_ARC_MAX_DEG
)

// alpha+ 弧（实线，向右下方向）
const alphaPlusPath = computed(() => {
  const start = { x: PROBE_CX, y: PROBE_CY + 70 }
  const rad = alphaArcAngle.value * Math.PI / 180
  const end = {
    x: PROBE_CX + Math.sin(rad) * ALPHA_ARC_R,
    y: start.y + Math.cos(rad) * ALPHA_ARC_R
  }
  return `M ${start.x} ${start.y} A ${ALPHA_ARC_R} ${ALPHA_ARC_R} 0 0 1 ${end.x.toFixed(1)} ${end.y.toFixed(1)}`
})

// alpha- 弧（虚线，向左下方向）
const alphaMinusPath = computed(() => {
  const start = { x: PROBE_CX, y: PROBE_CY + 70 }
  const rad = alphaArcAngle.value * Math.PI / 180
  const end = {
    x: PROBE_CX - Math.sin(rad) * ALPHA_ARC_R,
    y: start.y + Math.cos(rad) * ALPHA_ARC_R
  }
  return `M ${start.x} ${start.y} A ${ALPHA_ARC_R} ${ALPHA_ARC_R} 0 0 0 ${end.x.toFixed(1)} ${end.y.toFixed(1)}`
})

// beta 弧参数：从探针右侧向右上/右下弧出
const BETA_ARC_R = 80
const BETA_ARC_MAX_DEG = 45
const betaArcAngle = computed(() =>
  Math.min(Math.abs(beta.value) / 60, 1) * BETA_ARC_MAX_DEG
)

// beta+ 弧（实线，向右上方向）
const betaPlusPath = computed(() => {
  const start = { x: PROBE_CX + 100, y: PROBE_CY }
  const rad = betaArcAngle.value * Math.PI / 180
  const end = {
    x: start.x + Math.cos(rad) * BETA_ARC_R,
    y: PROBE_CY - Math.sin(rad) * BETA_ARC_R
  }
  return `M ${start.x} ${start.y} A ${BETA_ARC_R} ${BETA_ARC_R} 0 0 0 ${end.x.toFixed(1)} ${end.y.toFixed(1)}`
})

// beta- 弧（虚线，向右下方向）
const betaMinusPath = computed(() => {
  const start = { x: PROBE_CX + 100, y: PROBE_CY }
  const rad = betaArcAngle.value * Math.PI / 180
  const end = {
    x: start.x + Math.cos(rad) * BETA_ARC_R,
    y: PROBE_CY + Math.sin(rad) * BETA_ARC_R
  }
  return `M ${start.x} ${start.y} A ${BETA_ARC_R} ${BETA_ARC_R} 0 0 1 ${end.x.toFixed(1)} ${end.y.toFixed(1)}`
})
</script>

<template>
  <section class="probe-card flex h-full min-h-[480px] overflow-hidden rounded-xl">
    <!-- 左侧：SVG 示意图，占满剩余宽度 -->
    <div class="relative flex-1 min-w-0">
      <div class="probe-glow absolute inset-0"></div>

      <svg viewBox="0 0 720 540" role="img" aria-label="五孔探针角度示意" class="relative z-10 h-full w-full">
        <defs>
          <marker id="flow-arrow" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#d48b52" />
          </marker>
          <marker id="axis-arrow-blue" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#3355ff" />
          </marker>
          <marker id="axis-arrow-green" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#10b981" />
          </marker>
          <marker id="axis-arrow-red" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#ef4444" />
          </marker>
          <marker id="alpha-arrow" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#ffaa00" />
          </marker>
          <marker id="beta-arrow" markerWidth="12" markerHeight="12" refX="10" refY="6" orient="auto">
            <path d="M0,0 L12,6 L0,12 Z" fill="#ff00aa" />
          </marker>
          <linearGradient id="probe-body" x1="0" x2="1">
            <stop offset="0%" stop-color="#f8fafc" />
            <stop offset="45%" stop-color="#94a3b8" />
            <stop offset="100%" stop-color="#e2e8f0" />
          </linearGradient>
        </defs>

        <!-- 顶部标题文字 -->
        <text x="40" y="40" fill="#ffaa00" font-size="16" font-weight="bold">alpha: 偏航角（Yaw）</text>
        <text x="40" y="62" fill="#ff00aa" font-size="16" font-weight="bold">beta: 俯仰角（Pitch）</text>

        <!-- 流向箭头：横穿探针中心 -->
        <line x1="40" y1="260" x2="680" y2="260" stroke="#d48b52" stroke-width="3" stroke-dasharray="12 10" marker-end="url(#flow-arrow)" />
        <text x="60" y="248" fill="#d48b52" font-size="18" font-weight="bold">流向 Flow</text>

        <!-- 坐标轴系：原点统一在探针中心 (360, 260) -->
        <!-- Y 轴（蓝色，向上） -->
        <line x1="360" y1="260" x2="360" y2="80" stroke="#3355ff" stroke-width="4" marker-end="url(#axis-arrow-blue)" />
        <text x="372" y="92" fill="#3355ff" font-size="22" font-weight="bold">Y</text>

        <!-- -Z 轴（绿色，右下） -->
        <line x1="360" y1="260" x2="500" y2="400" stroke="#10b981" stroke-width="3" marker-end="url(#axis-arrow-green)" />
        <text x="510" y="418" fill="#10b981" font-size="18" font-weight="bold">-Z</text>

        <!-- X 轴（红色，左下，探针轴向） -->
        <line x1="360" y1="260" x2="220" y2="400" stroke="#ef4444" stroke-width="3" marker-end="url(#axis-arrow-red)" />
        <text x="120" y="418" fill="#ef4444" font-size="18" font-weight="bold">X probe axis</text>

        <!-- alpha 弧（偏航方向，水平面内，探针底部） -->
        <path :d="alphaPlusPath" fill="none" stroke="#ffaa00" stroke-width="5" marker-end="url(#alpha-arrow)" />
        <path :d="alphaMinusPath" fill="none" stroke="#ffaa00" stroke-width="5" stroke-dasharray="10 8" marker-end="url(#alpha-arrow)" />
        <text x="395" y="385" fill="#ffaa00" font-size="16" font-weight="bold">alpha+</text>
        <text x="245" y="385" fill="#ffaa00" font-size="16" font-weight="bold">alpha-</text>

        <!-- beta 弧（俯仰方向，垂直面内，探针右侧） -->
        <path :d="betaPlusPath" fill="none" stroke="#ff00aa" stroke-width="5" marker-end="url(#beta-arrow)" />
        <path :d="betaMinusPath" fill="none" stroke="#ff00aa" stroke-width="5" stroke-dasharray="10 8" marker-end="url(#beta-arrow)" />
        <text x="500" y="225" fill="#ff00aa" font-size="16" font-weight="bold">beta+</text>
        <text x="500" y="320" fill="#ff00aa" font-size="16" font-weight="bold">beta-</text>

        <!-- 探针本体（5 孔布局）：P1 中心，P2-P5 环形 -->
        <g transform="translate(360 260)">
          <ellipse cx="0" cy="0" rx="100" ry="65" fill="url(#probe-body)" opacity="0.98" />
          <ellipse cx="0" cy="0" rx="68" ry="42" fill="#dbe4ee" opacity="0.96" />
          <!-- P1 中心孔 -->
          <circle cx="0" cy="0" r="20" fill="#020617" />
          <!-- P2 上孔 -->
          <circle cx="0" cy="-48" r="20" fill="#020617" />
          <!-- P3 右孔 -->
          <circle cx="62" cy="0" r="20" fill="#020617" />
          <!-- P5 下孔 -->
          <circle cx="0" cy="48" r="20" fill="#020617" />
          <!-- P4 左孔 -->
          <circle cx="-62" cy="0" r="20" fill="#020617" />
          <!-- 孔编号标签 -->
          <text x="24" y="6" fill="#2563eb" font-size="16" font-weight="bold">P1</text>
          <text x="24" y="-42" fill="#2563eb" font-size="16" font-weight="bold">P2</text>
          <text x="70" y="6" fill="#2563eb" font-size="16" font-weight="bold">P3</text>
          <text x="-58" y="6" fill="#2563eb" font-size="16" font-weight="bold">P4</text>
          <text x="24" y="54" fill="#2563eb" font-size="16" font-weight="bold">P5</text>
          <!-- 辅助说明 -->
          <text x="-86" y="-78" fill="#94a3b8" font-size="12" font-weight="600">P1 center</text>
          <text x="-86" y="100" fill="#94a3b8" font-size="12" font-weight="600">P2-P5 ring</text>
        </g>
      </svg>
    </div>

    <!-- 右侧：角度控制面板，固定宽度，不再覆盖 SVG -->
    <aside class="probe-sidebar w-[300px] shrink-0 p-5">
      <h3 class="probe-sidebar-title mb-4 border-b pb-3 text-base font-semibold">五孔探针角度示意</h3>

      <label class="mb-2 flex justify-between text-xs">
        <span>alpha: 偏航角（偏航面）</span>
        <span class="font-bold" style="color: #ffaa00">{{ alpha }} deg</span>
      </label>
      <UiSlider v-model="alpha" :min="-60" :max="60" class="mb-4 w-full" />

      <label class="mb-2 flex justify-between text-xs">
        <span>beta: 俯仰角（俯仰面）</span>
        <span class="font-bold" style="color: #ff00aa">{{ beta }} deg</span>
      </label>
      <UiSlider v-model="beta" :min="-60" :max="60" class="mb-4 w-full" />

      <div class="info-box space-y-1 rounded-xl p-3 text-xs leading-5">
        <div><span class="font-bold" style="color: #ffaa00">alpha+</span>: flow offset in the yaw plane; alpha- is the opposite direction.</div>
        <div><span class="font-bold" style="color: #ff00aa">beta+</span>: flow offset in the pitch plane; beta- is the opposite direction.</div>
        <div>The dashed line indicates incoming flow direction. Probe axis and -Z orientation are labeled in the diagram.</div>
      </div>
    </aside>
  </section>
</template>

<style scoped>
.probe-card {
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  min-height: 480px;
}

/* 右侧控制面板：使用项目 token，避免硬编码 slate 色与主题不协调 */
.probe-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel);
  color: var(--text-primary);
}

.probe-sidebar-title {
  color: var(--text-primary);
}

.probe-glow {
  background:
    radial-gradient(circle at 50% 48%, color-mix(in srgb, var(--color-accent) 20%, transparent) 0%, transparent 38%),
    radial-gradient(circle at 75% 75%, color-mix(in srgb, var(--color-info) 12%, transparent) 0%, transparent 32%);
}

.info-box {
  border: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel) 75%, transparent);
  color: var(--text-secondary);
}
</style>
