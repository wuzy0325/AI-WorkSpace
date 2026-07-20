<script setup lang="ts">
// 七孔探针校准图标（spec Task 24）
// 几何布局：中心 1 孔（P7）+ 外围 6 孔按 60° 等分（P1~P6）
// 与五孔图标范式一致：中心大圆 + 外围小圆 + 连接线
withDefaults(defineProps<{ size?: number }>(), { size: 24 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <!-- 中心孔 P7（最大，正对来流方向） -->
    <circle cx="12" cy="12" r="3" />
    <!-- 外围 6 孔按 60° 等分（从顶部 12 点位置起顺时针：P1/P2/P3/P4/P5/P6） -->
    <!-- P1：顶部 0° -->
    <circle cx="12" cy="4" r="1.8" />
    <!-- P2：右上 60° -->
    <circle cx="18.93" cy="8" r="1.8" />
    <!-- P3：右下 120° -->
    <circle cx="18.93" cy="16" r="1.8" />
    <!-- P4：底部 180° -->
    <circle cx="12" cy="20" r="1.8" />
    <!-- P5：左下 240° -->
    <circle cx="5.07" cy="16" r="1.8" />
    <!-- P6：左上 300° -->
    <circle cx="5.07" cy="8" r="1.8" />
    <!-- 中心孔到外围 6 孔的连接线（表示压力传导关系） -->
    <line x1="12" y1="9" x2="12" y2="5.8" />
    <line x1="14.6" y1="10.5" x2="17.4" y2="8.9" />
    <line x1="14.6" y1="13.5" x2="17.4" y2="15.1" />
    <line x1="12" y1="15" x2="12" y2="18.2" />
    <line x1="9.4" y1="13.5" x2="6.6" y2="15.1" />
    <line x1="9.4" y1="10.5" x2="6.6" y2="8.9" />
  </svg>
</template>
