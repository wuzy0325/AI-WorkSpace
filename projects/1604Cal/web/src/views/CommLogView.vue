<template>
  <PageLayout>
    <div class="comm-log-page">
      <PageHeader
        title="系统日志"
        subtitle="硬件通讯日志与系统错误日志"
      />
      <CommLogPanel />
    </div>
  </PageLayout>
</template>

<script setup lang="ts">
import PageLayout from '@/components/common/PageLayout.vue'
import PageHeader from '@/components/common/PageHeader.vue'
import CommLogPanel from '@/components/common/CommLogPanel.vue'
</script>

<style scoped lang="scss">
.comm-log-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 16px 20px;
  gap: 12px;
}
</style>
