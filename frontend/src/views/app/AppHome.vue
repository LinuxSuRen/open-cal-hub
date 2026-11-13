<script setup lang="ts">
// 工作台：问候 + 今日与未来 7 天多日历合并日程 + 快速跳转。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import dayjs from 'dayjs'
import { calApi, type Calendar } from '../../api/calendars'
import { authState } from '../../stores/auth'
import { loadEvents } from '../../composables/useCalendarEvents'
import { groupByDay, type CalEvent } from '../../composables/eventLayout'
import { fmtTime, nowInUserTz, todayKey, tzDate, userTimezone } from '../../composables/useLocalTime'
import { errorText } from '../../composables/useApiError'
import { fillParam } from '../../components/calendar/constants'
import { t } from '../../i18n'

const router = useRouter()

const calendars = ref<Calendar[]>([])
const items = ref<CalEvent[]>([])
const loading = ref(false)
const error = ref<unknown>(null)

const tz = computed(() => userTimezone())
const today = todayKey()

const greeting = computed(() => {
  const h = nowInUserTz().hour()
  return t(h < 12 ? 'home.greeting.morning' : h < 18 ? 'home.greeting.afternoon' : 'home.greeting.evening')
})

const groups = computed(() => {
  const map = groupByDay(items.value, tz.value)
  return Array.from({ length: 8 }, (_, i) => {
    const key = dayjs(today).add(i, 'day').format('YYYY-MM-DD')
    const d = dayjs(key)
    const label =
      i === 0 ? t('home.today') : i === 1 ? t('home.tomorrow') : `${d.format('M月D日')} ${t(`date.weekday.${d.day()}`)}`
    return { key, label, list: map.get(key) ?? [] }
  })
})
const totalCount = computed(() => items.value.length)

function rowTime(item: CalEvent): string {
  return item.ev.all_day ? t('home.allDay') : fmtTime(item.ev.start)
}

function newEvent() {
  router.push({ path: '/app/calendar', query: { view: 'day', date: today } })
}

onMounted(async () => {
  loading.value = true
  try {
    const list = await calApi.list()
    calendars.value = Array.isArray(list) ? list : []
    if (calendars.value.length > 0) {
      const start = tzDate(today, tz.value)
      const from = start.utc().format()
      const to = start.add(8, 'day').utc().format()
      items.value = await loadEvents(calendars.value, from, to)
    }
  } catch (err) {
    error.value = err
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page home">
    <header class="home__head">
      <div>
        <h1 class="home__title">{{ greeting }}，{{ authState.user?.display_name || authState.user?.username }}</h1>
        <p class="home__sub">{{ t('home.upcoming') }}<template v-if="totalCount > 0"> · {{ fillParam(t('home.eventCount'), 'n', totalCount) }}</template></p>
      </div>
      <div class="home__actions">
        <button type="button" class="btn btn-primary" @click="newEvent">{{ t('home.newEvent') }}</button>
        <router-link class="btn" to="/app/calendar">{{ t('home.goCalendar') }}</router-link>
        <router-link class="btn" to="/app/calendars">{{ t('home.goCalendars') }}</router-link>
      </div>
    </header>

    <div v-if="error !== null" class="home__error" role="alert">{{ errorText(error) }}</div>
    <p v-else-if="loading">{{ t('state.loading') }}</p>

    <section v-for="g in totalCount > 0 ? groups : []" :key="g.key" class="home__group">
      <h2 class="home__group-title">{{ g.label }}</h2>
      <ul v-if="g.list.length" class="home__list">
        <li v-for="item in g.list" :key="item.ev.id + item.ev.start" class="home__row">
          <span class="home__row-time">{{ rowTime(item) }}</span>
          <span class="home__row-dot" :style="{ background: item.cal.color }" />
          <span class="home__row-summary">{{ item.ev.summary }}</span>
          <span class="home__row-cal">{{ item.cal.name }}</span>
          <span v-if="item.ev.location" class="home__row-loc">{{ item.ev.location }}</span>
        </li>
      </ul>
      <p v-else class="home__none">{{ t('cal.emptyDay') }}</p>
    </section>

    <div v-if="!loading && error === null && totalCount === 0" class="home__empty">
      <p>{{ t('home.empty') }}</p>
      <button type="button" class="btn btn-primary" @click="newEvent">{{ t('home.newEvent') }}</button>
    </div>
  </div>
</template>

<style scoped>
.home { max-width: 860px; display: flex; flex-direction: column; gap: var(--space-5); }
.home__head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-4); flex-wrap: wrap; }
.home__title { margin: 0; font-size: 20px; }
.home__sub { margin: var(--space-1) 0 0; font-size: 13px; color: var(--color-text-secondary); }
.home__actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
.home__error { padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px; }
.home__group-title { margin: 0 0 var(--space-1); font-size: 13px; color: var(--color-text-secondary); }
.home__list { list-style: none; margin: 0; padding: 0; background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m); }
.home__row { display: flex; align-items: baseline; gap: var(--space-3); padding: var(--space-2) var(--space-4); }
.home__row + .home__row { border-top: 1px solid var(--color-border-subtle); }
.home__row-time { font-family: var(--font-mono); font-size: 12px; color: var(--color-text-secondary); width: 48px; flex-shrink: 0; }
.home__row-dot { width: 10px; height: 10px; border-radius: var(--radius-full); flex-shrink: 0; align-self: center; }
.home__row-summary { font-size: 14px; }
.home__row-cal { font-size: 12px; color: var(--color-text-dim); }
.home__row-loc { font-size: 12px; color: var(--color-text-dim); margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.home__none { margin: 0; font-size: 12px; color: var(--color-text-dim); }
.home__empty { display: flex; flex-direction: column; align-items: center; gap: var(--space-4); padding: var(--space-16) 0; color: var(--color-text-dim); }
</style>
