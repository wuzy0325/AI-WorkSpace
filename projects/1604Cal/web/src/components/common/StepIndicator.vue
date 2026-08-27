<template>
  <nav
    class="step-indicator"
    :class="[`variant-${variant}`, { 'is-idle': showIdleHint }]"
    :aria-label="ariaLabel"
  >
    <ol class="step-list">
      <!-- idle 态：仅渲染第 1 步 active + 提示文字。
           此分支用于"流程尚未开始"的视觉提示，避免用户误以为流程已完成。
           与运行态分支分离，可让 idle 视觉更克制（不渲染后续 pending 步骤）。 -->
      <template v-if="showIdleHint && steps.length > 0">
        <li
          class="step-item active idle"
          aria-current="step"
        >
          <span class="step-marker">
            <span class="step-number">1</span>
          </span>
          <span class="step-label">{{ steps[0].label }}</span>
          <span class="step-idle-hint">{{ idleHintText }}</span>
        </li>
      </template>
      <!-- 运行态：渲染所有步骤，按 currentKey / doneKeys 决定状态 -->
      <template v-else>
        <li
          v-for="(step, i) in steps"
          :key="step.key"
          class="step-item"
          :class="stepClass(step.key)"
          :aria-current="step.key === currentKey ? 'step' : undefined"
        >
          <span class="step-marker">
            <!-- 已完成步骤显示对勾，让用户一眼看出进展 -->
            <el-icon
              v-if="isDone(step.key)"
              class="step-check"
            >
              <Check />
            </el-icon>
            <span
              v-else
              class="step-number"
            >{{ step.index ?? i + 1 }}</span>
          </span>
          <span class="step-label">{{ step.label }}</span>
          <!-- 步骤之间的连接线：1px slate-200。
               水平模式横向延伸（flex:1 撑开剩余空间），
               垂直模式纵向延伸（绝对定位从 marker 中心向下）。
               最后一项不渲染，避免末端出现悬空线。 -->
          <span
            v-if="i < steps.length - 1"
            class="step-line"
            aria-hidden="true"
          />
        </li>
      </template>
    </ol>
  </nav>
</template>

<script setup lang="ts">
import { Check } from '@element-plus/icons-vue'

/** 单个步骤定义：key 唯一标识，label 显示文案，index 可选自定义序号（默认按位置） */
interface Step {
  key: string
  label: string
  index?: number
}

interface Props {
  /** 步骤列表，按顺序渲染 */
  steps: Step[]
  /** 当前步骤 key（用于标记 active 态） */
  currentKey: string
  /** 已完成的步骤 key 数组（用于标记 done 态与对勾） */
  doneKeys?: string[]
  /** 排列方向：水平（默认）或垂直 */
  variant?: 'horizontal' | 'vertical'
  /** idle 态：仅渲染第 1 步 active + 提示文字，避免用户误以为流程已完成 */
  showIdleHint?: boolean
  /** idle 态提示文字 */
  idleHintText?: string
  /** nav 的 aria-label，用于无障碍标识步骤导航区域 */
  ariaLabel?: string
}

const props = withDefaults(defineProps<Props>(), {
  doneKeys: () => [],
  variant: 'horizontal',
  showIdleHint: false,
  idleHintText: '等待开始',
  ariaLabel: '步骤导航'
})

/** 判断指定步骤是否已完成 */
function isDone(key: string): boolean {
  return props.doneKeys.includes(key)
}

/** 计算步骤状态 class。
 *  done 优先级高于 active：已完成步骤不再标记为 active，
 *  避免 currentKey 同时出现在 doneKeys 时出现状态冲突。 */
function stepClass(key: string): Record<string, boolean> {
  const done = isDone(key)
  return {
    done,
    active: !done && key === props.currentKey,
    pending: !done && key !== props.currentKey
  }
}
</script>

<style scoped lang="scss">
.step-indicator {
  font-family: $font-sans;

  .step-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    align-items: center;
  }

  .step-item {
    position: relative;
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: $slate-500;
    /* 水平模式：每个 item 等分容器宽度，让连接线均匀撑开 */
    flex: 1;
    min-width: 0;

    /* 最后一项不参与等分，避免末端出现空白悬空 */
    &:last-child {
      flex: 0 0 auto;
    }
  }

  /* 步骤标记圆形：22x22，承载序号或对勾 */
  .step-marker {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: $slate-100;
    color: $slate-500;
    font-size: 11px;
    font-weight: 700;
    flex-shrink: 0;
    border: 1px solid transparent;
    transition: all 0.2s ease;
    z-index: 1;

    .step-check {
      font-size: 12px;
    }
  }

  .step-label {
    white-space: nowrap;
    font-weight: 500;
  }

  /* 步骤之间的连接线：1px slate-200，水平模式横向延伸撑开剩余空间 */
  .step-line {
    flex: 1;
    height: 1px;
    background: $slate-200;
    margin-left: 8px;
    min-width: 16px;
  }

  /* idle 态提示文字：弱化展示，避免与 label 抢视觉 */
  .step-idle-hint {
    margin-left: 8px;
    font-size: 11px;
    color: $slate-400;
    font-weight: 400;
  }

  /* idle 态下只有一项，不参与等分，让 hint 紧贴 label */
  &.is-idle .step-item {
    flex: 0 0 auto;
  }

  /* 已完成步骤：mint 浅色背景 + mint-dark 文字 */
  .step-item.done {
    color: $mint-dark;

    .step-marker {
      background: rgba(16, 185, 129, 0.12);
      color: $mint-dark;
      border-color: rgba(16, 185, 129, 0.3);
    }
  }

  /* 当前步骤：mint 渐变背景 + 白色文字，最强视觉强调 */
  .step-item.active {
    color: $mint-dark;
    font-weight: 600;

    .step-marker {
      background: linear-gradient(135deg, $mint, $mint-dark);
      color: #ffffff;
      border-color: $mint;
      box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.15);
    }
  }

  /* 未到步骤：slate-100 背景 + slate-500 文字，弱化 */
  .step-item.pending {
    color: $slate-500;

    .step-marker {
      background: $slate-100;
      color: $slate-500;
      border-color: $slate-200;
    }
  }

  /* 垂直模式：步骤纵向排列，连接线改为纵向 */
  &.variant-vertical {
    .step-list {
      flex-direction: column;
      align-items: stretch;
    }

    .step-item {
      flex: none;
      padding: 6px 0;

      /* 垂直模式连接线：绝对定位，从 marker 中心向下延伸到下一项 */
      .step-line {
        position: absolute;
        left: 11px;
        top: 28px;
        width: 1px;
        height: calc(100% - 16px);
        margin-left: 0;
        min-width: 0;
      }
    }
  }
}
</style>
