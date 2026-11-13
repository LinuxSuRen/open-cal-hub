<script setup lang="ts">
// 迷你月历：锚点月导航与日期选择；跨月日期可点并切换。
import { computed } from 'vue'
import dayjs from 'dayjs'
import { startOfWeekKey } from '../../composables/useCalendarRange'
import { t } from '../../i18n'
import { GRID_WEEKDAYS, weekdayLabel } from './constants'

const props = defineProps<{ anchorKey: string; todayKey?: string }>()
const emit = defineEmits<{ (e: 'select', dateKey: string): void; (e: 'nav', dir: 1 | -1): void }>()

const title = computed(() => dayjs(props.anchorKey).format('YYYY年M月'))

const cells = computed(() => {
  const firstKey = dayjs(props.anchorKey).startOf('month').format('YYYY-MM-DD')
  const startKey = startOfWeekKey(firstKey)
  return Array.from({ length: 42 }, (_, i) => {
    const d = dayjs(startKey).add(i, 'day')
    const key = d.format('YYYY-MM-DD')
    return {
      key,
      num: d.date(),
      out: d.format('YYYY-MM') !== dayjs(props.anchorKey).format('YYYY-MM'),
      today: key === props.todayKey,
      sel: key === props.anchorKey,
    }
  })
})
</script>

<template>
  <div class="mini">
    <div class="mini__head">
      <button type="button" class="mini__nav" :aria-label="t('aria.prev')" @click="emit('nav', -1)">‹</button>
      <span class="mini__title">{{ title }}</span>
      <button type="button" class="mini__nav" :aria-label="t('aria.next')" @click="emit('nav', 1)">›</button>
    </div>
    <div class="mini__grid">
      <span v-for="n in GRID_WEEKDAYS" :key="'h' + n" class="mini__wd">{{ weekdayLabel(n).slice(1) }}</span>
      <button
        v-for="c in cells"
        :key="c.key"
        type="button"
        class="mini__cell"
        :class="{ 'mini__cell--out': c.out, 'mini__cell--today': c.today, 'mini__cell--sel': c.sel }"
        @click="emit('select', c.key)"
      >
        {{ c.num }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.mini { padding: var(--space-4) var(--space-4) var(--space-2); }
.mini__head { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--space-2); }
.mini__title { font-size: 13px; font-weight: 600; }
.mini__nav {
  border: none; background: none; cursor: pointer;
  color: var(--color-text-secondary); font-size: 16px; line-height: 1;
  padding: var(--space-1) var(--space-2); border-radius: var(--radius-s);
}
.mini__nav:hover { background: var(--color-bg-sunken); color: var(--color-text-primary); }
.mini__grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 2px; }
.mini__wd { text-align: center; font-size: 11px; color: var(--color-text-dim); padding: 2px 0; }
.mini__cell {
  border: none; background: none; cursor: pointer;
  font-size: 12px; color: var(--color-text-primary);
  height: 26px; border-radius: var(--radius-full);
  display: flex; align-items: center; justify-content: center;
}
.mini__cell:hover { background: var(--color-bg-sunken); }
.mini__cell--out { color: var(--color-text-dim); }
.mini__cell--today { box-shadow: inset 0 0 0 1px var(--color-accent); color: var(--color-accent); }
.mini__cell--sel { background: var(--color-accent); color: #fff; }
.mini__cell--sel:hover { background: var(--color-accent-hover); }
</style>
