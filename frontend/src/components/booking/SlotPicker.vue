<script setup lang="ts">
// 可复用槽位选择器：日期条（无槽位日期置灰）+ 槽位网格 + 时区选择。
// 预约页与预约管理页改期共用；slot_unavailable 后由父组件调 refresh()。
import { computed, onMounted, ref, watch } from 'vue'
import { pubApi } from '../../api/pub'
import { ApiError } from '../../api'
import { apiErrorText } from '../../i18n/pub'
import { t, getLocale } from '../../i18n'
import SlotGrid from './SlotGrid.vue'
import { dayHasAvailable, addDays, todayInTz, weekdayOf, humanDate, type SlotDay, type SlotItem } from './slots'
import { browserTimezone, timezoneOptions, tzOffsetLabel } from './timezone'

const props = withDefaults(defineProps<{
  username: string
  slug: string
  /** 浏览边界：minDate/maxDate 为 YYYY-MM-DD；缺省为今天起不设上限 */
  minDate?: string
  maxDate?: string
  showTz?: boolean
  selectedStart?: string | null
  /** 初始时区（默认浏览器时区） */
  initialTz?: string
}>(), { showTz: true })

const emit = defineEmits<{
  (e: 'select', slot: SlotItem): void
  (e: 'tz-change', tz: string): void
}>()

const WINDOW = 14
const STEP = 7

const tz = ref(props.initialTz || browserTimezone())
const windowStart = ref('')
const days = ref<SlotDay[]>([])
const loading = ref(false)
const error = ref('')
const selectedDate = ref('')

const tzChoices = computed(() => timezoneOptions(tz.value, browserTimezone()))
const bTz = computed(() => browserTimezone())
function tzLabel(z: string): string {
  const local = z === bTz.value ? ` · ${t('pub.tzLocal')}` : ''
  return `${z}（${tzOffsetLabel(z)}）${local}`
}
const today = computed(() => todayInTz(tz.value))

function clampStart(start: string): string {
  const min = props.minDate && props.minDate > today.value ? props.minDate : today.value
  if (start < min) return min
  if (props.maxDate && start > props.maxDate) return props.maxDate
  return start
}

const canPrev = computed(() => {
  const prev = addDays(windowStart.value, -STEP)
  const min = props.minDate && props.minDate > today.value ? props.minDate : today.value
  return addDays(prev, WINDOW - 1) >= min && windowStart.value > min
})
const canNext = computed(() => props.maxDate ? addDays(windowStart.value, STEP) <= props.maxDate : true)

let fetchSeq = 0

async function fetchDays(): Promise<void> {
  if (!windowStart.value) return
  const seq = ++fetchSeq
  loading.value = true
  error.value = ''
  try {
    const res = await pubApi.slots(props.username, props.slug, windowStart.value, WINDOW, tz.value)
    if (seq !== fetchSeq) return // 过期响应（已切换时区/窗口），丢弃
    days.value = res?.days ?? []
    // 保持或回落选中日期
    const cur = days.value.find(d => d.date === selectedDate.value)
    if (!cur || !dayHasAvailable(cur)) {
      selectedDate.value = days.value.find(d => dayHasAvailable(d))?.date ?? ''
    }
  } catch (err) {
    if (seq !== fetchSeq) return
    error.value = err instanceof ApiError ? apiErrorText(err) : apiErrorText(null)
    days.value = []
    selectedDate.value = ''
  } finally {
    if (seq === fetchSeq) loading.value = false
  }
}

function reset(): void {
  windowStart.value = clampStart(today.value)
  selectedDate.value = ''
  void fetchDays()
}

function shift(dir: -1 | 1): void {
  windowStart.value = clampStart(addDays(windowStart.value, dir * STEP))
  void fetchDays()
}

function refresh(): void {
  void fetchDays()
}

function onSlot(slot: SlotItem): void {
  emit('select', slot)
}

function onTzChange(v: Event): void {
  tz.value = (v.target as HTMLSelectElement).value
  emit('tz-change', tz.value)
  reset()
}

const selectedDay = computed(() => days.value.find(d => d.date === selectedDate.value))
const anyAvailable = computed(() => days.value.some(d => dayHasAvailable(d)))

watch(() => props.minDate, () => { if (windowStart.value && windowStart.value < (props.minDate ?? '')) reset() })
onMounted(reset)

defineExpose({ refresh })
</script>

<template>
  <div class="picker">
    <div v-if="showTz" class="picker__tz">
      <label class="picker__tz-label" for="picker-tz">{{ t('pub.tzLabel') }}</label>
      <select id="picker-tz" class="input picker__tz-select" :value="tz" @change="onTzChange">
        <option v-for="z in tzChoices" :key="z" :value="z">{{ tzLabel(z) }}</option>
      </select>
    </div>

    <div class="picker__strip-row">
      <button class="btn picker__nav" type="button" :disabled="!canPrev" @click="shift(-1)" :aria-label="t('pub.prevRange')">‹</button>
      <div class="picker__strip" role="listbox" :aria-label="t('pub.step.date')">
        <button
          v-for="d in days" :key="d.date" type="button" role="option"
          :aria-selected="d.date === selectedDate"
          class="picker__day" :class="{ 'picker__day--selected': d.date === selectedDate }"
          :disabled="!dayHasAvailable(d)" @click="selectedDate = d.date"
        >
          <span class="picker__day-wd">{{ t(`pub.wd.${weekdayOf(d.date)}`) }}</span>
          <span class="picker__day-num">{{ d.date.slice(8, 10) }}</span>
        </button>
      </div>
      <button class="btn picker__nav" type="button" :disabled="!canNext" @click="shift(1)" :aria-label="t('pub.nextRange')">›</button>
    </div>

    <p v-if="loading" class="picker__hint">{{ t('state.loading') }}</p>
    <p v-else-if="error" class="picker__error">{{ error }}</p>
    <template v-else>
      <p v-if="!anyAvailable" class="picker__hint">{{ t('pub.noSlotsInRange') }}</p>
      <template v-else>
        <div v-if="selectedDate" class="picker__date-head">{{ humanDate(selectedDate, tz, getLocale()) }}</div>
        <SlotGrid
          :slots="selectedDay?.slots ?? []" :tz="tz" :selected-start="selectedStart"
          @select="onSlot"
        />
      </template>
    </template>
  </div>
</template>

<style scoped>
.picker { display: flex; flex-direction: column; gap: var(--space-4); }
.picker__tz { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
.picker__tz-label { color: var(--color-text-secondary); font-size: 13px; }
.picker__tz-select { width: auto; min-width: 240px; padding: 6px 10px; }
.picker__strip-row { display: flex; align-items: stretch; gap: var(--space-2); }
.picker__nav { flex: none; width: 34px; padding: 0; }
.picker__strip {
  flex: 1; display: grid; grid-auto-flow: column; grid-auto-columns: minmax(44px, 1fr);
  gap: var(--space-1); overflow-x: auto; padding-bottom: var(--space-1);
}
.picker__day {
  display: flex; flex-direction: column; align-items: center; gap: 2px;
  padding: var(--space-2) 0; border-radius: var(--radius-s);
  border: 1px solid transparent; background: var(--color-bg-panel); cursor: pointer;
}
.picker__day:hover:not(:disabled) { border-color: var(--color-accent); }
.picker__day:disabled { opacity: 0.3; cursor: default; }
.picker__day--selected { background: var(--color-accent-soft); border-color: var(--color-accent); }
.picker__day-wd { font-size: 11px; color: var(--color-text-dim); }
.picker__day-num { font-size: 15px; font-weight: 600; font-family: var(--font-mono); }
.picker__hint, .picker__error { text-align: center; padding: var(--space-4) 0; margin: 0; }
.picker__hint { color: var(--color-text-dim); }
.picker__error { color: var(--color-danger); }
.picker__date-head { font-weight: 600; margin: var(--space-2) 0 0; }
@media (max-width: 560px) {
  .picker__tz-select { min-width: 0; flex: 1; }
}
</style>
