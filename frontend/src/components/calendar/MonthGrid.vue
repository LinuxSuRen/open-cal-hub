<script setup lang="ts">
// 月格：手写 CSS Grid 7 列 × N 周；全天置顶条；限时日程按日历着色；跨月边界事件按天切片渲染。
import { computed } from 'vue'
import type { Dayjs } from '../../composables/useLocalTime'
import { monthCell, type CalEvent } from '../../composables/eventLayout'
import { t } from '../../i18n'
import { fillParam, GRID_WEEKDAYS, softBg, weekdayLabel } from './constants'

const props = defineProps<{
  days: Dayjs[]
  anchorKey: string
  items: CalEvent[]
  tz: string
  todayKey: string
  maxVisible?: number
}>()
const emit = defineEmits<{ (e: 'create', dateKey: string): void; (e: 'open', item: CalEvent): void; (e: 'more', dateKey: string): void }>()

const monthPrefix = computed(() => props.anchorKey.slice(0, 7))
const weeks = computed(() => Math.ceil(props.days.length / 7))

const cells = computed(() =>
  props.days.map(d => {
    const key = d.format('YYYY-MM-DD')
    return {
      key,
      num: d.date(),
      out: key.slice(0, 7) !== monthPrefix.value,
      today: key === props.todayKey,
      data: monthCell(props.items, d, props.tz, props.maxVisible ?? 3),
    }
  }),
)
</script>

<template>
  <div class="mg" :style="{ gridTemplateRows: `40px repeat(${weeks}, 1fr)` }">
    <div v-for="n in GRID_WEEKDAYS" :key="'h' + n" class="mg__wd">{{ weekdayLabel(n) }}</div>
    <div
      v-for="c in cells"
      :key="c.key"
      class="mg__cell"
      :class="{ 'mg__cell--out': c.out, 'mg__cell--today': c.today }"
      @click="emit('create', c.key)"
    >
      <span class="mg__num" :class="{ 'mg__num--today': c.today }">{{ c.num }}</span>
      <div class="mg__events">
        <button
          v-for="a in c.data.allDay"
          :key="'a' + a.ev.id"
          type="button"
          class="mg__allday"
          :style="{ background: a.cal.color }"
          :title="a.ev.summary"
          @click.stop="emit('open', a)"
        >
          {{ a.ev.summary }}
        </button>
        <button
          v-for="s in c.data.timed"
          :key="s.item.ev.id + s.startMin"
          type="button"
          class="mg__chip"
          :class="{ 'mg__chip--cont': s.continuesBefore }"
          :style="{ background: softBg(s.item.cal.color), borderColor: s.item.cal.color }"
          :title="s.item.ev.summary"
          @click.stop="emit('open', s.item)"
        >
          <span v-if="!s.continuesBefore" class="mg__chip-time">{{ String(Math.floor(s.startMin / 60)).padStart(2, '0') }}:{{ String(s.startMin % 60).padStart(2, '0') }}</span>
          <span class="mg__chip-text">{{ s.item.ev.summary }}</span>
        </button>
        <button v-if="c.data.hidden > 0" type="button" class="mg__more" @click.stop="emit('more', c.key)">
          {{ fillParam(t('cal.moreEvents'), 'n', c.data.hidden) }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mg {
  height: 100%; min-height: 0;
  display: grid; grid-template-columns: repeat(7, 1fr);
  background: var(--color-bg-panel);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m);
  overflow: hidden;
}
.mg__wd {
  display: flex; align-items: center; justify-content: flex-end;
  padding: 0 var(--space-3);
  font-size: 12px; color: var(--color-text-dim);
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-bg-sunken);
}
.mg__cell {
  min-height: 0; padding: 4px 6px;
  border-right: 1px solid var(--color-border-subtle);
  border-top: 1px solid var(--color-border-subtle);
  display: flex; flex-direction: column; gap: 2px;
  cursor: pointer; overflow: hidden;
}
.mg__cell:nth-child(7n + 8) { border-right: none; }
.mg__cell:hover { background: var(--color-bg-base); }
.mg__cell--out .mg__num { color: var(--color-text-dim); }
.mg__cell--out { background: var(--color-bg-base); }
.mg__num { font-size: 12px; color: var(--color-text-secondary); width: 22px; height: 22px; display: inline-flex; align-items: center; justify-content: center; border-radius: var(--radius-full); }
.mg__num--today { background: var(--color-accent); color: #fff; font-weight: 600; }
.mg__events { display: flex; flex-direction: column; gap: 2px; min-height: 0; }
.mg__allday {
  border: none; cursor: pointer; text-align: left;
  font-size: 11px; color: #fff; line-height: 1.2;
  padding: 2px 6px; border-radius: 4px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.mg__allday:hover { filter: brightness(1.08); }
.mg__chip {
  border: 1px solid; border-left-width: 3px; cursor: pointer; text-align: left;
  font-size: 11px; color: var(--color-text-primary); line-height: 1.2;
  padding: 1px 5px; border-radius: 4px;
  display: flex; gap: 4px; align-items: baseline;
  white-space: nowrap; overflow: hidden;
}
.mg__chip-time { color: var(--color-text-secondary); flex-shrink: 0; }
.mg__chip-text { overflow: hidden; text-overflow: ellipsis; }
.mg__chip--cont .mg__chip-text { color: var(--color-text-secondary); }
.mg__more {
  border: none; background: none; cursor: pointer;
  font-size: 11px; color: var(--color-text-secondary); padding: 1px 5px; text-align: left;
}
.mg__more:hover { color: var(--color-accent); }
</style>
