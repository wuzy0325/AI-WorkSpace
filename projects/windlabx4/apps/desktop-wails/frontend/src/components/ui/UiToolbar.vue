<template>
  <div class="ui-toolbar">
    <slot />
  </div>
</template>

<style scoped>
.ui-toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}
</style>
