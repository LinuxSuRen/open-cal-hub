<script setup lang="ts">
// 时间轴网格：周视图 7 列 / 日视图 1 列。全天置顶行，限时日程绝对定位 + 车道排布；今日列高亮与当前时间线。
import { computed, onMounted, ref } from 'vue'
import type { Dayjs } from '../../composables/useLocalTime'
import { daySegments, type CalEvent } from '../../composables/eventLayout'
import { t } from '../../i18n'
import { pad2, softBg, weekdayLabel } from './constants'

const props = defineProps<{
  days: Dayjs[]
  items: CalEvent[]
  tz: string
  todayKey: string
  now: Dayjs
  hourHeight?: number
}>()
const emit = defineEmits<{ (e: 'create', slot: { dateKey: string; minutes: number }): void; (e: 'open', item: CalEvent): void }>()

const H = computed(() => props.hourHeight ?? 48)
const scroll = ref<HTMLElement | null>(null)

const columns = computed(() =>
  props.days.map(d => {
    const key = d.format('YYYY-MM-DD')
    return {
      key,
      dayNum: d.date(),
      weekday: d.day(),
      isToday: key === props.todayKey,
      seg: daySegments(props.items, d, props.tz),
    }
  }),
)

const hours = Array.from({ length: 24 }, (_, i) => i)
const hasAllDay = computed(() => columns.value.some(c => c.seg.allDay.length > 0))
const nowMinutes = computed(() => props.now.hour() * 60 + props.now.minute())
const showNowLine = computed(() => props.days.some(d => d.format('YYYY-MM-DD') === props.todayKey))

// 事件块定位
function segStyle(seg: { startMin: number; endMin: number; lane: number; lanes: number }): Record<string, string> {
  const top = (seg.startMin / 60) * H.value
  const height = Math.max((seg.endMin - seg.startMin) / 60 * H.value, 16)
  const width = 100 / seg.lanes
  return {
    top: `${top}px`,
    height: `${height}px`,
    left: `calc(${seg.lane * width}% + 2px)`,
    width: `calc(${width}% - 4px)`,
  }
}

function onColumnClick(e: MouseEvent, dateKey: string) {
  const el = e.currentTarget as HTMLElement
  const y = e.clientY - el.getBoundingClientRect().top
  const minutes = Math.max(0, Math.min(23 * 60 + 30, Math.round((y / H.value) * 60 / 30) * 30))
  emit('create', { dateKey, minutes })
}

onMounted(() => {
  if (scroll.value) scroll.value.scrollTop = 7 * H.value
})
</script>

<template>
  <div class="tg">
    <div class="tg__head" :style="{ gridTemplateColumns: `52px repeat(${days.length}, 1fr)` }">
      <span />
      <div v-for="c in columns" :key="c.key" class="tg__day" :class="{ 'tg__day--today': c.isToday }">
        <span class="tg__wd">{{ weekdayLabel(c.weekday) }}</span>
        <span class="tg__num" :class="{ 'tg__num--today': c.isToday }">{{ c.dayNum }}</span>
      </div>
    </div>

    <div v-if="hasAllDay" class="tg__allday" :style="{ gridTemplateColumns: `52px repeat(${days.length}, 1fr)` }">
      <span class="tg__allday-label">{{ t('cal.allDayLane') }}</span>
      <div v-for="c in columns" :key="'a' + c.key" class="tg__allday-cell">
        <button
          v-for="a in c.seg.allDay"
          :key="a.ev.id"
          type="button"
          class="tg__allday-bar"
          :style="{ background: a.cal.color }"
          :title="a.ev.summary"
          @click.stop="emit('open', a)"
        >
          {{ a.ev.summary }}
        </button>
      </div>
    </div>

    <div ref="scroll" class="tg__scroll">
      <div class="tg__canvas" :style="{ gridTemplateColumns: `52px repeat(${days.length}, 1fr)`, height: `${24 * H}px` }">
        <div class="tg__gutter">
          <span v-for="h in hours" :key="h" class="tg__hour" :style="{ top: `${h * H}px` }">{{ pad2(h) }}:00</span>
        </div>
        <div
          v-for="c in columns"
          :key="c.key"
          class="tg__col"
          :class="{ 'tg__col--today': c.isToday }"
          @click="onColumnClick($event, c.key)"
        >
          <div
            v-if="c.isToday && showNowLine"
            class="tg__now"
            :style="{ top: `${(nowMinutes / 60) * H}px` }"
          />
          <button
            v-for="s in c.seg.timed"
            :key="s.item.ev.id + '-' + s.startMin"
            type="button"
            class="tg__ev"
            :style="{ ...segStyle(s), background: softBg(s.item.cal.color), borderLeftColor: s.item.cal.color }"
            @click.stop="emit('open', s.item)"
          >
            <span class="tg__ev-time">{{ pad2(Math.floor(s.startMin / 60)) }}:{{ pad2(s.startMin % 60) }}<template v-if="s.continuesAfter"> →</template><template v-if="s.continuesBefore"> ← </template></span>
            <span class="tg__ev-title">{{ s.item.ev.summary }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tg { height: 100%; min-height: 0; display: flex; flex-direction: column; background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m); overflow: hidden; }
.tg__head { display: grid; border-bottom: 1px solid var(--color-border-subtle); background: var(--color-bg-sunken); }
.tg__day { display: flex; flex-direction: column; align-items: center; padding: var(--space-2) 0; gap: 2px; }
.tg__wd { font-size: 11px; color: var(--color-text-dim); }
.tg__num { font-size: 16px; color: var(--color-text-primary); width: 28px; height: 28px; display: flex; align-items: center; justify-content: center; border-radius: var(--radius-full); }
.tg__num--today { background: var(--color-accent); color: #fff; font-weight: 600; }
.tg__allday { display: grid; border-bottom: 1px solid var(--color-border-subtle); }
.tg__allday-label { font-size: 11px; color: var(--color-text-dim); padding: var(--space-2) var(--space-2) 0 0; text-align: right; }
.tg__allday-cell { display: flex; flex-direction: column; gap: 2px; padding: 4px 2px; }
.tg__allday-bar {
  border: none; cursor: pointer; text-align: left;
  font-size: 11px; color: #fff; padding: 2px 6px; border-radius: 4px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.tg__allday-bar:hover { filter: brightness(1.08); }
.tg__scroll { flex: 1; min-height: 0; overflow-y: auto; }
.tg__canvas { display: grid; position: relative; }
.tg__gutter { position: relative; }
.tg__hour {
  position: absolute; right: 6px; transform: translateY(-50%);
  font-size: 10px; color: var(--color-text-dim); font-family: var(--font-mono);
}
.tg__col {
  position: relative; cursor: copy;
  background-image:
    linear-gradient(to bottom, var(--color-border-subtle) 1px, transparent 1px);
  background-size: 100% calc(v-bind('H') * 1px);
}
.tg__col--today { background-color: color-mix(in srgb, var(--color-accent) 4%, transparent); }
.tg__now { position: absolute; left: 0; right: 0; height: 2px; background: var(--color-danger); z-index: 2; pointer-events: none; }
.tg__now::before { content: ''; position: absolute; left: -3px; top: -3px; width: 8px; height: 8px; border-radius: 50%; background: var(--color-danger); }
.tg__ev {
  position: absolute; z-index: 1; cursor: pointer;
  border: none; border-left: 3px solid; border-radius: 4px;
  padding: 2px 6px; text-align: left; overflow: hidden;
  display: flex; flex-direction: column; gap: 1px;
}
.tg__ev-time { font-size: 10px; color: var(--color-text-secondary); font-family: var(--font-mono); }
.tg__ev-title { font-size: 12px; color: var(--color-text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
