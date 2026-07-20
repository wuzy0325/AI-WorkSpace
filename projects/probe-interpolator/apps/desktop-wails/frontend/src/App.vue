<script setup lang="ts">
import { ref, onMounted, shallowRef } from 'vue'
import ProbeSelectPage from './components/ProbeSelectPage.vue'
import { GetActiveProbe, ClearActiveProbe, type ProbeKind } from './wails-adapter'

// activeProbe 为 null 表示仍在选择页，非 null 表示已进入工作区。
const activeProbe = ref<ProbeKind | null>(null)

// 工作区组件动态加载，避免启动时一次性加载三个大组件。
// 用 shallowRef 而非 ref：工作区组件是大组件，shallowRef 避免深度响应开销。
const workspaceComponent = shallowRef<any>(null)

async function loadWorkspace(kind: ProbeKind) {
  // 按探针类型动态 import 对应工作区组件：
  //   - five  → FiveHoleWorkspace.vue（Task 3 实现）
  //   - three → ThreeHoleWorkspace.vue（Task 4 实现）
  //   - seven → SevenHoleWorkspace.vue（Task 5 实现）
  // 用动态 import 而非静态 import：避免启动时一次性加载三个大组件，仅加载用户选中的那个。
  let mod: any
  if (kind === 'five') {
    mod = await import('./components/FiveHoleWorkspace.vue')
  } else if (kind === 'three') {
    mod = await import('./components/ThreeHoleWorkspace.vue')
  } else {
    mod = await import('./components/SevenHoleWorkspace.vue')
  }
  workspaceComponent.value = mod.default
}

function onProbeSelected(kind: ProbeKind) {
  activeProbe.value = kind
  loadWorkspace(kind)
}

// onBackToSelect 由工作区顶栏"返回"按钮触发：
//   1. 调后端 ClearActiveProbe 清空激活状态（GetActiveProbe 此后返回空字符串）
//   2. 前端 activeProbe 置 null，ProbeSelectPage 重新挂载
//   3. workspaceComponent 置 null，工作区组件卸载（.prb 等后端 state 保留，下次进入自动恢复）
async function onBackToSelect() {
  try {
    await ClearActiveProbe()
  } catch {
    // 即使后端清空失败，前端也允许返回（避免用户被困在工作区）
  }
  activeProbe.value = null
  workspaceComponent.value = null
}

onMounted(async () => {
  // 热重载后恢复：若后端已设置探针类型，直接进入对应工作区
  try {
    const kind = await GetActiveProbe()
    if (kind) {
      onProbeSelected(kind)
    }
  } catch {
    // 未选择是正常状态，留在选择页
  }
})
</script>

<template>
  <ProbeSelectPage v-if="!activeProbe" @selected="onProbeSelected" />
  <component v-else :is="workspaceComponent" :kind="activeProbe" @back="onBackToSelect" />
</template>

<style>
/* 全局基础样式，工作区组件自带 scoped 样式覆盖各自区域 */
* {
  box-sizing: border-box;
}

html,
body,
#app {
  margin: 0;
  padding: 0;
  height: 100%;
  font-family: system-ui, -apple-system, 'Segoe UI', sans-serif;
}
</style>
