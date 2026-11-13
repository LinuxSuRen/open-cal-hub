<script setup lang="ts">
// 槽位网格：按上午/下午/晚间分组渲染时间按钮（纯展示，分组逻辑在 slots.ts）。
import { computed } from 'vue'
import { t } from '../../i18n'
import { groupSlotsByPeriod, fmtTime, type SlotItem } from './slots'

const props = defineProps<{
  slots: SlotItem[]
  tz: string
  selectedStart?: string | null
  disabled?: boolean
}>()

const emit = defineEmits<{ (e: 'select', slot: SlotItem): void }>()

const groups = computed(() => {
  const g = groupSlotsByPeriod(props.slots, props.tz)
  return [
    { key: 'morning', label: t('pub.slot.morning'), slots: g.morning },
    { key: 'afternoon', label: t('pub.slot.afternoon'), slots: g.afternoon },
    { key: 'evening', label: t('pub.slot.evening'), slots: g.evening },
  ].filter(g => g.slots.length > 0)
})
</script>

<template>
  <div class="slotgrid">
    <div v-for="g in groups" :key="g.key" class="slotgrid__group">
      <div class="slotgrid__group-label">{{ g.label }}</div>
      <div class="slotgrid__grid">
        <button
          v-for="s in g.slots" :key="s.start" type="button" class="slotgrid__slot"
          :class="{ 'slotgrid__slot--selected': selectedStart === s.start }"
          :disabled="!s.available || disabled"
          @click="emit('select', s)"
        >{{ fmtTime(s.start, tz) }}</button>
      </div>
    </div>
    <p v-if="groups.length === 0" class="slotgrid__empty">{{ t('pub.noSlots') }}</p>
  </div>
</template>

<style scoped>
.slotgrid { display: flex; flex-direction: column; gap: var(--space-5); }
.slotgrid__group-label {
  font-size: 12px; color: var(--color-text-dim); margin-bottom: var(--space-2);
  letter-spacing: 0.04em;
}
.slotgrid__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(86px, 1fr)); gap: var(--space-2); }
.slotgrid__slot {
  padding: 9px 0; border-radius: var(--radius-s); border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-panel); color: var(--color-text-primary);
  font-size: 14px; font-family: var(--font-mono); cursor: pointer; transition: border-color 0.12s;
}
.slotgrid__slot:hover:not(:disabled) { border-color: var(--color-accent); color: var(--color-accent); }
.slotgrid__slot:disabled { color: var(--color-text-dim); background: var(--color-bg-sunken); cursor: default; text-decoration: line-through; }
.slotgrid__slot--selected { background: var(--color-accent); border-color: var(--color-accent); color: #fff; }
.slotgrid__empty { color: var(--color-text-dim); text-align: center; padding: var(--space-6) 0; margin: 0; }
</style>
