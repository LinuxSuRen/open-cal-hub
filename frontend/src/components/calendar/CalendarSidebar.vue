<script setup lang="ts">
// 日历页侧栏：迷你月历导航 + 日历可见性筛选 + 日历管理入口。
import { t } from '../../i18n'
import type { Calendar } from '../../api/calendars'
import MiniMonth from './MiniMonth.vue'

defineProps<{
  anchorKey: string
  todayKey: string
  calendars: Calendar[]
  selectedIds: number[]
}>()
const emit = defineEmits<{ (e: 'nav', dir: 1 | -1): void; (e: 'select', dateKey: string): void; (e: 'toggle', id: number): void }>()
</script>

<template>
  <aside class="side">
    <MiniMonth :anchor-key="anchorKey" :today-key="todayKey" @nav="d => emit('nav', d)" @select="k => emit('select', k)" />
    <div class="side__section">
      <span class="side__title">{{ t('cal.calendarFilter') }}</span>
      <ul class="side__list">
        <li v-for="c in calendars" :key="c.id" class="side__item">
          <label class="side__check">
            <input
              type="checkbox"
              :checked="selectedIds.includes(c.id)"
              :aria-label="c.name"
              @change="emit('toggle', c.id)"
            />
            <span class="side__dot" :style="{ background: c.color }" />
            <span class="side__name" :title="c.name">{{ c.name }}</span>
            <span class="side__role">{{ t('cals.role.' + c.role) }}</span>
          </label>
        </li>
      </ul>
      <p v-if="calendars.length === 0" class="side__empty">{{ t('cal.noCalendar') }}</p>
      <router-link class="side__manage" to="/app/calendars">{{ t('home.goCalendars') }}</router-link>
    </div>
  </aside>
</template>

<style scoped>
.side {
  width: 232px; flex-shrink: 0;
  border-right: 1px solid var(--color-border-subtle);
  background: var(--color-bg-panel);
  display: flex; flex-direction: column; overflow-y: auto;
}
.side__section { padding: var(--space-3) var(--space-4); display: flex; flex-direction: column; gap: var(--space-2); }
.side__title { font-size: 12px; color: var(--color-text-dim); }
.side__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.side__check { display: flex; align-items: center; gap: var(--space-2); padding: 4px 4px; border-radius: var(--radius-s); cursor: pointer; }
.side__check:hover { background: var(--color-bg-sunken); }
.side__check input { accent-color: var(--color-accent); flex-shrink: 0; }
.side__dot { width: 10px; height: 10px; border-radius: var(--radius-full); flex-shrink: 0; }
.side__name { flex: 1; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.side__role { font-size: 11px; color: var(--color-text-dim); flex-shrink: 0; }
.side__empty { font-size: 12px; color: var(--color-text-dim); margin: 0; }
.side__manage { font-size: 13px; margin-top: var(--space-2); }
</style>
