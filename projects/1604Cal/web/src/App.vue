<template>
  <el-config-provider :locale="zhCn">
    <div class="app-shell">
      <div class="app-viewport">
        <RouterView />
      </div>
      <FloatingLogPanel />
    </div>
  </el-config-provider>
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import { ElConfigProvider } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import FloatingLogPanel from '@/components/common/FloatingLogPanel.vue'
</script>

<style scoped lang="scss">
.app-shell {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.app-viewport {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
