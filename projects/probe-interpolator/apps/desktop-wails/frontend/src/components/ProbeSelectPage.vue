<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { SetActiveProbe, GetAvailableProbes, GetActiveProbe, type ProbeInfo, type ProbeKind } from '../probe-adapter'

// emits 通知父组件 App.vue：用户已选定探针类型，请切换到对应工作区。
const emit = defineEmits<{
  (e: 'selected', kind: ProbeKind): void
}>()

const probes = ref<ProbeInfo[]>([])
const errorMsg = ref('')
const selecting = ref(false)

onMounted(async () => {
  // 启动时拉取探针列表 + 检查是否已选择（理论上首次启动应是未选择状态）
  try {
    probes.value = await GetAvailableProbes()
  } catch (e) {
    errorMsg.value = `加载探针列表失败: ${e}`
  }

  // 若后端已设置探针类型（例如热重载后状态保留），直接通知父组件跳转
  // v0.1.1 起 GetActiveProbe 未选择时返回空字符串，按 if(kind) 判定留在选择页
  try {
    const kind = await GetActiveProbe()
    if (kind) {
      emit('selected', kind)
    }
  } catch {
    // 后端不可用时留在选择页
  }
})

async function onSelect(kind: ProbeKind) {
  if (selecting.value) return
  selecting.value = true
  errorMsg.value = ''
  try {
    await SetActiveProbe(kind)
    emit('selected', kind)
  } catch (e) {
    errorMsg.value = `${e}`
  } finally {
    selecting.value = false
  }
}
</script>

<template>
  <div class="select-page">
    <header class="hero">
      <h1>探针插值器</h1>
      <p>请选择本次使用的探针类型（可随时从工作区返回切换）</p>
    </header>

    <div v-if="errorMsg" class="error-banner">{{ errorMsg }}</div>

    <div class="probe-cards">
      <button
        v-for="probe in probes"
        :key="probe.kind"
        class="probe-card"
        :disabled="selecting"
        @click="onSelect(probe.kind)"
      >
        <div class="probe-holes">{{ probe.holes }} 孔</div>
        <h2>{{ probe.name }}</h2>
        <p>{{ probe.description }}</p>
      </button>
    </div>

    <footer class="footer">
      <span>v0.1.0</span>
    </footer>
  </div>
</template>

<style scoped>
.select-page {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: #f5f7fa;
  color: #1f2937;
  font-family: system-ui, -apple-system, 'Segoe UI', sans-serif;
}

.hero {
  padding: 3rem 2rem 2rem;
  text-align: center;
  background: linear-gradient(135deg, #1e3a5f 0%, #2c5282 100%);
  color: white;
}

.hero h1 {
  margin: 0 0 0.5rem 0;
  font-size: 2.25rem;
  font-weight: 700;
}

.hero p {
  margin: 0;
  color: #cbd5e1;
  font-size: 0.95rem;
}

.error-banner {
  margin: 1rem auto;
  padding: 0.75rem 1.25rem;
  max-width: 800px;
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 6px;
  color: #b91c1c;
  font-size: 0.9rem;
}

.probe-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 1.5rem;
  padding: 2rem;
  max-width: 1000px;
  margin: 0 auto;
  width: 100%;
}

.probe-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 2rem 1.5rem;
  background: white;
  border: 2px solid #e5e7eb;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
  text-align: center;
}

.probe-card:hover:not(:disabled) {
  border-color: #2c5282;
  box-shadow: 0 8px 16px rgba(44, 82, 130, 0.15);
  transform: translateY(-2px);
}

.probe-card:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.probe-holes {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  margin-bottom: 1rem;
  background: #ebf4ff;
  color: #2c5282;
  border-radius: 50%;
  font-size: 1rem;
  font-weight: 700;
}

.probe-card h2 {
  margin: 0 0 0.5rem 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: #1f2937;
}

.probe-card p {
  margin: 0;
  color: #6b7280;
  font-size: 0.875rem;
  line-height: 1.4;
}

.footer {
  margin-top: auto;
  padding: 1rem 2rem;
  text-align: center;
  color: #9ca3af;
  font-size: 0.8rem;
}
</style>
