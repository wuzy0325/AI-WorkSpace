<script setup lang="ts">
</script>

<template>
  <div class="log-view">
    <div class="log-view__head">
      <p class="eyebrow">Logs</p>
      <h2>运行日志</h2>
    </div>
    <div class="log-view__placeholder">
      <div class="log-line">
        <span class="log-time">--:--:--</span>
        <span class="log-level info">INFO</span>
        <span>Wind-DAQ UI initialized</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-view {
  padding: var(--space-4);
  min-height: 0;
  overflow-y: auto;
}

.log-view__head {
  margin-bottom: var(--space-4);
}

.log-view__head h2 {
  margin: 0;
  font-size: 1.35rem;
}

.log-view__placeholder {
  padding: var(--space-3);
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.2);
  font-family: var(--font-family-mono, monospace);
  font-size: 0.78rem;
}

.log-line {
  display: flex;
  gap: var(--space-3);
  padding: 0.3rem 0;
}

.log-time {
  color: var(--text-muted);
  flex-shrink: 0;
}

.log-level {
  width: 44px;
  flex-shrink: 0;
  font-weight: 700;
  text-transform: uppercase;
}

.log-level.info {
  color: var(--accent-primary);
}
</style>
