<script setup lang="ts">
// 日历页：月/周/日/列表四视图；侧栏筛选；跨边界按视图范围拉取（上限 62 天由 useCalendarRange 保证）。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import dayjs from 'dayjs'
import { calApi, type Calendar } from '../../api/calendars'
import { computeRange, rangeTitle, shiftAnchor, type CalView } from '../../composables/useCalendarRange'
import { useCalendarEvents } from '../../composables/useCalendarEvents'
import { groupByDay, type CalEvent } from '../../composables/eventLayout'
import { dayKeyOf, todayKey, tzDate, useNow, userTimezone, fmtTime, type Dayjs } from '../../composables/useLocalTime'
import { errorText } from '../../composables/useApiError'
import { toast, toastError } from '../../composables/useToast'
import { t } from '../../i18n'
import CalendarSidebar from '../../components/calendar/CalendarSidebar.vue'
import MonthGrid from '../../components/calendar/MonthGrid.vue'
import TimeGrid from '../../components/calendar/TimeGrid.vue'
import EventEditorDialog from '../../components/calendar/EventEditorDialog.vue'

const route = useRoute()
const router = useRouter()

const VIEWS: CalView[] = ['month', 'week', 'day', 'list']

function validView(v: unknown): CalView | null {
  return VIEWS.includes(v as CalView) ? (v as CalView) : null
}
function validDate(v: unknown): string | null {
  return typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) ? v : null
}

const view = ref<CalView>(validView(route.query.view) ?? 'month')
const anchorKey = ref<string>(validDate(route.query.date) ?? todayKey())

const calendars = ref<Calendar[]>([])
const selectedIds = ref<number[]>([])
const { loading, error, events, load } = useCalendarEvents()
const now = useNow()

const tz = computed(() => userTimezone())
const todayNowKey = computed(() => dayKeyOf(now.value))
const editableCals = computed(() => calendars.value.filter(c => c.role !== 'viewer'))
const range = computed(() => computeRange(anchorKey.value, view.value, tz.value))
const gridDays = computed<Dayjs[]>(() =>
  view.value === 'day' ? [tzDate(anchorKey.value, tz.value)] : range.value.days,
)

const editorVisible = ref(false)
const editing = ref<CalEvent | null>(null)
const draft = ref<{ dateKey: string; minutes: number } | null>(null)

async function loadCalendars() {
  try {
    const list = await calApi.list()
    calendars.value = Array.isArray(list) ? list : []
    if (selectedIds.value.length === 0 && calendars.value.length > 0) {
      selectedIds.value = calendars.value.map(c => c.id)
    }
  } catch (err) {
    toastError(err)
  }
}

onMounted(loadCalendars)

function reload() {
  const cals = calendars.value.filter(c => selectedIds.value.includes(c.id))
  load(cals, range.value.from, range.value.to)
}

const selectedKey = computed(() => selectedIds.value.join(','))
watch([() => range.value.from, () => range.value.to, selectedKey], reload)

// 深链同步：date/view 写回 query，供工作台跳转与刷新保持
watch([view, anchorKey], () => {
  router.replace({ query: { ...route.query, view: view.value, date: anchorKey.value } }).catch(() => undefined)
})

const title = computed(() => {
  const base = rangeTitle(anchorKey.value, view.value)
  if (view.value === 'day') {
    return `${base} ${t(`date.weekday.${dayjs(anchorKey.value).day()}`)}`
  }
  return base
})

function shift(dir: 1 | -1) {
  anchorKey.value = shiftAnchor(anchorKey.value, view.value, dir)
}
function goToday() {
  anchorKey.value = todayKey()
}
function goDay(dateKey: string) {
  anchorKey.value = dateKey
  view.value = 'day'
}
function toggleCalendar(id: number) {
  selectedIds.value = selectedIds.value.includes(id)
    ? selectedIds.value.filter(x => x !== id)
    : [...selectedIds.value, id]
}

function openCreate(dateKey: string, minutes = 9 * 60) {
  if (editableCals.value.length === 0) {
    toast(t('cal.noCalendar'), 'error')
    return
  }
  editing.value = null
  draft.value = { dateKey, minutes }
  editorVisible.value = true
}
function openEdit(item: CalEvent) {
  editing.value = item
  editorVisible.value = true
}

// 列表视图聚合
const agendaGroups = computed(() => {
  if (view.value !== 'list') return []
  const grouped = groupByDay(events.value, tz.value)
  const today = todayKey()
  const tomorrow = dayjs(today).add(1, 'day').format('YYYY-MM-DD')
  return range.value.days
    .map(d => {
      const key = d.format('YYYY-MM-DD')
      return {
        key,
        weekday: d.day(),
        label: key === today ? t('home.today') : key === tomorrow ? t('home.tomorrow') : `${d.format('M月D日')} ${t(`date.weekday.${d.day()}`)}`,
        items: grouped.get(key) ?? [],
      }
    })
    .filter(g => g.items.length > 0)
})

function agendaTime(item: CalEvent): string {
  return item.ev.all_day ? t('home.allDay') : fmtTime(item.ev.start)
}
</script>

<template>
  <div class="calp">
    <CalendarSidebar
      :anchor-key="anchorKey"
      :today-key="todayNowKey"
      :calendars="calendars"
      :selected-ids="selectedIds"
      @nav="shift"
      @select="k => (anchorKey = k)"
      @toggle="toggleCalendar"
    />

    <main class="calp__main">
      <div class="calp__toolbar">
        <div class="calp__nav">
          <button type="button" class="btn calp__nav-btn" :aria-label="t('aria.prev')" @click="shift(-1)">‹</button>
          <button type="button" class="btn calp__nav-btn" :aria-label="t('aria.next')" @click="shift(1)">›</button>
          <button type="button" class="btn" @click="goToday">{{ t('cal.today') }}</button>
        </div>
        <h2 class="calp__title">
          {{ title }}
          <span v-if="loading" class="calp__loading" />
        </h2>
        <div class="calp__right">
          <div class="calp__views" role="tablist">
            <button
              v-for="v in VIEWS"
              :key="v"
              type="button"
              role="tab"
              :aria-selected="view === v"
              class="calp__view-btn"
              :class="{ 'calp__view-btn--on': view === v }"
              @click="view = v"
            >
              {{ t('cal.view.' + v) }}
            </button>
          </div>
          <button type="button" class="btn btn-primary" @click="openCreate(anchorKey)">{{ t('cal.newEvent') }}</button>
        </div>
      </div>

      <div v-if="error !== null" class="calp__error" role="alert">{{ errorText(error) }}</div>

      <div class="calp__view">
        <template v-if="calendars.length > 0">
          <MonthGrid
            v-if="view === 'month'"
            :days="range.days"
            :anchor-key="anchorKey"
            :items="events"
            :tz="tz"
            :today-key="todayNowKey"
            @create="k => openCreate(k)"
            @open="openEdit"
            @more="goDay"
          />
          <TimeGrid
            v-else-if="view === 'week' || view === 'day'"
            :days="gridDays"
            :items="events"
            :tz="tz"
            :today-key="todayNowKey"
            :now="now"
            @create="s => openCreate(s.dateKey, s.minutes)"
            @open="openEdit"
          />
          <div v-else-if="view === 'list'" class="calp__agenda">
            <section v-for="g in agendaGroups" :key="g.key" class="calp__group">
              <h3 class="calp__group-title">{{ g.label }}</h3>
              <button v-for="item in g.items" :key="item.ev.id + item.ev.start" type="button" class="calp__row" @click="openEdit(item)">
                <span class="calp__row-time">{{ agendaTime(item) }}</span>
                <span class="calp__row-dot" :style="{ background: item.cal.color }" />
                <span class="calp__row-summary">{{ item.ev.summary }}</span>
                <span class="calp__row-cal">{{ item.cal.name }}</span>
                <span v-if="item.ev.location" class="calp__row-loc">{{ item.ev.location }}</span>
              </button>
            </section>
            <p v-if="agendaGroups.length === 0" class="calp__empty">{{ t('cal.emptyDay') }}</p>
          </div>
        </template>
        <div v-else class="calp__empty-panel">
          <p>{{ t('cal.noCalendar') }}</p>
          <router-link class="btn btn-primary" to="/app/calendars">{{ t('cal.goCalendars') }}</router-link>
        </div>
      </div>
    </main>

    <EventEditorDialog
      v-model:visible="editorVisible"
      :calendars="editableCals"
      :event="editing"
      :draft="draft"
      :tz="tz"
      @saved="reload"
      @deleted="reload"
    />
  </div>
</template>

<style scoped>
.calp { flex: 1; display: flex; min-height: 0; }
.calp__main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.calp__toolbar {
  display: flex; align-items: center; gap: var(--space-4);
  padding: var(--space-3) var(--space-4);
}
.calp__nav { display: flex; gap: var(--space-2); }
.calp__nav-btn { padding: 8px 12px; }
.calp__title { margin: 0; font-size: 16px; font-weight: 600; display: flex; align-items: center; gap: var(--space-2); }
.calp__loading {
  width: 12px; height: 12px; border-radius: 50%;
  border: 2px solid var(--color-accent-soft); border-top-color: var(--color-accent);
  animation: calp-spin 0.8s linear infinite;
}
@keyframes calp-spin { to { transform: rotate(360deg); } }
.calp__right { margin-left: auto; display: flex; align-items: center; gap: var(--space-3); }
.calp__views {
  display: flex; padding: 2px; gap: 2px;
  background: var(--color-bg-sunken); border-radius: var(--radius-s);
}
.calp__view-btn {
  border: none; background: none; cursor: pointer;
  padding: 5px 14px; font-size: 13px; color: var(--color-text-secondary);
  border-radius: var(--radius-s);
}
.calp__view-btn--on { background: var(--color-bg-panel); color: var(--color-text-primary); font-weight: 500; box-shadow: 0 1px 2px rgba(18, 22, 31, 0.08); }
.calp__error { margin: 0 var(--space-4); padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px; }
.calp__view { flex: 1; min-height: 0; padding: 0 var(--space-4) var(--space-4); display: flex; }
.calp__agenda { flex: 1; overflow-y: auto; background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m); padding: var(--space-3) var(--space-4); }
.calp__group + .calp__group { margin-top: var(--space-4); }
.calp__group-title { margin: 0 0 var(--space-1); font-size: 13px; color: var(--color-text-secondary); }
.calp__row {
  width: 100%; display: flex; align-items: baseline; gap: var(--space-3);
  border: none; background: none; cursor: pointer; text-align: left;
  padding: var(--space-2) var(--space-2); border-radius: var(--radius-s);
}
.calp__row:hover { background: var(--color-bg-sunken); }
.calp__row-time { font-family: var(--font-mono); font-size: 12px; color: var(--color-text-secondary); width: 52px; flex-shrink: 0; }
.calp__row-dot { width: 10px; height: 10px; border-radius: var(--radius-full); flex-shrink: 0; align-self: center; }
.calp__row-summary { font-size: 14px; color: var(--color-text-primary); }
.calp__row-cal { font-size: 12px; color: var(--color-text-dim); }
.calp__row-loc { font-size: 12px; color: var(--color-text-dim); margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.calp__empty { color: var(--color-text-dim); font-size: 13px; padding: var(--space-3); }
.calp__empty-panel { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: var(--space-4); color: var(--color-text-dim); }
</style>
