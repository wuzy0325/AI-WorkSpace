<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    thermocoupleType?: string
    coldJunction?: string
    filterHz?: number
  }>(),
  { thermocoupleType: 'K', coldJunction: 'internal', filterHz: 50 },
)

const emit = defineEmits<{
  (e: 'update:thermocoupleType', v: string): void
  (e: 'update:coldJunction', v: string): void
  (e: 'update:filterHz', v: number): void
}>()

const tcTypes = ['K', 'J', 'T', 'E', 'N', 'R', 'S', 'B']
const cjOptions = ['internal', 'external', 'fixed']
</script>

<template>
  <div class="t1603-config">
    <div class="t1603-config__field">
      <label>Thermocouple Type</label>
      <select
        :value="thermocoupleType"
        @change="emit('update:thermocoupleType', ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="t in tcTypes" :key="t" :value="t">{{ t }}</option>
      </select>
    </div>
    <div class="t1603-config__field">
      <label>Cold Junction</label>
      <select
        :value="coldJunction"
        @change="emit('update:coldJunction', ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="cj in cjOptions" :key="cj" :value="cj">{{ cj }}</option>
      </select>
    </div>
    <div class="t1603-config__field">
      <label>Filter (Hz)</label>
      <input
        type="number"
        :value="filterHz"
        min="1"
        max="500"
        @input="emit('update:filterHz', Number(($event.target as HTMLInputElement).value))"
      />
    </div>
  </div>
</template>

<style scoped>
.t1603-config {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-3);
  padding: var(--space-3);
  border-radius: 0.5rem;
  background: rgba(30, 41, 59, 0.3);
}

.t1603-config__field label {
  display: block;
  margin-bottom: 0.3rem;
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
}

.t1603-config__field select,
.t1603-config__field input {
  width: 100%;
  padding: 0.4rem 0.6rem;
  border-radius: 0.35rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: 0.85rem;
}
</style>
