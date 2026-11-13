<script setup lang="ts">
// 日程编辑器：新建与编辑共用；重复日程的保存/删除先询问作用范围（仅本次 / 整个系列）。
import { computed, ref, watch } from 'vue'
import dayjs from 'dayjs'
import { calApi, type Calendar } from '../../api/calendars'
import { buildRrulePayload, parseRrule, type RruleModel } from '../../composables/rrule'
import { allDayDateKey, allDayEndKeyInclusive, combineToUtc, todayKey, toTz } from '../../composables/useLocalTime'
import { errorText } from '../../composables/useApiError'
import { toast, toastError } from '../../composables/useToast'
import { fillParam } from './constants'
import { t } from '../../i18n'
import type { CalEvent } from '../../composables/eventLayout'
import DialogShell from './DialogShell.vue'
import ReminderField from './ReminderField.vue'
import RruleEditor from './RruleEditor.vue'

const props = defineProps<{
  visible: boolean
  calendars: Calendar[]
  event: CalEvent | null
  draft: { dateKey: string; minutes: number } | null
  tz: string
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void; (e: 'deleted'): void }>()

const isCreate = computed(() => props.event === null)
const readonly = computed(() => props.event !== null && props.event.cal.role === 'viewer')
const recurring = computed(() => {
  if (!props.event) return false
  return props.event.ev.recurring || parseRrule(props.event.ev.rrule) !== null
})

const summary = ref('')
const location = ref('')
const description = ref('')
const calendarId = ref<number | null>(null)
const allDay = ref(false)
const startDate = ref('')
const startTime = ref('09:00')
const endDate = ref('')
const endTime = ref('10:00')
const reminders = ref<number[]>([])
const rrule = ref<RruleModel | null>(null)

const submitting = ref(false)
const errText = ref('')
const scopeAsk = ref<'save' | 'delete' | null>(null)
const confirmDelete = ref(false)

const tzNote = computed(() => fillParam(t('ev.timezoneNote'), 'tz', props.tz))
const title = computed(() => t(isCreate.value ? 'ev.title.new' : 'ev.title.edit'))

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

function initForm() {
  errText.value = ''
  scopeAsk.value = null
  confirmDelete.value = false
  submitting.value = false
  if (props.event) {
    const ev = props.event.ev
    calendarId.value = props.event.cal.id
    summary.value = ev.summary
    location.value = ev.location
    description.value = ev.description
    reminders.value = [...(ev.reminders ?? [])]
    rrule.value = parseRrule(ev.rrule)
    allDay.value = ev.all_day
    if (ev.all_day) {
      startDate.value = allDayDateKey(ev.start)
      endDate.value = allDayEndKeyInclusive(ev.end)
      startTime.value = '09:00'
      endTime.value = '10:00'
    } else {
      const s = toTz(ev.start, props.tz)
      const e = toTz(ev.end, props.tz)
      startDate.value = s.format('YYYY-MM-DD')
      startTime.value = s.format('HH:mm')
      endDate.value = e.format('YYYY-MM-DD')
      endTime.value = e.format('HH:mm')
    }
    return
  }
  calendarId.value = props.calendars[0]?.id ?? null
  summary.value = ''
  location.value = ''
  description.value = ''
  reminders.value = []
  rrule.value = null
  allDay.value = false
  const d = props.draft ?? { dateKey: todayKey(), minutes: 9 * 60 }
  startDate.value = d.dateKey
  endDate.value = d.dateKey
  const total = Math.min(d.minutes, 23 * 60 + 30)
  startTime.value = `${pad(Math.floor(total / 60))}:${pad(total % 60)}`
  const endTotal = total + 60
  if (endTotal >= 1440) {
    endDate.value = dayjs(d.dateKey).add(1, 'day').format('YYYY-MM-DD')
    endTime.value = `${pad(Math.floor((endTotal - 1440) / 60))}:${pad((endTotal - 1440) % 60)}`
  } else {
    endTime.value = `${pad(Math.floor(endTotal / 60))}:${pad(endTotal % 60)}`
  }
}

watch(
  () => props.visible,
  v => {
    if (v) initForm()
  },
)

function splitHM(v: string): [number, number] {
  const [h, m] = v.split(':').map(x => parseInt(x, 10))
  return [Number.isFinite(h) ? h : 0, Number.isFinite(m) ? m : 0]
}

function validate(): string {
  if (!summary.value.trim()) return t('validation.summaryRequired')
  if (allDay.value) {
    if (endDate.value < startDate.value) return t('validation.endBeforeStart')
    return ''
  }
  const s = combineToUtc(startDate.value, ...splitHM(startTime.value), props.tz)
  const e = combineToUtc(endDate.value, ...splitHM(endTime.value), props.tz)
  if (e <= s) return t('validation.endBeforeStart')
  return ''
}

function buildPayload() {
  const common = {
    summary: summary.value.trim(),
    location: location.value.trim(),
    description: description.value.trim(),
    reminders: [...reminders.value],
    rrule: buildRrulePayload(rrule.value),
  }
  if (allDay.value) {
    return {
      ...common,
      all_day: true,
      start: startDate.value,
      end: dayjs(endDate.value).add(1, 'day').format('YYYY-MM-DD'),
    }
  }
  const [sh, sm] = splitHM(startTime.value)
  const [eh, em] = splitHM(endTime.value)
  return {
    ...common,
    all_day: false,
    tz: props.tz,
    start: combineToUtc(startDate.value, sh, sm, props.tz),
    end: combineToUtc(endDate.value, eh, em, props.tz),
  }
}

function close() {
  emit('update:visible', false)
}

async function save(scope?: 'single' | 'all') {
  if (readonly.value || submitting.value) return
  if (scopeAsk.value === 'save' && !scope) return
  if (!isCreate.value && recurring.value && !scope) {
    scopeAsk.value = 'save'
    return
  }
  const invalid = validate()
  if (invalid) {
    errText.value = invalid
    return
  }
  submitting.value = true
  errText.value = ''
  try {
    const payload = buildPayload()
    if (isCreate.value) {
      await calApi.createEvent(calendarId.value as number, payload)
    } else {
      await calApi.updateEvent(props.event!.ev.id, payload, scope ?? 'all')
    }
    toast(t('ev.saved'))
    emit('saved')
    close()
  } catch (err) {
    errText.value = errorText(err)
  } finally {
    submitting.value = false
  }
}

async function remove(scope?: 'single' | 'all') {
  if (readonly.value || isCreate.value || submitting.value) return
  if (!confirmDelete.value) {
    confirmDelete.value = true
    return
  }
  if (recurring.value && !scope) {
    scopeAsk.value = 'delete'
    return
  }
  submitting.value = true
  errText.value = ''
  try {
    await calApi.deleteEvent(props.event!.ev.id, scope ?? 'all')
    toast(t('ev.deleted'))
    emit('deleted')
    close()
  } catch (err) {
    errText.value = errorText(err)
    confirmDelete.value = false
    scopeAsk.value = null
    toastError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <DialogShell :visible="visible" :title="title" width="560px" @update:visible="v => (v ? null : close())">
    <div v-if="errText" class="ev-form__error" role="alert">{{ errText }}</div>
    <p v-if="readonly" class="ev-form__note">{{ t('ev.readonly') }}</p>

    <div class="ev-form">
      <label class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.summary') }}</span>
        <input v-model="summary" class="input" type="text" :disabled="readonly" />
      </label>

      <div class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.calendar') }}</span>
        <select v-if="isCreate" v-model.number="calendarId" class="input" :disabled="readonly || calendars.length < 2">
          <option v-for="c in calendars" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
        <span v-else class="ev-form__cal">
          <span class="ev-form__dot" :style="{ background: event?.cal.color }" />
          {{ event?.cal.name }}
          <span v-if="recurring" class="ev-form__badge">{{ t('ev.recurringBadge') }}</span>
        </span>
      </div>

      <div class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.start') }}</span>
        <div class="ev-form__when">
          <input v-model="startDate" class="input" type="date" :disabled="readonly" />
          <input v-if="!allDay" v-model="startTime" class="input" type="time" :disabled="readonly" />
          <label class="ev-form__allday"><input v-model="allDay" type="checkbox" :disabled="readonly" />{{ t('ev.allDay') }}</label>
        </div>
      </div>

      <div class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.end') }}</span>
        <div class="ev-form__when">
          <input v-model="endDate" class="input" type="date" :disabled="readonly" />
          <input v-if="!allDay" v-model="endTime" class="input" type="time" :disabled="readonly" />
        </div>
      </div>
      <p class="ev-form__tz">{{ tzNote }}</p>

      <label class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.location') }}</span>
        <input v-model="location" class="input" type="text" :disabled="readonly" />
      </label>

      <label class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.description') }}</span>
        <textarea v-model="description" class="input ev-form__desc" rows="3" :disabled="readonly" />
      </label>

      <div class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.repeat') }}</span>
        <RruleEditor v-model="rrule" :start-key="startDate" :disabled="readonly" />
      </div>

      <div class="ev-form__field">
        <span class="ev-form__label">{{ t('ev.reminders') }}</span>
        <ReminderField v-model="reminders" :disabled="readonly" />
      </div>
    </div>

    <div v-if="scopeAsk" class="ev-form__scope">
      <p>{{ t('ev.scope.prompt') }}</p>
      <div class="ev-form__scope-btns">
        <button type="button" class="btn" @click="scopeAsk = null">{{ t('action.cancel') }}</button>
        <button type="button" class="btn" @click="scopeAsk === 'save' ? save('single') : remove('single')">{{ t('ev.scope.single') }}</button>
        <button type="button" class="btn btn-primary" @click="scopeAsk === 'save' ? save('all') : remove('all')">{{ t('ev.scope.all') }}</button>
      </div>
    </div>

    <template #footer>
      <template v-if="!scopeAsk">
        <button v-if="confirmDelete" type="button" class="btn" @click="confirmDelete = false">{{ t('action.cancel') }}</button>
        <button v-if="!isCreate && !readonly" type="button" class="btn btn-danger" @click="remove()">
          {{ confirmDelete ? t('action.confirm') : t('action.delete') }}
        </button>
        <span v-if="confirmDelete" class="ev-form__confirm-text">{{ t('ev.deleteConfirm') }}</span>
        <span v-else class="ev-form__spacer" />
        <button type="button" class="btn" @click="close">{{ t('action.cancel') }}</button>
        <button v-if="!readonly" type="button" class="btn btn-primary" :disabled="submitting" @click="save()">{{ t('action.save') }}</button>
      </template>
    </template>
  </DialogShell>
</template>

<style scoped>
.ev-form { display: flex; flex-direction: column; gap: var(--space-3); }
.ev-form__field { display: flex; flex-direction: column; gap: var(--space-1); }
.ev-form__label { font-size: 12px; color: var(--color-text-secondary); }
.ev-form__when { display: flex; align-items: center; gap: var(--space-2); }
.ev-form__when .input { width: auto; flex: 1; }
.ev-form__allday { display: inline-flex; align-items: center; gap: var(--space-1); font-size: 13px; color: var(--color-text-secondary); white-space: nowrap; }
.ev-form__allday input { accent-color: var(--color-accent); }
.ev-form__tz { margin: -4px 0 0; font-size: 12px; color: var(--color-text-dim); }
.ev-form__desc { resize: vertical; font-family: var(--font-body); }
.ev-form__cal { display: inline-flex; align-items: center; gap: var(--space-2); font-size: 14px; padding: 6px 0; }
.ev-form__dot { width: 10px; height: 10px; border-radius: var(--radius-full); display: inline-block; }
.ev-form__badge {
  font-size: 11px; padding: 1px 8px; border-radius: var(--radius-full);
  background: var(--color-bg-sunken); color: var(--color-text-secondary);
}
.ev-form__error {
  margin-bottom: var(--space-3); padding: var(--space-2) var(--space-3);
  background: var(--color-danger-soft); color: var(--color-danger);
  border-radius: var(--radius-s); font-size: 13px;
}
.ev-form__note { margin: 0 0 var(--space-3); font-size: 13px; color: var(--color-warning); }
.ev-form__scope {
  margin-top: var(--space-3); padding: var(--space-3);
  background: var(--color-accent-soft); border-radius: var(--radius-s);
  display: flex; flex-direction: column; gap: var(--space-2);
}
.ev-form__scope p { margin: 0; font-size: 13px; color: var(--color-text-primary); }
.ev-form__scope-btns { display: flex; justify-content: flex-end; gap: var(--space-2); }
.ev-form__spacer { flex: 1; }
.ev-form__confirm-text { font-size: 13px; color: var(--color-danger); align-self: center; }
</style>
