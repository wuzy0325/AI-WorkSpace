<script setup lang="ts">
import { computed, ref } from 'vue'

const alpha = ref(25)
const beta = ref(20)

const alphaArcEnd = computed(() => ({
  x: 128 + alpha.value * 0.9,
  y: 320 - Math.abs(alpha.value) * 0.45
}))

const betaArcEnd = computed(() => ({
  x: 238 + beta.value * 0.45,
  y: 228 - beta.value * 0.85
}))
</script>

<template>
  <section class="relative flex h-full min-h-[560px] overflow-hidden rounded-xl border border-slate-200 bg-[#020617] dark:border-slate-700">
    <div class="absolute inset-0 bg-[radial-gradient(circle_at_45%_38%,rgba(37,99,235,0.28),transparent_34%),radial-gradient(circle_at_70%_70%,rgba(217,70,239,0.15),transparent_30%)]"></div>

    <svg viewBox="0 0 920 560" role="img" aria-label="五孔探针角度示意" class="relative z-10 h-full min-h-[560px] w-full">
      <defs>
        <marker id="flow-arrow" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#d48b52" />
        </marker>
        <marker id="axis-arrow-blue" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#3355ff" />
        </marker>
        <marker id="axis-arrow-green" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#10b981" />
        </marker>
        <marker id="axis-arrow-red" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#ef4444" />
        </marker>
        <marker id="alpha-arrow" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#ffaa00" />
        </marker>
        <marker id="beta-arrow" markerWidth="14" markerHeight="14" refX="12" refY="7" orient="auto">
          <path d="M0,0 L14,7 L0,14 Z" fill="#ff00aa" />
        </marker>
        <linearGradient id="probe-body" x1="0" x2="1">
          <stop offset="0%" stop-color="#f8fafc" />
          <stop offset="45%" stop-color="#94a3b8" />
          <stop offset="100%" stop-color="#e2e8f0" />
        </linearGradient>
      </defs>

      <path d="M70 430 L160 475 L278 460 L405 394 L560 410 L824 330" fill="none" stroke="#334155" stroke-width="3" opacity="0.55" />
      <path d="M180 450 L330 450 L500 425 L725 425" fill="none" stroke="#475569" stroke-width="3" opacity="0.35" />

      <line x1="120" y1="300" x2="780" y2="300" stroke="#d48b52" stroke-width="4" stroke-dasharray="14 12" marker-end="url(#flow-arrow)" />
      <text x="190" y="278" fill="#d48b52" class="text-3xl font-bold">流向</text>

      <line x1="450" y1="390" x2="450" y2="150" stroke="#3355ff" stroke-width="6" marker-end="url(#axis-arrow-blue)" />
      <text x="472" y="178" fill="#3355ff" class="text-4xl font-bold">Y</text>
      <line x1="450" y1="390" x2="610" y2="445" stroke="#10b981" stroke-width="5" marker-end="url(#axis-arrow-green)" />
      <text x="608" y="468" fill="#10b981" class="text-2xl font-bold">-Z</text>
      <line x1="450" y1="390" x2="340" y2="445" stroke="#ef4444" stroke-width="5" marker-end="url(#axis-arrow-red)" />
      <text x="304" y="468" fill="#ef4444" class="text-2xl font-bold">X probe axis</text>

      <path
        :d="`M130 420 C150 ${360 - Math.abs(alpha) * 0.4}, 195 ${338 - Math.abs(alpha) * 0.35}, ${alphaArcEnd.x} ${alphaArcEnd.y}`"
        fill="none"
        stroke="#ffaa00"
        stroke-width="7"
        marker-end="url(#alpha-arrow)"
      />
      <path
        :d="`M230 418 C205 ${360 + Math.abs(alpha) * 0.25}, 180 ${350 + Math.abs(alpha) * 0.35}, ${170 - alpha * 0.45} ${322 + Math.abs(alpha) * 0.42}`"
        fill="none"
        stroke="#ffaa00"
        stroke-width="7"
        stroke-dasharray="12 10"
        marker-end="url(#alpha-arrow)"
      />
      <text x="255" y="424" fill="#ffaa00" class="text-4xl font-bold">alpha+</text>
      <text x="255" y="488" fill="#ffaa00" class="text-3xl font-bold">alpha-</text>
      <text x="570" y="96" fill="#ffaa00" class="text-3xl font-bold">alpha yaw</text>

      <path
        :d="`M398 318 C350 310, 344 254, ${betaArcEnd.x} ${betaArcEnd.y}`"
        fill="none"
        stroke="#ff00aa"
        stroke-width="7"
        marker-end="url(#beta-arrow)"
      />
      <path
        :d="`M398 338 C354 354, 350 405, ${248 - beta * 0.35} ${420 + Math.abs(beta) * 0.25}`"
        fill="none"
        stroke="#ff00aa"
        stroke-width="7"
        stroke-dasharray="12 10"
        marker-end="url(#beta-arrow)"
      />
      <text x="358" y="286" fill="#ff00aa" class="text-4xl font-bold">beta+</text>
      <text x="358" y="404" fill="#ff00aa" class="text-4xl font-bold">beta-</text>
      <text x="570" y="136" fill="#ff00aa" class="text-3xl font-bold">beta pitch</text>

      <g transform="translate(565 300)">
        <ellipse cx="0" cy="0" rx="138" ry="86" fill="url(#probe-body)" opacity="0.98" />
        <ellipse cx="0" cy="0" rx="92" ry="56" fill="#dbe4ee" opacity="0.96" />
        <circle cx="0" cy="0" r="27" fill="#020617" />
        <circle cx="0" cy="-62" r="27" fill="#020617" />
        <circle cx="82" cy="0" r="27" fill="#020617" />
        <circle cx="0" cy="62" r="27" fill="#020617" />
        <circle cx="-82" cy="0" r="27" fill="#020617" />
        <text x="34" y="8" fill="#2563eb" class="text-3xl font-bold">P1</text>
        <text x="34" y="-50" fill="#2563eb" class="text-3xl font-bold">P2</text>
        <text x="116" y="8" fill="#2563eb" class="text-3xl font-bold">P3</text>
        <text x="-50" y="8" fill="#2563eb" class="text-3xl font-bold">P4</text>
        <text x="34" y="72" fill="#2563eb" class="text-3xl font-bold">P5</text>
        <text x="-62" y="-66" fill="#94a3b8" class="text-2xl font-semibold">P1 center</text>
        <text x="-68" y="124" fill="#94a3b8" class="text-2xl font-semibold">P2-P5 ring</text>
      </g>
    </svg>

    <aside class="absolute right-5 top-5 z-20 w-[300px] rounded-2xl border border-white/10 bg-slate-950/85 p-5 text-slate-100 shadow-2xl backdrop-blur">
      <h3 class="mb-4 border-b border-white/10 pb-3 text-base font-semibold text-slate-100">五孔探针角度示意</h3>
      <label class="mb-2 flex justify-between text-xs text-slate-300">
        <span>alpha: 偏航角（偏航面）</span>
        <span class="font-bold text-[#ffaa00]">{{ alpha }} deg</span>
      </label>
      <input v-model.number="alpha" type="range" min="-60" max="60" class="mb-4 w-full accent-amber-400" />

      <label class="mb-2 flex justify-between text-xs text-slate-300">
        <span>beta: 俯仰角（俯仰面）</span>
        <span class="font-bold text-[#ff00aa]">{{ beta }} deg</span>
      </label>
      <input v-model.number="beta" type="range" min="-60" max="60" class="mb-4 w-full accent-fuchsia-500" />

      <div class="space-y-1 rounded-xl border border-slate-700 bg-slate-900/75 p-3 text-xs leading-5 text-slate-300">
        <div><span class="font-bold text-amber-400">alpha+</span>: flow offset in the yaw plane; alpha- is the opposite direction.</div>
        <div><span class="font-bold text-fuchsia-400">beta+</span>: flow offset in the pitch plane; beta- is the opposite direction.</div>
        <div>The dashed line indicates incoming flow direction. Probe axis and -Z orientation are labeled in the diagram.</div>
      </div>
    </aside>
  </section>
</template>

