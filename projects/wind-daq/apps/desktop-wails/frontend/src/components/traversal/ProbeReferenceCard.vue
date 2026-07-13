<script setup lang="ts">
/**
 * 五孔探针参考图 —— 以「表达清楚」为第一目标
 *
 * 孔序（与 core/calibration + fivehole interpolation 一致）：
 *   P1 下 · P2 中心 · P3 上 · P4 左 · P5 右
 *
 * 布局（单一示意图，元素互不重叠）：
 *   中心：端面 + 五孔（孔序正确）
 *   下方：α 偏航角（橙弧，随滑块变大）
 *   右侧：β 俯仰角（粉弧，随滑块变大）
 *   顶部：来流方向（棕色虚线）
 *   左下：X / Y / −Z 坐标 triad
 *   侧栏：滑块 + 当前角度 + 说明
 */
import { computed, ref } from 'vue'
import UiSlider from '@components/ui/UiSlider.vue'
import { useI18nStore } from '@stores/i18nStore'

const i18n = useI18nStore()
const t = computed(() => i18n.t)

const alpha = ref(25)
const beta = ref(20)

// 角度制极坐标：deg=0 右，90 上，180 左，270 下（SVG y 轴向下）
function polar(cx: number, cy: number, r: number, deg: number) {
  const rad = (deg * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy - r * Math.sin(rad) }
}

function arcPath(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
  const s = polar(cx, cy, r, startDeg)
  const e = polar(cx, cy, r, endDeg)
  const delta = endDeg - startDeg
  const large = Math.abs(delta) > 180 ? 1 : 0
  const sweep = delta >= 0 ? 1 : 0
  return `M ${s.x.toFixed(1)} ${s.y.toFixed(1)} A ${r} ${r} 0 ${large} ${sweep} ${e.x.toFixed(1)} ${e.y.toFixed(1)}`
}

// 弧跨度随滑块增大；标签固定在安全位置，不随弧跑
function spanOf(v: number) {
  return 16 + (Math.min(Math.abs(v), 60) / 60) * 44
}

// ── 端面中心 ──
const CX = 250
const CY = 235
const FACE_RO = 74
const FACE_RI = 58
const PORT_OFF = 42

const aSpan = computed(() => spanOf(alpha.value))
const bSpan = computed(() => spanOf(beta.value))

// α 在下方（270° 附近）
const alphaArc = computed(() => arcPath(CX, CY, 116, 270 - aSpan.value / 2, 270 + aSpan.value / 2))
// β 在右侧（0° 附近）
const betaArc = computed(() => arcPath(CX, CY, 98, -bSpan.value / 2, bSpan.value / 2))

const ports = [
  { id: 'P2', dx: 0, dy: 0, inside: true },
  { id: 'P3', dx: 0, dy: -PORT_OFF, inside: false },
  { id: 'P1', dx: 0, dy: PORT_OFF, inside: false },
  { id: 'P4', dx: -PORT_OFF, dy: 0, inside: false },
  { id: 'P5', dx: PORT_OFF, dy: 0, inside: false },
] as const

// 孔标签白底小牌位置（远离弧与坐标，保证可读）
const portLabelPos: Record<string, { x: number; y: number; anchor: 'start' | 'middle' | 'end' }> = {
  P3: { x: CX, y: 145, anchor: 'middle' },
  P1: { x: CX, y: 332, anchor: 'middle' },
  P4: { x: CX - PORT_OFF - 40, y: CY + 4, anchor: 'end' },
  P5: { x: 360, y: CY + 4, anchor: 'start' },
}

// 标签白底 rect 左上角 x：让 rect 相对锚点居中
// - middle：rect 中心 = pos.x  → rect.x = pos.x - 16
// - end：rect 右边 = pos.x     → rect.x = pos.x - 32
// - start：rect 左边 = pos.x   → rect.x = pos.x
function portLabelRectX(id: string): number {
  const p = portLabelPos[id]
  if (p.anchor === 'end') return p.x - 32
  if (p.anchor === 'start') return p.x
  return p.x - 16
}

// 标签 text x：在 SVG 中 text 锚点由 text-anchor 决定
// - middle：text 中心 = pos.x
// - end：text 右边 = pos.x（贴 rect 右内边）
// - start：text 左边 = pos.x（贴 rect 左内边）
function portLabelTextX(id: string): number {
  return portLabelPos[id].x
}
</script>

<template>
  <section class="probe-card flex h-full min-h-[480px] overflow-hidden rounded-xl">
    <!-- 主图 -->
    <div class="relative flex-1 min-w-0">
      <div class="probe-glow absolute inset-0" />

      <svg
        viewBox="0 0 560 470"
        role="img"
        :aria-label="t.travProbeDiagramTitle"
        class="relative z-10 h-full w-full"
      >
        <defs>
          <marker id="mk-flow" markerWidth="10" markerHeight="10" refX="5" refY="5" orient="auto">
            <path d="M0,0 L10,5 L0,10 Z" fill="#b45309" />
          </marker>
          <marker id="mk-a" markerWidth="9" markerHeight="9" refX="4.5" refY="4.5" orient="auto">
            <path d="M0,0 L9,4.5 L0,9 Z" fill="#e67700" />
          </marker>
          <marker id="mk-b" markerWidth="9" markerHeight="9" refX="4.5" refY="4.5" orient="auto">
            <path d="M0,0 L9,4.5 L0,9 Z" fill="#c2255c" />
          </marker>
          <marker id="mk-x" markerWidth="9" markerHeight="9" refX="8" refY="4.5" orient="auto">
            <path d="M0,0 L9,4.5 L0,9 Z" fill="#e03131" />
          </marker>
          <marker id="mk-y" markerWidth="9" markerHeight="9" refX="8" refY="4.5" orient="auto">
            <path d="M0,0 L9,4.5 L0,9 Z" fill="#364fc7" />
          </marker>
          <marker id="mk-z" markerWidth="9" markerHeight="9" refX="8" refY="4.5" orient="auto">
            <path d="M0,0 L9,4.5 L0,9 Z" fill="#0ca678" />
          </marker>
          <radialGradient id="face-metal" cx="38%" cy="34%" r="75%">
            <stop offset="0%" stop-color="#f8fafc" />
            <stop offset="45%" stop-color="#cbd5e1" />
            <stop offset="100%" stop-color="#94a3b8" />
          </radialGradient>
          <filter id="soft" x="-30%" y="-30%" width="160%" height="160%">
            <feDropShadow dx="1.5" dy="3" stdDeviation="3" flood-color="#0f172a" flood-opacity="0.16" />
          </filter>
        </defs>

        <!-- 探针轴（参考虚线，置于底层） -->
        <line :x1="CX" :y1="CY" :x2="CX" :y2="330" stroke="#94a3b8" stroke-width="1.4" stroke-dasharray="5 4" />
        <text :x="CX + 8" :y="252" fill="#64748b" font-size="10" font-weight="600">
          {{ t.travProbeAxisLabel }}
        </text>

        <!-- 来流（顶部，棕色虚线，指向端面） -->
        <line x1="250" y1="38" x2="250" y2="112" stroke="#b45309" stroke-width="2.2" stroke-dasharray="8 5" marker-end="url(#mk-flow)" />
        <text x="250" y="28" fill="#b45309" font-size="12" font-weight="700" text-anchor="middle">
          {{ t.travProbeFlowDir }}
        </text>

        <!-- 端面（金属质感 + 阴影，体现立体） -->
        <g filter="url(#soft)">
          <circle :cx="CX" :cy="CY" :r="FACE_RO" fill="url(#face-metal)" stroke="#64748b" stroke-width="1.5" />
          <circle :cx="CX" :cy="CY" :r="FACE_RI" fill="#e2e8f0" stroke="#94a3b8" stroke-width="1" />
          <!-- 左上高光，强化球面感 -->
          <path d="M 212 200 A 74 74 0 0 1 285 192" fill="none" stroke="#ffffff" stroke-width="3" stroke-linecap="round" opacity="0.5" />
          <!-- 五孔 -->
          <template v-for="p in ports" :key="p.id">
            <circle
              :cx="CX + p.dx"
              :cy="CY + p.dy"
              :r="p.id === 'P2' ? 13 : 11"
              fill="#0f172a"
            />
            <circle :cx="CX + p.dx - 2" :cy="CY + p.dy - 2" r="2.5" fill="#ffffff" opacity="0.25" />
          </template>
        </g>

        <!-- 孔标签（白底小牌，永远可读、不互相压） -->
        <template v-for="p in ports" :key="'lbl-' + p.id">
          <g v-if="!p.inside">
            <rect
              :x="portLabelRectX(p.id)"
              :y="portLabelPos[p.id].y - 13"
              width="32"
              height="20"
              rx="5"
              fill="#ffffff"
              stroke="#cbd5e1"
            />
            <text
              :x="portLabelTextX(p.id)"
              :y="portLabelPos[p.id].y + 3"
              fill="#1e40af"
              font-size="13"
              font-weight="700"
              :text-anchor="portLabelPos[p.id].anchor"
            >
              {{ p.id }}
            </text>
          </g>
          <text v-else :x="CX" :y="CY + 4" fill="#ffffff" font-size="12" font-weight="700" text-anchor="middle">
            P2
          </text>
        </template>

        <!-- α 弧（橙，下方） + 读数牌 -->
        <path :d="alphaArc" fill="none" stroke="#e67700" stroke-width="3" marker-end="url(#mk-a)" />
        <g>
          <rect x="214" y="366" width="72" height="22" rx="6" fill="#fff7ed" stroke="#e67700" />
          <text x="250" y="381" fill="#e67700" font-size="13" font-weight="700" text-anchor="middle">
            α = {{ alpha }}°
          </text>
        </g>

        <!-- β 弧（粉，右侧） + 读数牌 -->
        <path :d="betaArc" fill="none" stroke="#c2255c" stroke-width="3" marker-end="url(#mk-b)" />
        <g>
          <rect x="372" y="224" width="72" height="22" rx="6" fill="#fff0f6" stroke="#c2255c" />
          <text x="408" y="239" fill="#c2255c" font-size="13" font-weight="700" text-anchor="middle">
            β = {{ beta }}°
          </text>
        </g>

        <!-- 坐标 triad（左下角，独立不重叠） -->
        <g>
          <line x1="64" y1="424" x2="138" y2="424" stroke="#e03131" stroke-width="2.4" marker-end="url(#mk-x)" />
          <text x="144" y="428" fill="#e03131" font-size="14" font-weight="700">X</text>
          <line x1="64" y1="424" x2="64" y2="358" stroke="#364fc7" stroke-width="2.4" marker-end="url(#mk-y)" />
          <text x="68" y="352" fill="#364fc7" font-size="14" font-weight="700">Y</text>
          <line x1="64" y1="424" x2="24" y2="386" stroke="#0ca678" stroke-width="2.4" marker-end="url(#mk-z)" />
          <text x="6" y="384" fill="#0ca678" font-size="13" font-weight="700">−Z</text>
        </g>
      </svg>
    </div>

    <!-- 右侧控制 -->
    <aside class="probe-sidebar w-[300px] shrink-0 p-5">
      <h3 class="probe-sidebar-title mb-1 border-b pb-3 text-base font-semibold">
        {{ t.travProbeDiagramTitle }}
      </h3>
      <p class="mb-4 text-[11px] leading-relaxed text-[var(--text-muted)]">
        {{ t.travProbeDiagramHint }}
      </p>

      <label class="mb-2 flex items-center justify-between text-xs">
        <span class="flex items-center gap-1.5">
          <span class="legend-dot" style="background:#e67700" />
          {{ t.travProbeAlphaYaw }}
        </span>
        <span class="font-bold tabular-nums" style="color:#e67700">{{ alpha }}°</span>
      </label>
      <UiSlider v-model="alpha" :min="-60" :max="60" class="mb-5 w-full" />

      <label class="mb-2 flex items-center justify-between text-xs">
        <span class="flex items-center gap-1.5">
          <span class="legend-dot" style="background:#c2255c" />
          {{ t.travProbeBetaPitch }}
        </span>
        <span class="font-bold tabular-nums" style="color:#c2255c">{{ beta }}°</span>
      </label>
      <UiSlider v-model="beta" :min="-60" :max="60" class="mb-5 w-full" />

      <div class="info-box space-y-2.5 rounded-xl p-3.5 text-xs leading-5">
        <div class="info-row">
          <span class="info-key" style="color:#e67700">α</span>
          <span>{{ t.travProbeAlphaPlusHint }}</span>
        </div>
        <div class="info-row">
          <span class="info-key" style="color:#c2255c">β</span>
          <span>{{ t.travProbeBetaPlusHint }}</span>
        </div>
        <div class="info-row info-row--muted">
          <span>{{ t.travProbeDashedHint }}</span>
        </div>
        <div class="info-row info-row--muted">
          <span>{{ t.travProbePortsHint }}</span>
        </div>
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
.probe-sidebar {
  border-left: 1px solid var(--border-default);
  background: var(--bg-panel);
  color: var(--text-primary);
}
.probe-sidebar-title {
  color: var(--text-primary);
  border-color: var(--border-default);
}
.probe-glow {
  background:
    radial-gradient(circle at 30% 42%, color-mix(in srgb, var(--color-accent) 8%, transparent) 0%, transparent 42%),
    radial-gradient(circle at 70% 50%, color-mix(in srgb, var(--color-info) 7%, transparent) 0%, transparent 40%);
}
.info-box {
  border: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel) 80%, var(--bg-canvas));
  color: var(--text-secondary);
}
.info-row {
  display: flex;
  gap: 0.5rem;
  align-items: flex-start;
}
.info-key {
  flex-shrink: 0;
  font-weight: 700;
  min-width: 1.25rem;
}
.info-row--muted {
  color: var(--text-muted);
  padding-top: 0.25rem;
  border-top: 1px dashed var(--border-default);
}
.legend-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  flex-shrink: 0;
}
</style>
