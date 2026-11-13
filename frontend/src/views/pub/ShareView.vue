<script setup lang="ts">
// 公开日历分享 /s/{token}：只读迷你月视图 + 事件列表切换 + ICS 订阅直链。
// 404（share_not_found）/ 410（share_expired）专门页面。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { pubApi, shareIcsUrl, type PublicShare } from '../../api/pub'
import { ApiError, type EventInstance } from '../../api'
import { t, getLocale } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { addDays, dateInTz, fmtTime, humanDate, todayInTz } from '../../components/booking/slots'
import ErrorPanel from '../../components/ds/ErrorPanel.vue'
import PubShell from './PubShell.vue'

const route = useRoute()
const token = String(route.params.token ?? '')

const share = ref<PublicShare | null>(null)
const loading = ref(true)
const errorCode = ref('')

const view = ref<'month' | 'list'>('month')
const monthStart = ref('') // YYYY-MM-01
const events = ref<EventInstance[]>([])
const eventsLoading = ref(false)
const selectedDate = ref('')

const monthLabel = computed(() => {
  if (!monthStart.value) return ''
  const d = new Date(`${monthStart.value}T12:00:00Z`)
  return new Intl.DateTimeFormat(getLocale() === 'en' ? 'en-US' : 'zh-CN', { year: 'numeric', month: 'long' }).format(d)
})

const byDate = computed<Map<string, EventInstance[]>>(() => {
  const map = new Map<string, EventInstance[]>()
  if (!share.value) return map
  for (const ev of events.value) {
    const key = dateInTz(ev.start, share.value.timezone)
    const list = map.get(key) ?? []
    list.push(ev)
    map.set(key, list)
  }
  return map
})

/** 周一起始的 6×7 网格日期。 */
const grid = computed<string[]>(() => {
  if (!monthStart.value) return []
  const firstWd = new Date(`${monthStart.value}T12:00:00Z`).getUTCDay() // 0=周日
  const back = (firstWd + 6) % 7
  const start = addDays(monthStart.value, -back)
  return Array.from({ length: 42 }, (_, i) => addDays(start, i))
})

function lastOfMonth(ymd: string): string {
  const d = new Date(`${ymd}T12:00:00Z`)
  const last = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0))
  return last.toISOString().slice(0, 10)
}

async function loadMonth(): Promise<void> {
  if (!monthStart.value) return
  eventsLoading.value = true
  try {
    events.value = (await pubApi.shareEvents(token, monthStart.value, lastOfMonth(monthStart.value))) ?? []
  } catch {
    events.value = []
  } finally {
    eventsLoading.value = false
  }
}

function shiftMonth(n: number): void {
  const d = new Date(`${monthStart.value}T12:00:00Z`)
  const next = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + n, 1))
  monthStart.value = next.toISOString().slice(0, 10)
  selectedDate.value = ''
}

function goToday(): void {
  const today = todayInTz(share.value?.timezone ?? 'UTC')
  monthStart.value = `${today.slice(0, 7)}-01`
  selectedDate.value = today
}

const sortedDates = computed(() => [...byDate.value.keys()].sort())

onMounted(async () => {
  try {
    share.value = await pubApi.share(token)
    const today = todayInTz(share.value.timezone)
    monthStart.value = `${today.slice(0, 7)}-01`
  } catch (err) {
    errorCode.value = err instanceof ApiError ? err.code : 'network'
  } finally {
    loading.value = false
  }
})

watch(monthStart, () => { void loadMonth() })

function inMonth(date: string): boolean {
  return date.slice(0, 7) === monthStart.value.slice(0, 7)
}
function dayEvents(date: string): EventInstance[] {
  return byDate.value.get(date) ?? []
}
</script>

<template>
  <PubShell width="760px">
    <div v-if="loading" class="sv__loading">{{ t('state.loading') }}</div>
    <ErrorPanel
      v-else-if="errorCode === 'cal/share_not_found'"
      :title="t('pub.shareNotFound.title')" :desc="t('pub.shareNotFound.desc')"
    />
    <ErrorPanel
      v-else-if="errorCode === 'cal/share_expired'"
      :title="t('pub.shareExpired.title')" :desc="t('pub.shareExpired.desc')"
    />
    <ErrorPanel
      v-else-if="errorCode"
      :title="apiErrorText({ code: errorCode, message: '' })"
    />

    <template v-else-if="share">
      <div class="sv__head" :style="{ '--cal-color': share.calendar.color }">
        <div class="sv__head-main">
          <span class="sv__cal-dot" />
          <div>
            <h1 class="sv__title">{{ share.calendar.name }}</h1>
            <p class="sv__owner">{{ share.owner_display_name }} · {{ share.timezone }}</p>
          </div>
        </div>
        <a class="btn sv__ics" :href="shareIcsUrl(token)" :title="t('pub.share.icsHint')">{{ t('pub.share.ics') }}</a>
      </div>
      <p v-if="share.calendar.description" class="sv__desc">{{ share.calendar.description }}</p>

      <div class="sv__toolbar">
        <div class="sv__seg">
          <button type="button" class="sv__seg-btn" :class="{ 'sv__seg-btn--on': view === 'month' }" @click="view = 'month'">{{ t('pub.share.viewMonth') }}</button>
          <button type="button" class="sv__seg-btn" :class="{ 'sv__seg-btn--on': view === 'list' }" @click="view = 'list'">{{ t('pub.share.viewList') }}</button>
        </div>
      </div>

      <!-- 月视图 -->
      <div v-if="view === 'month'" class="sv__panel">
        <div class="sv__month-bar">
          <button class="btn sv__mb-btn" type="button" @click="shiftMonth(-1)">‹</button>
          <span class="sv__month-label">{{ monthLabel }}</span>
          <button class="btn sv__mb-btn" type="button" @click="goToday">{{ t('pub.share.today') }}</button>
          <button class="btn sv__mb-btn" type="button" @click="shiftMonth(1)">›</button>
        </div>
        <div class="sv__wd-row">
          <span v-for="i in 7" :key="i" class="sv__wd">{{ t(`pub.wd.${i % 7}`) }}</span>
        </div>
        <div class="sv__grid">
          <button
            v-for="date in grid" :key="date" type="button" class="sv__cell"
            :class="{
              'sv__cell--out': !inMonth(date),
              'sv__cell--today': share && date === todayInTz(share.timezone),
              'sv__cell--selected': date === selectedDate,
            }"
            @click="selectedDate = date"
          >
            <span class="sv__day-num">{{ Number(date.slice(8, 10)) }}</span>
            <span v-if="dayEvents(date).length > 0" class="sv__cell-dots">{{ dayEvents(date).length }}</span>
          </button>
        </div>

        <div v-if="selectedDate" class="sv__day">
          <h3 class="sv__day-title">{{ t('pub.share.dayEvents') }} · {{ humanDate(selectedDate, share.timezone, getLocale()) }}</h3>
          <p v-if="dayEvents(selectedDate).length === 0" class="sv__empty">{{ t('pub.share.noEvents') }}</p>
          <ul v-else class="sv__events">
            <li v-for="ev in dayEvents(selectedDate)" :key="ev.uid + ev.start" class="sv__event" :class="{ 'sv__event--cancelled': ev.status === 'cancelled' }">
              <span class="sv__event-time">{{ ev.all_day ? t('pub.share.allDay') : fmtTime(ev.start, share.timezone) }}</span>
              <span class="sv__event-name">{{ ev.summary }}</span>
            </li>
          </ul>
        </div>
      </div>

      <!-- 列表视图 -->
      <div v-else class="sv__panel">
        <p v-if="eventsLoading" class="sv__empty">{{ t('state.loading') }}</p>
        <p v-else-if="sortedDates.length === 0" class="sv__empty">{{ t('pub.share.noEvents') }}</p>
        <template v-else>
          <section v-for="date in sortedDates" :key="date" class="sv__list-day">
            <h3 class="sv__day-title">{{ humanDate(date, share.timezone, getLocale()) }}</h3>
            <ul class="sv__events">
              <li v-for="ev in byDate.get(date) ?? []" :key="ev.uid + ev.start" class="sv__event" :class="{ 'sv__event--cancelled': ev.status === 'cancelled' }">
                <span class="sv__event-time">{{ ev.all_day ? t('pub.share.allDay') : `${fmtTime(ev.start, share.timezone)}–${fmtTime(ev.end, share.timezone)}` }}</span>
                <span class="sv__event-name">{{ ev.summary }}</span>
                <span v-if="ev.location" class="sv__event-loc">{{ ev.location }}</span>
              </li>
            </ul>
          </section>
        </template>
      </div>
    </template>
  </PubShell>
</template>

<style scoped>
.sv__loading { text-align: center; color: var(--color-text-dim); padding: var(--space-16) 0; }
.sv__head {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-4);
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-left: 4px solid var(--cal-color, var(--color-accent));
  border-radius: var(--radius-l); padding: var(--space-5) var(--space-6); box-shadow: var(--shadow-panel);
  flex-wrap: wrap;
}
.sv__head-main { display: flex; align-items: center; gap: var(--space-4); min-width: 0; }
.sv__cal-dot { width: 12px; height: 12px; border-radius: var(--radius-full); background: var(--cal-color, var(--color-accent)); flex: none; }
.sv__title { margin: 0; font-size: 18px; }
.sv__owner { margin: var(--space-1) 0 0; color: var(--color-text-secondary); font-size: 13px; }
.sv__desc { margin: var(--space-4) 0 0; color: var(--color-text-secondary); }
.sv__toolbar { display: flex; justify-content: center; margin: var(--space-6) 0 var(--space-4); }
.sv__seg { display: inline-flex; border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s); overflow: hidden; background: var(--color-bg-panel); }
.sv__seg-btn { border: none; background: none; padding: 7px 18px; cursor: pointer; color: var(--color-text-secondary); font-size: 13px; }
.sv__seg-btn--on { background: var(--color-accent); color: #fff; }
.sv__panel {
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l); padding: var(--space-5) var(--space-6) var(--space-6); box-shadow: var(--shadow-panel);
}
.sv__month-bar { display: flex; align-items: center; gap: var(--space-3); justify-content: center; }
.sv__mb-btn { min-width: 34px; padding: 5px 10px; }
.sv__month-label { font-weight: 600; min-width: 120px; text-align: center; }
.sv__wd-row { display: grid; grid-template-columns: repeat(7, 1fr); margin-top: var(--space-4); }
.sv__wd { text-align: center; font-size: 12px; color: var(--color-text-dim); padding: var(--space-1) 0; }
.sv__grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; }
.sv__cell {
  position: relative; aspect-ratio: 1 / 0.8; border: 1px solid transparent; border-radius: var(--radius-s);
  background: var(--color-bg-panel); cursor: pointer; display: flex; flex-direction: column;
  align-items: center; justify-content: center; gap: 2px; padding: 2px;
}
.sv__cell:hover { border-color: var(--color-accent); }
.sv__cell--out { opacity: 0.35; }
.sv__cell--today .sv__day-num { color: var(--color-accent); font-weight: 700; }
.sv__cell--selected { background: var(--color-accent-soft); border-color: var(--color-accent); }
.sv__day-num { font-size: 13px; font-family: var(--font-mono); }
.sv__cell-dots {
  font-size: 10px; line-height: 1; padding: 2px 5px; border-radius: var(--radius-full);
  background: var(--cal-color, var(--color-accent)); color: #fff;
}
.sv__day { margin-top: var(--space-5); border-top: 1px solid var(--color-border-subtle); padding-top: var(--space-4); }
.sv__day-title { margin: 0 0 var(--space-3); font-size: 14px; }
.sv__empty { color: var(--color-text-dim); text-align: center; padding: var(--space-6) 0; margin: 0; }
.sv__list-day { margin-top: var(--space-4); }
.sv__list-day:first-of-type { margin-top: 0; }
.sv__events { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.sv__event {
  display: flex; align-items: baseline; gap: var(--space-3); padding: var(--space-2) var(--space-3);
  background: var(--color-bg-base); border-radius: var(--radius-s); flex-wrap: wrap;
}
.sv__event--cancelled .sv__event-name { text-decoration: line-through; color: var(--color-text-dim); }
.sv__event-time { font-family: var(--font-mono); font-size: 13px; color: var(--color-text-secondary); flex: none; }
.sv__event-name { font-weight: 500; }
.sv__event-loc { color: var(--color-text-dim); font-size: 12px; }
@media (max-width: 560px) {
  .sv__panel { padding: var(--space-4); }
  .sv__event { padding: var(--space-2); }
}
</style>
