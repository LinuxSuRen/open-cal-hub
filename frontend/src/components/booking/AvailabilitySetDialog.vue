<script setup lang="ts">
// 可用时间集编辑器：每周规则网格（周一~周日，多时段）+ 日期特例 + 默认集标记。
// 新建与编辑共用：editing 为 null 表示新建；保存成功后 emit saved 由父级刷新。
import { reactive, ref, watch } from 'vue'
import { bookingApi, type AvailabilitySet } from '../../api/booking'
import { ApiError } from '../../api'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { showToast } from '../ds/toast'
import Modal from '../ds/Modal.vue'
import { HHmmToMinutes, minutesToHHmm } from './slots'
import { browserTimezone, timezoneOptions, tzOffsetLabel } from './timezone'

const props = defineProps<{ visible: boolean; editing: AvailabilitySet | null }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

interface SpanDraft { start: string; end: string }
interface DateDraft { date: string; kind: 'available' | 'unavailable'; useTime: boolean; start: string; end: string }

const WEEK_ORDER = [1, 2, 3, 4, 5, 6, 0]

const draft = reactive({
  name: '',
  timezone: browserTimezone(),
  isDefault: false,
  weekly: {} as Record<number, SpanDraft[]>,
  dates: [] as DateDraft[],
})
const saving = ref(false)
const errors = ref<string[]>([])

function reset(): void {
  errors.value = []
  draft.name = props.editing?.name ?? ''
  draft.timezone = props.editing?.timezone ?? browserTimezone()
  draft.isDefault = props.editing?.is_default ?? false
  draft.weekly = {}
  for (const wd of WEEK_ORDER) {
    draft.weekly[wd] = (props.editing?.rules ?? [])
      .filter(r => r.weekday === wd)
      .map(r => ({ start: minutesToHHmm(r.start_minute), end: minutesToHHmm(r.end_minute) }))
  }
  draft.dates = (props.editing?.dates ?? []).map(d => ({
    date: d.date,
    kind: d.kind,
    useTime: !(d.start_minute === 0 && d.end_minute >= 1440),
    start: minutesToHHmm(d.start_minute),
    end: minutesToHHmm(d.end_minute >= 1440 ? 1439 : d.end_minute),
  }))
}

watch(() => props.visible, v => { if (v) reset() }, { immediate: true })

function addSpan(wd: number): void {
  draft.weekly[wd] = draft.weekly[wd] ?? []
  const last = draft.weekly[wd][draft.weekly[wd].length - 1]
  const start = last ? last.end : '09:00'
  const s = HHmmToMinutes(start) ?? 540
  draft.weekly[wd].push({ start, end: minutesToHHmm(Math.min(1439, s + 60)) })
}
function removeSpan(wd: number, i: number): void {
  draft.weekly[wd]?.splice(i, 1)
}
function addDate(): void {
  draft.dates.push({ date: '', kind: 'unavailable', useTime: false, start: '09:00', end: '17:00' })
}
function removeDate(i: number): void {
  draft.dates.splice(i, 1)
}

function validate(): string[] {
  const errs: string[] = []
  if (!draft.name.trim()) errs.push(t('pub.form.required') + ` · ${t('booking.avail.name')}`)
  for (const wd of WEEK_ORDER) {
    const spans = (draft.weekly[wd] ?? []).map(s => ({ s: HHmmToMinutes(s.start), e: HHmmToMinutes(s.end) }))
    if (spans.some(x => x.s === null || x.e === null || x.s >= x.e)) {
      errs.push(`${t(`booking.weekday.${wd}`)}: ${t('booking.avail.spanOrder')}`)
      continue
    }
    const sorted = [...spans].sort((a, b) => (a.s ?? 0) - (b.s ?? 0))
    for (let i = 1; i < sorted.length; i++) {
      if ((sorted[i].s ?? 0) < (sorted[i - 1].e ?? 0)) {
        errs.push(`${t(`booking.weekday.${wd}`)}: ${t('booking.avail.overlap')}`)
        break
      }
    }
  }
  if (draft.dates.some(d => !d.date)) errs.push(t('pub.form.required') + ` · ${t('booking.avail.dates')}`)
  return errs
}

async function save(): Promise<void> {
  errors.value = validate()
  if (errors.value.length > 0) return
  const rules = WEEK_ORDER.flatMap(wd =>
    (draft.weekly[wd] ?? []).map(s => ({
      weekday: wd,
      start_minute: HHmmToMinutes(s.start) ?? 0,
      end_minute: HHmmToMinutes(s.end) ?? 0,
    })),
  )
  const dates = draft.dates.map(d => ({
    date: d.date,
    kind: d.kind,
    start_minute: d.useTime ? (HHmmToMinutes(d.start) ?? 0) : 0,
    end_minute: d.useTime ? (HHmmToMinutes(d.end) ?? 1440) : 1440,
  }))
  saving.value = true
  try {
    const payload = { name: draft.name.trim(), timezone: draft.timezone, is_default: draft.isDefault, rules, dates }
    if (props.editing) await bookingApi.updateAvailabilitySet(props.editing.id, payload)
    else await bookingApi.createAvailabilitySet(payload)
    showToast(t('booking.avail.saved'), 'success')
    emit('saved')
    emit('update:visible', false)
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Modal :visible="visible" :title="t('booking.cfg.tabAvail')" :width="'680px'" @update:visible="emit('update:visible', $event)">
    <div class="avform">
      <div class="avform__row">
        <label class="field">
          <span class="field__label">{{ t('booking.avail.name') }} *</span>
          <input v-model="draft.name" class="input" :placeholder="t('booking.avail.namePh')" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.avail.tz') }}</span>
          <select v-model="draft.timezone" class="input">
            <option v-for="z in timezoneOptions(draft.timezone)" :key="z" :value="z">{{ z }}（{{ tzOffsetLabel(z) }}）</option>
          </select>
        </label>
      </div>
      <label class="check">
        <input v-model="draft.isDefault" type="checkbox" />
        <span>{{ t('booking.avail.markDefault') }}</span>
      </label>

      <h4 class="avform__sec">{{ t('booking.avail.weekly') }}</h4>
      <div class="weekly">
        <div v-for="wd in WEEK_ORDER" :key="wd" class="weekly__row">
          <div class="weekly__day">{{ t(`booking.weekday.${wd}`) }}</div>
          <div class="weekly__spans">
            <div v-if="!draft.weekly[wd] || draft.weekly[wd].length === 0" class="weekly__empty">{{ t('booking.rulesEmpty') }}</div>
            <div v-for="(span, i) in draft.weekly[wd] ?? []" :key="i" class="weekly__span">
              <input v-model="span.start" type="time" step="300" class="input weekly__time" />
              <span class="weekly__dash">–</span>
              <input v-model="span.end" type="time" step="300" class="input weekly__time" />
              <button class="weekly__del" type="button" @click="removeSpan(wd, i)">✕</button>
            </div>
            <button class="btn weekly__add" type="button" @click="addSpan(wd)">{{ t('booking.avail.addSpan') }}</button>
          </div>
        </div>
      </div>

      <h4 class="avform__sec">{{ t('booking.avail.dates') }}</h4>
      <div v-for="(d, i) in draft.dates" :key="i" class="exdate">
        <input v-model="d.date" type="date" class="input exdate__date" />
        <select v-model="d.kind" class="input exdate__kind">
          <option value="unavailable">{{ t('booking.avail.kind.unavailable') }}</option>
          <option value="available">{{ t('booking.avail.kind.available') }}</option>
        </select>
        <label class="check check--inline">
          <input v-model="d.useTime" type="checkbox" />
          <span>{{ t('booking.avail.timeRange') }}</span>
        </label>
        <template v-if="d.useTime">
          <input v-model="d.start" type="time" step="300" class="input exdate__time" />
          <span class="weekly__dash">–</span>
          <input v-model="d.end" type="time" step="300" class="input exdate__time" />
        </template>
        <button class="weekly__del" type="button" @click="removeDate(i)">✕</button>
      </div>
      <button class="btn weekly__add" type="button" @click="addDate">{{ t('booking.avail.addDate') }}</button>

      <ul v-if="errors.length" class="avform__errors">
        <li v-for="(e, i) in errors" :key="i">{{ e }}</li>
      </ul>
    </div>
    <template #footer>
      <button class="btn" type="button" @click="emit('update:visible', false)">{{ t('action.cancel') }}</button>
      <button class="btn btn-primary" type="button" :disabled="saving" @click="save">{{ t('action.save') }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.avform { display: flex; flex-direction: column; gap: var(--space-4); }
.avform__row { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.field { display: flex; flex-direction: column; gap: var(--space-1); }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
.check { display: flex; align-items: center; gap: var(--space-2); color: var(--color-text-secondary); }
.check--inline { white-space: nowrap; }
.avform__sec { margin: var(--space-2) 0 0; font-size: 14px; }
.weekly { display: flex; flex-direction: column; gap: var(--space-2); }
.weekly__row { display: grid; grid-template-columns: 56px 1fr; gap: var(--space-3); align-items: start; }
.weekly__day { padding-top: 7px; color: var(--color-text-secondary); font-size: 13px; }
.weekly__spans { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; }
.weekly__empty { color: var(--color-text-dim); font-size: 13px; }
.weekly__span { display: flex; align-items: center; gap: var(--space-1); }
.weekly__time { width: 104px; padding: 6px 8px; font-family: var(--font-mono); }
.weekly__dash { color: var(--color-text-dim); }
.weekly__add { padding: 5px 12px; font-size: 13px; }
.weekly__del {
  border: none; background: none; cursor: pointer; color: var(--color-text-dim);
  padding: 2px 6px; border-radius: var(--radius-s);
}
.weekly__del:hover { color: var(--color-danger); background: var(--color-danger-soft); }
.exdate { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
.exdate__date, .exdate__kind { width: auto; }
.exdate__date { font-family: var(--font-mono); }
.exdate__time { width: 104px; padding: 6px 8px; font-family: var(--font-mono); }
.avform__errors { margin: 0; padding: var(--space-3) var(--space-4); background: var(--color-danger-soft); color: var(--color-danger); border-radius: var(--radius-s); list-style: none; }
.avform__errors li + li { margin-top: 4px; }
@media (max-width: 560px) {
  .avform__row { grid-template-columns: 1fr; }
}
</style>
