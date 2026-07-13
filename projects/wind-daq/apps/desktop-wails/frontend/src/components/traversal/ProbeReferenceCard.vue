<script setup lang="ts">
/**
 * 五孔探针角度参考图（对齐产品参考图风格）
 *
 * 孔序约定（与 core/calibration + fivehole interpolation 一致）：
 *   P1 = 下孔 · P2 = 中心孔 · P3 = 上孔 · P4 = 左孔 · P5 = 右孔
 *
 * 图面布局（仿参考图）：
 *   左：XYZ 坐标系 + 俯仰面 / 偏转面 + α/β 正负弧 + 来流
 *   右：立体锥头探针（含 L 形支杆）+ 端面孔序引线
 */
import { computed, ref } from 'vue'
import UiSlider from '@components/ui/UiSlider.vue'
import { useI18nStore } from '@stores/i18nStore'

const i18n = useI18nStore()
const t = computed(() => i18n.t)

const alpha = ref(25)
const beta = ref(20)

/** 数学角：0°=右，逆时针为正（SVG y 向下 → sin 取负） */
function polar(cx: number, cy: number, r: number, deg: number) {
  const rad = (deg * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy - r * Math.sin(rad) }
}

function arcPath(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
  const s = polar(cx, cy, r, startDeg)
  const e = polar(cx, cy, r, endDeg)
  const delta = endDeg - startDeg
  const large = Math.abs(delta) > 180 ? 1 : 0
  const sweep = delta >= 0 ? 0 : 1
  return `M ${s.x.toFixed(1)} ${s.y.toFixed(1)} A ${r} ${r} 0 ${large} ${sweep} ${e.x.toFixed(1)} ${e.y.toFixed(1)}`
}

function spanOf(v: number) {
  return 14 + (Math.min(Math.abs(v), 60) / 60) * 36
}

// ── 坐标系原点（参考图左侧三轴交点）──
const OX = 230
const OY = 290

// 偏转面（水平 yaw 面）上 α：基准沿 -Z（向左，180°）
// α+ 朝向图面“前下”一侧（顺时针视觉），α− 反侧
const ALPHA_R = 62
const ALPHA_BASE = 180 // 沿 -Z
const alphaSpan = computed(() => spanOf(alpha.value))
const alphaPlusArc = computed(() =>
  arcPath(OX, OY, ALPHA_R, ALPHA_BASE, ALPHA_BASE - alphaSpan.value),
)
const alphaMinusArc = computed(() =>
  arcPath(OX, OY, ALPHA_R, ALPHA_BASE, ALPHA_BASE + alphaSpan.value),
)
const alphaPlusLabel = computed(() =>
  polar(OX, OY, ALPHA_R + 16, ALPHA_BASE - alphaSpan.value * 0.55),
)
const alphaMinusLabel = computed(() =>
  polar(OX, OY, ALPHA_R + 16, ALPHA_BASE + alphaSpan.value * 0.55),
)

// 俯仰面（垂直 pitch 面）上 β：基准沿 -Z，β+ 向上，β− 向下
const BETA_R = 54
const BETA_BASE = 180
const betaSpan = computed(() => spanOf(beta.value))
const betaPlusArc = computed(() =>
  arcPath(OX, OY, BETA_R, BETA_BASE, BETA_BASE + betaSpan.value),
)
const betaMinusArc = computed(() =>
  arcPath(OX, OY, BETA_R, BETA_BASE, BETA_BASE - betaSpan.value),
)
const betaPlusLabel = computed(() =>
  polar(OX, OY, BETA_R + 16, BETA_BASE + betaSpan.value * 0.55),
)
const betaMinusLabel = computed(() =>
  polar(OX, OY, BETA_R + 16, BETA_BASE - betaSpan.value * 0.55),
)
</script>

<template>
  <section class="probe-card flex h-full min-h-[500px] overflow-hidden rounded-xl">
    <div class="relative flex-1 min-w-0">
      <div class="probe-glow absolute inset-0" />

      <svg
        viewBox="0 0 760 560"
        role="img"
        :aria-label="t.travProbeDiagramTitle"
        class="relative z-10 h-full w-full"
      >
        <defs>
          <!-- 箭头 -->
          <marker id="arr-x" markerWidth="10" markerHeight="10" refX="9" refY="5" orient="auto">
            <path d="M0,0 L10,5 L0,10 Z" fill="#e03131" />
          </marker>
          <marker id="arr-y" markerWidth="10" markerHeight="10" refX="9" refY="5" orient="auto">
            <path d="M0,0 L10,5 L0,10 Z" fill="#364fc7" />
          </marker>
          <marker id="arr-z" markerWidth="10" markerHeight="10" refX="9" refY="5" orient="auto">
            <path d="M0,0 L10,5 L0,10 Z" fill="#0ca678" />
          </marker>
          <marker id="arr-flow" markerWidth="10" markerHeight="10" refX="9" refY="5" orient="auto">
            <path d="M0,0 L10,5 L0,10 Z" fill="#c2783a" />
          </marker>
          <marker id="arr-a" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" fill="#e67700" />
          </marker>
          <marker id="arr-b" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
            <path d="M0,0 L8,4 L0,8 Z" fill="#c2255c" />
          </marker>

          <!-- 金属材质 -->
          <linearGradient id="metal-body" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stop-color="#f1f5f9" />
            <stop offset="35%" stop-color="#cbd5e1" />
            <stop offset="70%" stop-color="#94a3b8" />
            <stop offset="100%" stop-color="#e2e8f0" />
          </linearGradient>
          <linearGradient id="metal-side" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stop-color="#e2e8f0" />
            <stop offset="50%" stop-color="#94a3b8" />
            <stop offset="100%" stop-color="#64748b" />
          </linearGradient>
          <radialGradient id="metal-tip" cx="40%" cy="35%" r="65%">
            <stop offset="0%" stop-color="#f8fafc" />
            <stop offset="55%" stop-color="#cbd5e1" />
            <stop offset="100%" stop-color="#64748b" />
          </radialGradient>
          <linearGradient id="stem-grad" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stop-color="#94a3b8" />
            <stop offset="40%" stop-color="#e2e8f0" />
            <stop offset="100%" stop-color="#64748b" />
          </linearGradient>
          <filter id="soft" x="-15%" y="-15%" width="130%" height="130%">
            <feDropShadow dx="1" dy="3" stdDeviation="3.5" flood-color="#0f172a" flood-opacity="0.16" />
          </filter>
        </defs>

        <!-- ════════════ 左侧：坐标面 + 角度 ════════════ -->

        <!-- 偏转面（水平 yaw 面，透视平行四边形） -->
        <path
          d="M 70 310 L 230 290 L 390 330 L 230 350 Z"
          fill="#f8fafc"
          stroke="#94a3b8"
          stroke-width="1.2"
          opacity="0.85"
        />
        <text x="250" y="348" fill="#64748b" font-size="12" font-weight="600">
          {{ t.travProbeYawPlane }}
        </text>

        <!-- 俯仰面（垂直 pitch 面） -->
        <path
          d="M 140 160 L 230 150 L 230 290 L 140 300 Z"
          fill="#f1f5f9"
          stroke="#94a3b8"
          stroke-width="1.2"
          opacity="0.8"
        />
        <text x="148" y="190" fill="#64748b" font-size="12" font-weight="600">
          {{ t.travProbePitchPlane }}
        </text>

        <!-- 小参考长方体（参考图中心的 box） -->
        <g opacity="0.9">
          <!-- 底 -->
          <path d="M 200 300 L 260 290 L 300 310 L 240 320 Z" fill="#e2e8f0" stroke="#64748b" stroke-width="1" />
          <!-- 顶 -->
          <path d="M 200 260 L 260 250 L 300 270 L 240 280 Z" fill="#f8fafc" stroke="#64748b" stroke-width="1" />
          <!-- 左侧 -->
          <path d="M 200 260 L 200 300 L 240 320 L 240 280 Z" fill="#cbd5e1" stroke="#64748b" stroke-width="1" />
          <!-- 右侧 -->
          <path d="M 260 250 L 300 270 L 300 310 L 260 290 Z" fill="#94a3b8" stroke="#64748b" stroke-width="1" />
        </g>

        <!-- 坐标轴：原点 O -->
        <!-- Y ↑ 蓝 -->
        <line x1="230" y1="290" x2="230" y2="110" stroke="#364fc7" stroke-width="2.5" marker-end="url(#arr-y)" />
        <text x="240" y="108" fill="#364fc7" font-size="16" font-weight="700">Y</text>

        <!-- X ↘ 红（探针轴向方向） -->
        <line x1="230" y1="290" x2="380" y2="400" stroke="#e03131" stroke-width="2.5" marker-end="url(#arr-x)" />
        <text x="388" y="416" fill="#e03131" font-size="16" font-weight="700">X</text>

        <!-- −Z ← 绿 -->
        <line x1="230" y1="290" x2="70" y2="290" stroke="#0ca678" stroke-width="2.5" marker-end="url(#arr-z)" />
        <text x="48" y="286" fill="#0ca678" font-size="15" font-weight="700">−Z</text>

        <!-- 来流：沿 −Z→O 方向（从左指向右） -->
        <line
          x1="55" y1="250" x2="195" y2="275"
          stroke="#c2783a" stroke-width="2.2" stroke-dasharray="8 6"
          marker-end="url(#arr-flow)"
        />
        <line
          x1="70" y1="268" x2="200" y2="288"
          stroke="#c2783a" stroke-width="2.2" stroke-dasharray="8 6"
          marker-end="url(#arr-flow)"
        />
        <text x="88" y="242" fill="#c2783a" font-size="13" font-weight="700">
          {{ t.travProbeFlowDir }}
        </text>

        <!-- α 弧（偏转面 / 水平，橙色） -->
        <path
          :d="alphaPlusArc"
          fill="none"
          stroke="#e67700"
          stroke-width="2.5"
          marker-end="url(#arr-a)"
        />
        <path
          :d="alphaMinusArc"
          fill="none"
          stroke="#e67700"
          stroke-width="2.5"
          stroke-dasharray="5 4"
          marker-end="url(#arr-a)"
        />
        <text
          :x="alphaPlusLabel.x"
          :y="alphaPlusLabel.y"
          fill="#e67700"
          font-size="13"
          font-weight="700"
          text-anchor="middle"
        >α+</text>
        <text
          :x="alphaMinusLabel.x"
          :y="alphaMinusLabel.y"
          fill="#e67700"
          font-size="13"
          font-weight="700"
          text-anchor="middle"
        >α−</text>

        <!-- β 弧（俯仰面 / 垂直，品红） -->
        <path
          :d="betaPlusArc"
          fill="none"
          stroke="#c2255c"
          stroke-width="2.5"
          marker-end="url(#arr-b)"
        />
        <path
          :d="betaMinusArc"
          fill="none"
          stroke="#c2255c"
          stroke-width="2.5"
          stroke-dasharray="5 4"
          marker-end="url(#arr-b)"
        />
        <text
          :x="betaPlusLabel.x"
          :y="betaPlusLabel.y"
          fill="#c2255c"
          font-size="13"
          font-weight="700"
          text-anchor="middle"
        >β+</text>
        <text
          :x="betaMinusLabel.x"
          :y="betaMinusLabel.y"
          fill="#c2255c"
          font-size="13"
          font-weight="700"
          text-anchor="middle"
        >β−</text>

        <!-- ════════════ 右侧：立体五孔探针 ════════════ -->
        <g filter="url(#soft)">
          <!-- L 形支杆：垂直段 -->
          <path
            d="M 620 70
               C 635 70, 645 85, 645 110
               L 645 200
               C 645 220, 635 235, 610 240
               L 560 248
               C 545 250, 535 258, 530 272
               L 522 295
               C 518 308, 522 320, 535 328
               L 560 340"
            fill="none"
            stroke="url(#stem-grad)"
            stroke-width="28"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <!-- 支杆高光 -->
          <path
            d="M 625 80 L 635 120 L 635 195 L 608 235 L 560 244"
            fill="none"
            stroke="#f8fafc"
            stroke-width="5"
            stroke-linecap="round"
            opacity="0.45"
          />

          <!-- 探针套筒（圆柱体，透视） -->
          <ellipse cx="545" cy="348" rx="38" ry="22" fill="url(#metal-side)" stroke="#64748b" stroke-width="1" />
          <path
            d="M 507 348
               L 500 330
               C 500 318, 512 310, 528 308
               L 562 305
               C 580 303, 590 312, 590 324
               L 583 348
               Z"
            fill="url(#metal-body)"
            stroke="#64748b"
            stroke-width="1"
          />
          <ellipse cx="545" cy="328" rx="36" ry="18" fill="url(#metal-body)" stroke="#94a3b8" stroke-width="0.8" />

          <!-- 锥头主体（立体圆锥） -->
          <!-- 锥侧面阴影 -->
          <path
            d="M 470 340
               L 508 318
               C 522 312, 538 314, 548 324
               L 548 358
               C 538 368, 522 370, 508 364
               L 470 348
               Z"
            fill="url(#metal-side)"
            stroke="#64748b"
            stroke-width="1"
          />
          <!-- 锥头前端圆盘（端面略椭圆，略朝向观察者） -->
          <ellipse cx="468" cy="344" rx="28" ry="32" fill="url(#metal-tip)" stroke="#64748b" stroke-width="1.2" />
          <!-- 锥高光 -->
          <ellipse cx="460" cy="332" rx="10" ry="14" fill="#ffffff" opacity="0.35" />

          <!-- ── 端面五孔：正确孔序 ──
               观察方向：大致沿 +X 看向锥头端面（从前方看）
               P2 中心 · P3 上 · P1 下 · P4 左 · P5 右 -->
          <!-- P2 中心 -->
          <ellipse cx="468" cy="344" rx="5.5" ry="6" fill="#0f172a" />
          <!-- P3 上 -->
          <ellipse cx="468" cy="324" rx="5" ry="5.5" fill="#0f172a" />
          <!-- P1 下 -->
          <ellipse cx="468" cy="364" rx="5" ry="5.5" fill="#0f172a" />
          <!-- P4 左 -->
          <ellipse cx="450" cy="344" rx="5" ry="5.5" fill="#0f172a" />
          <!-- P5 右 -->
          <ellipse cx="486" cy="344" rx="5" ry="5.5" fill="#0f172a" />
        </g>

        <!-- 孔序引线（端面 → 标签，仿参考图） -->
        <!-- 左侧竖线参考 -->
        <line x1="420" y1="300" x2="420" y2="400" stroke="#334155" stroke-width="1.2" />
        <line x1="400" y1="300" x2="400" y2="400" stroke="#334155" stroke-width="1.2" opacity="0.35" />

        <!-- P3 上 -->
        <line x1="468" y1="324" x2="420" y2="308" stroke="#334155" stroke-width="1.1" />
        <text x="388" y="306" fill="#1e40af" font-size="13" font-weight="700" text-anchor="end">P3</text>
        <text x="388" y="318" fill="#64748b" font-size="10" text-anchor="end">{{ t.travProbePortTop }}</text>

        <!-- P2 中 -->
        <line x1="468" y1="344" x2="420" y2="344" stroke="#334155" stroke-width="1.1" />
        <text x="388" y="342" fill="#1e40af" font-size="13" font-weight="700" text-anchor="end">P2</text>
        <text x="388" y="354" fill="#64748b" font-size="10" text-anchor="end">{{ t.travProbePortCenter }}</text>

        <!-- P1 下 -->
        <line x1="468" y1="364" x2="420" y2="388" stroke="#334155" stroke-width="1.1" />
        <text x="388" y="392" fill="#1e40af" font-size="13" font-weight="700" text-anchor="end">P1</text>
        <text x="388" y="404" fill="#64748b" font-size="10" text-anchor="end">{{ t.travProbePortBottom }}</text>

        <!-- P4 / P5 右侧小注 -->
        <line x1="450" y1="344" x2="430" y2="360" stroke="#334155" stroke-width="1" opacity="0.7" />
        <text x="424" y="372" fill="#1e40af" font-size="11" font-weight="700" text-anchor="end">P4</text>
        <line x1="486" y1="344" x2="510" y2="360" stroke="#334155" stroke-width="1" opacity="0.7" />
        <text x="516" y="372" fill="#1e40af" font-size="11" font-weight="700">P5</text>

        <!-- 底部极简图例 -->
        <g transform="translate(36 512)">
          <rect x="0" y="-14" width="420" height="28" rx="8" fill="#ffffff" stroke="#e2e8f0" opacity="0.95" />
          <text x="12" y="4" fill="#64748b" font-size="11" font-weight="600">
            P2 center · P1 bottom · P3 top · P4 left · P5 right
          </text>
          <circle cx="300" cy="-1" r="4" fill="#e67700" />
          <text x="310" y="3" fill="#e67700" font-size="11" font-weight="600">α</text>
          <circle cx="340" cy="-1" r="4" fill="#c2255c" />
          <text x="350" y="3" fill="#c2255c" font-size="11" font-weight="600">β</text>
          <line x1="372" y1="-1" x2="396" y2="-1" stroke="#c2783a" stroke-width="2" stroke-dasharray="4 3" />
          <text x="400" y="3" fill="#c2783a" font-size="11" font-weight="600">flow</text>
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
          {{ t.travProbeAlphaYawPlane }}
        </span>
        <span class="font-bold tabular-nums" style="color:#e67700">{{ alpha }}°</span>
      </label>
      <UiSlider v-model="alpha" :min="-60" :max="60" class="mb-5 w-full" />

      <label class="mb-2 flex items-center justify-between text-xs">
        <span class="flex items-center gap-1.5">
          <span class="legend-dot" style="background:#c2255c" />
          {{ t.travProbeBetaPitchPlane }}
        </span>
        <span class="font-bold tabular-nums" style="color:#c2255c">{{ beta }}°</span>
      </label>
      <UiSlider v-model="beta" :min="-60" :max="60" class="mb-5 w-full" />

      <div class="info-box space-y-2.5 rounded-xl p-3.5 text-xs leading-5">
        <div class="info-row">
          <span class="info-key" style="color:#e67700">α+</span>
          <span>{{ t.travProbeAlphaPlusHint }}</span>
        </div>
        <div class="info-row">
          <span class="info-key" style="color:#c2255c">β+</span>
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
  min-height: 500px;
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
    radial-gradient(circle at 30% 45%, color-mix(in srgb, var(--color-accent) 10%, transparent) 0%, transparent 40%),
    radial-gradient(circle at 75% 55%, color-mix(in srgb, var(--color-info) 8%, transparent) 0%, transparent 35%);
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
  min-width: 1.5rem;
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
