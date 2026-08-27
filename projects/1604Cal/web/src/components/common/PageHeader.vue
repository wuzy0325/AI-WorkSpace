<template>
  <header
    class="page-header"
    :class="{ 'has-border': showBorder }"
  >
    <div class="header-left">
      <button 
        v-if="showBack" 
        class="back-btn" 
        :title="backText"
        @click="handleBack"
      >
        <el-icon><ArrowLeft /></el-icon>
      </button>
      <div class="header-title">
        <h1>{{ title }}</h1>
        <p v-if="subtitle">
          {{ subtitle }}
        </p>
      </div>
    </div>
    <div class="header-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'

interface Props {
  /** 页面标题 */
  title: string
  /** 页面副标题 */
  subtitle?: string
  /** 是否显示返回按钮 */
  showBack?: boolean
  /** 返回按钮文字提示 */
  backText?: string
  /** 自定义返回路径，默认返回首页 */
  backPath?: string
  /** 是否显示底部边框 */
  showBorder?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  subtitle: '',
  showBack: true,
  backText: '返回',
  backPath: '/',
  showBorder: true
})

const emit = defineEmits<{
  back: []
}>()

const router = useRouter()

function handleBack(): void {
  if (props.backPath) {
    router.push(props.backPath)
  } else {
    emit('back')
  }
}
</script>

<style scoped lang="scss">
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  flex-shrink: 0;
  padding-bottom: $spacing-4;
  
  &.has-border {
    border-bottom: 1px solid $border-color-light;
    margin-bottom: $spacing-6;
  }
}

.header-left {
  display: flex;
  align-items: center;
  gap: $spacing-4;
}

.back-btn {
  width: 40px;
  height: 40px;
  background: rgba($neutral-800, 0.6);
  border: 1px solid $border-color;
  border-radius: $radius-md;
  color: $text-secondary;
  cursor: pointer;
  transition: all $transition-fast;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;

  &:hover {
    background: rgba($neutral-700, 0.8);
    color: $text-primary;
    border-color: $border-color-strong;
  }
}

.header-title {
  h1 {
    font-size: 28px;
    font-weight: $font-weight-bold;
    color: $text-primary;
    margin: 0 0 $spacing-1;
    letter-spacing: -0.02em;
    line-height: 1.2;
  }

  p {
    font-size: $font-size-sm;
    color: $text-secondary;
    margin: 0;
    line-height: 1.5;
  }
}

.header-actions {
  display: flex;
  align-items: center;
  gap: $spacing-3;
}

// 响应式适配
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: $spacing-4;
    
    &.has-border {
      margin-bottom: $spacing-4;
    }
  }

  .header-left {
    width: 100%;
  }

  .header-title h1 {
    font-size: $font-size-xl;
  }
  
  .header-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
