<script setup lang="ts">
// 预约类型编辑器：新建/编辑共用。名称/标识/时长/地点/颜色/缓冲/间隔/提前通知/日期范围/
// 可用时间集/目标日历（只读引用 calApi.list 数据由父级传入）/启用开关。
import { reactive, ref, watch } from 'vue'
import { bookingApi, type AvailabilitySet, type EventType } from '../../api/booking'
import { ApiError } from '../../api'
import type { Calendar } from '../../api/calendars'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { showToast } from '../ds/toast'
import Modal from '../ds/Modal.vue'

const props = defineProps<{
  visible: boolean
  editing: EventType | null
  calendars: Calendar[]
  sets: AvailabilitySet[]
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

const PALETTE = ['#4F6EF7', '#1F9D61', '#B97E0D', '#D5484F', '#7C5CE0', '#0E9AA7', '#5B7DB1', '#C25E8A']
const LOCATION_TYPES = ['none', 'online', 'in_person', 'phone'] as const

const draft = reactive({
  name: '',
  slug: '',
  slugTouched: false,
  description: '',
  duration: 30,
  locationType: 'none' as (typeof LOCATION_TYPES)[number],
  locationValue: '',
  color: PALETTE[0],
  bufferBefore: 0,
  bufferAfter: 0,
  slotInterval: 30,
  minNotice: 60,
  rangeType: 'rolling' as 'rolling' | 'fixed',
  rangeDays: 30,
  startDate: '',
  endDate: '',
  availabilityId: 0,
  calendarId: 0,
  active: true,
})
const saving = ref(false)
const errors = ref<string[]>([])

function reset(): void {
  errors.value = []
  const e = props.editing
  draft.name = e?.name ?? ''
  draft.slug = e?.slug ?? ''
  draft.slugTouched = !!e
  draft.description = e?.description ?? ''
  draft.duration = e?.duration_minutes ?? 30
  draft.locationType = e?.location_type ?? 'none'
  draft.locationValue = e?.location_value ?? ''
  draft.color = e?.color ?? PALETTE[0]
  draft.bufferBefore = e?.buffer_before ?? 0
  draft.bufferAfter = e?.buffer_after ?? 0
  draft.slotInterval = e?.slot_interval ?? 30
  draft.minNotice = e?.min_notice_minutes ?? 60
  draft.rangeType = e?.range_type ?? 'rolling'
  draft.rangeDays = e?.range_days ?? 30
  draft.startDate = e?.start_date ?? ''
  draft.endDate = e?.end_date ?? ''
  draft.availabilityId = e?.availability_id ?? props.sets[0]?.id ?? 0
  draft.calendarId = e?.calendar_id ?? props.calendars[0]?.id ?? 0
  draft.active = e?.active ?? true
}

watch(() => props.visible, v => { if (v) reset() }, { immediate: true })

function onNameInput(): void {
  if (!draft.slugTouched) {
    draft.slug = draft.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64)
  }
}

function validate(): string[] {
  const errs: string[] = []
  if (!draft.name.trim()) errs.push(`${t('booking.types.name')}: ${t('pub.form.required')}`)
  if (!/^[a-z0-9-]{1,64}$/.test(draft.slug)) errs.push(`${t('booking.types.slug')}: ${t('booking.types.slugInvalid')}`)
  if (draft.duration < 5 || draft.duration > 600) errs.push(`${t('booking.types.duration')}: 5-600`)
  if (draft.bufferBefore < 0 || draft.bufferBefore > 240 || draft.bufferAfter < 0 || draft.bufferAfter > 240) errs.push(`${t('booking.types.bufferBefore')}: 0-240`)
  if (draft.slotInterval < 5 || draft.slotInterval > 240) errs.push(`${t('booking.types.slotInterval')}: 5-240`)
  if (draft.minNotice < 0) errs.push(`${t('booking.types.minNotice')}: ≥ 0`)
  if (draft.rangeType === 'rolling' && (draft.rangeDays < 1 || draft.rangeDays > 365)) errs.push(`${t('booking.types.rangeDays')}: 1-365`)
  if (draft.rangeType === 'fixed') {
    if (!draft.startDate || !draft.endDate) errs.push(`${t('booking.types.range')}: ${t('pub.form.required')}`)
    else if (draft.startDate > draft.endDate) errs.push(t('booking.types.dateOrder'))
  }
  if (!draft.availabilityId) errs.push(`${t('booking.types.availability')}: ${t('pub.form.required')}`)
  if (!draft.calendarId) errs.push(`${t('booking.types.calendar')}: ${t('pub.form.required')}`)
  return errs
}

async function save(): Promise<void> {
  errors.value = validate()
  if (errors.value.length > 0) return
  saving.value = true
  try {
    const payload = {
      name: draft.name.trim(),
      slug: draft.slug,
      description: draft.description,
      duration_minutes: draft.duration,
      location_type: draft.locationType,
      location_value: draft.locationType === 'none' ? '' : draft.locationValue,
      color: draft.color,
      buffer_before: draft.bufferBefore,
      buffer_after: draft.bufferAfter,
      slot_interval: draft.slotInterval,
      min_notice_minutes: draft.minNotice,
      range_type: draft.rangeType,
      range_days: draft.rangeDays,
      start_date: draft.rangeType === 'fixed' ? draft.startDate : null,
      end_date: draft.rangeType === 'fixed' ? draft.endDate : null,
      availability_id: draft.availabilityId,
      calendar_id: draft.calendarId,
      active: draft.active,
    }
    if (props.editing) await bookingApi.updateEventType(props.editing.id, payload)
    else await bookingApi.createEventType(payload)
    showToast(t('booking.types.saved'), 'success')
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
  <Modal :visible="visible" :title="t('booking.cfg.tabTypes')" :width="'640px'" @update:visible="emit('update:visible', $event)">
    <div class="etform">
      <div class="etform__grid2">
        <label class="field">
          <span class="field__label">{{ t('booking.types.name') }} *</span>
          <input v-model="draft.name" class="input" @input="onNameInput" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.slug') }} *</span>
          <input v-model="draft.slug" class="input etform__mono" :placeholder="t('booking.types.slugPh')" @input="draft.slugTouched = true" />
        </label>
      </div>
      <label class="field">
        <span class="field__label">{{ t('settings.profile.bio') }}</span>
        <textarea v-model="draft.description" class="input etform__desc" rows="2" />
      </label>
      <div class="etform__grid2">
        <label class="field">
          <span class="field__label">{{ t('booking.types.duration') }}</span>
          <input v-model.number="draft.duration" type="number" min="5" max="600" step="5" class="input etform__mono" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.color') }}</span>
          <div class="etform__colors">
            <button
              v-for="c in PALETTE" :key="c" type="button" class="etform__swatch"
              :class="{ 'etform__swatch--active': draft.color === c }" :style="{ background: c }"
              :aria-label="c" @click="draft.color = c"
            />
            <input v-model="draft.color" class="input etform__mono etform__color-input" />
          </div>
        </label>
      </div>
      <div class="etform__grid2">
        <label class="field">
          <span class="field__label">{{ t('booking.types.location') }}</span>
          <select v-model="draft.locationType" class="input">
            <option v-for="lt in LOCATION_TYPES" :key="lt" :value="lt">{{ t(`booking.types.location.${lt}`) }}</option>
          </select>
        </label>
        <label v-if="draft.locationType !== 'none'" class="field">
          <span class="field__label">{{ t('booking.types.locationValue') }}</span>
          <input v-model="draft.locationValue" class="input" :placeholder="t('booking.types.locationPh')" />
        </label>
      </div>
      <div class="etform__grid4">
        <label class="field">
          <span class="field__label">{{ t('booking.types.bufferBefore') }}</span>
          <input v-model.number="draft.bufferBefore" type="number" min="0" max="240" step="5" class="input etform__mono" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.bufferAfter') }}</span>
          <input v-model.number="draft.bufferAfter" type="number" min="0" max="240" step="5" class="input etform__mono" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.slotInterval') }}</span>
          <input v-model.number="draft.slotInterval" type="number" min="5" max="240" step="5" class="input etform__mono" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.minNotice') }}</span>
          <input v-model.number="draft.minNotice" type="number" min="0" step="15" class="input etform__mono" />
        </label>
      </div>
      <fieldset class="etform__range">
        <legend class="field__label">{{ t('booking.types.range') }}</legend>
        <div class="etform__range-body">
          <div class="etform__seg">
            <button type="button" class="etform__seg-btn" :class="{ 'etform__seg-btn--on': draft.rangeType === 'rolling' }" @click="draft.rangeType = 'rolling'">{{ t('booking.types.range.rolling') }}</button>
            <button type="button" class="etform__seg-btn" :class="{ 'etform__seg-btn--on': draft.rangeType === 'fixed' }" @click="draft.rangeType = 'fixed'">{{ t('booking.types.range.fixed') }}</button>
          </div>
          <label v-if="draft.rangeType === 'rolling'" class="field">
            <span class="field__label">{{ t('booking.types.rangeDays') }}</span>
            <input v-model.number="draft.rangeDays" type="number" min="1" max="365" class="input etform__mono" />
          </label>
          <template v-else>
            <label class="field">
              <span class="field__label">{{ t('booking.types.startDate') }}</span>
              <input v-model="draft.startDate" type="date" class="input etform__mono" />
            </label>
            <label class="field">
              <span class="field__label">{{ t('booking.types.endDate') }}</span>
              <input v-model="draft.endDate" type="date" class="input etform__mono" />
            </label>
          </template>
        </div>
      </fieldset>
      <div class="etform__grid2">
        <label class="field">
          <span class="field__label">{{ t('booking.types.availability') }} *</span>
          <select v-model.number="draft.availabilityId" class="input">
            <option v-for="s in sets" :key="s.id" :value="s.id">{{ s.name }}{{ s.is_default ? ` · ${t('booking.avail.default')}` : '' }}</option>
          </select>
        </label>
        <label class="field">
          <span class="field__label">{{ t('booking.types.calendar') }} *</span>
          <select v-model.number="draft.calendarId" class="input">
            <option v-for="c in calendars" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </label>
      </div>
      <label class="check">
        <input v-model="draft.active" type="checkbox" />
        <span>{{ t('booking.types.active') }}</span>
      </label>
      <ul v-if="errors.length" class="etform__errors">
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
.etform { display: flex; flex-direction: column; gap: var(--space-4); }
.etform__grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.etform__grid4 { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--space-3); }
.etform__mono { font-family: var(--font-mono); }
.etform__desc { resize: vertical; }
.etform__colors { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.etform__swatch {
  width: 26px; height: 26px; border-radius: var(--radius-full); cursor: pointer;
  border: 2px solid transparent; padding: 0;
}
.etform__swatch--active { border-color: var(--color-text-bright); box-shadow: 0 0 0 2px #fff inset; }
.etform__color-input { width: 108px; padding: 5px 8px; }
.etform__range { border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m); padding: var(--space-3) var(--space-4) var(--space-4); margin: 0; }
.etform__range-body { display: flex; gap: var(--space-4); align-items: end; flex-wrap: wrap; margin-top: var(--space-2); }
.etform__seg { display: inline-flex; border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s); overflow: hidden; }
.etform__seg-btn {
  border: none; background: var(--color-bg-panel); padding: 8px 14px; cursor: pointer;
  color: var(--color-text-secondary); font-size: 13px;
}
.etform__seg-btn--on { background: var(--color-accent); color: #fff; }
.field { display: flex; flex-direction: column; gap: var(--space-1); min-width: 0; }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
.check { display: flex; align-items: center; gap: var(--space-2); color: var(--color-text-secondary); }
.etform__errors { margin: 0; padding: var(--space-3) var(--space-4); background: var(--color-danger-soft); color: var(--color-danger); border-radius: var(--radius-s); list-style: none; }
.etform__errors li + li { margin-top: 4px; }
@media (max-width: 560px) {
  .etform__grid2, .etform__grid4 { grid-template-columns: 1fr 1fr; }
}
</style>
