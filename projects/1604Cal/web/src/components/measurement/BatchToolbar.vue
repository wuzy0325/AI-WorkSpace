<template>
  <!--
    分批模式顶部工具条。
    职责：阶段指示 + 通道入口 + 退出分批。
    所有分批阶段（range-input / group-view / batch-running / completed / report）统一渲染此工具条，
    避免某些阶段不渲染 MeasurementControl 导致用户卡死无法退出。

    布局原则：
      - 一行内放下所有工作台级控件，不再为分批开关单独占行
      - 阶段指示器 flex:1 占满中段空间
      - 通道入口与退出按钮固定在右侧
  -->
  <div class="batch-toolbar">
    <div class="toolbar-left">
      <span class="mode-tag">分批模式</span>
      <StepIndicator
        :steps="steps"
        :current-key="currentKey"
        :done-keys="doneKeys"
        aria-label="分批计量阶段"
      />
    </div>

    <div class="toolbar-right">
      <button
        type="button"
        class="channel-entry"
        @click="$emit('open-channel-dialog')"
      >
        <el-icon><Grid /></el-icon>
        <span>通道 {{ channelCount }}/16</span>
      </button>
      <button
        type="button"
        class="exit-btn"
        @click="$emit('exit-batch')"
      >
        <el-icon><CloseBold /></el-icon>
        <span>退出分批</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Grid, CloseBold } from '@element-plus/icons-vue'
import StepIndicator from '@/components/common/StepIndicator.vue'

defineProps<{
  /** 阶段步骤列表 */
  steps: Array<{ key: string; label: string; index: number }>
  /** 当前阶段 key */
  currentKey: string
  /** 已完成阶段 key 列表 */
  doneKeys: string[]
  /** 已选通道数量 */
  channelCount: number
}>()

defineEmits<{
  /** 退出分批模式 */
  'exit-batch': []
  /** 打开通道选择弹窗 */
  'open-channel-dialog': []
}>()
</script>

<style scoped lang="scss">
.batch-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  flex-shrink: 0;
  font-family: $font-sans;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

/* "分批模式"标签：弱化背景，与阶段指示器视觉分组 */
.mode-tag {
  display: inline-flex;
  align-items: center;
  padding: 3px 10px;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  color: $mint-dark;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  flex-shrink: 0;
}

/* 通道入口：与常规模式 MeasurementControl 中的 channel-select-btn 视觉一致 */
.channel-entry {
  height: 28px;
  padding: 0 10px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  background: $slate-50;
  color: $blue;
  font-size: 12px;
  font-family: $font-mono;
  font-weight: 500;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.15s ease;

  &:hover {
    background: $slate-100;
    border-color: $slate-300;
  }

  .el-icon {
    font-size: 12px;
    color: $slate-400;
  }
}

/* 退出分批按钮：危险色调，提示退出会清空数据 */
.exit-btn {
  height: 28px;
  padding: 0 12px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  background: #fff;
  color: $slate-600;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.15s ease;

  &:hover {
    border-color: $red;
    color: $red;
    background: rgba(239, 68, 68, 0.06);
  }

  &:active {
    transform: translateY(1px);
  }

  .el-icon {
    font-size: 12px;
  }
}
</style>
