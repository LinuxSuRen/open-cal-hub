<script setup lang="ts">
// 公开预约页 /u/{username}/{slug}：类型说明 → 时区+日期+槽位 → 确认表单（含蜜罐）→ 成功页。
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pubApi, type PublicEventType, type GuestBookingView } from '../../api/pub'
import { ApiError } from '../../api'
import { t, getLocale } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { addDays, formatDuration, fmtTime, humanDate, todayInTz, type SlotItem } from '../../components/booking/slots'
import { browserTimezone } from '../../components/booking/timezone'
import SlotPicker from '../../components/booking/SlotPicker.vue'
import ErrorPanel from '../../components/ds/ErrorPanel.vue'
import { showToast } from '../../components/ds/toast'
import PubShell from './PubShell.vue'

const route = useRoute()
const username = String(route.params.username ?? '')
const slug = String(route.params.slug ?? '')

const et = ref<PublicEventType | null>(null)
const loading = ref(true)
const errorCode = ref('')

const tz = ref(browserTimezone())
const slot = ref<SlotItem | null>(null)
const step = ref<'pick' | 'form' | 'done'>('pick')

const form = ref({ name: '', email: '', note: '', company: '' })
const formErrors = ref<{ name?: string; email?: string }>({})
const submitting = ref(false)
const booked = ref<{ booking: GuestBookingView; manage_url: string } | null>(null)

const picker = ref<InstanceType<typeof SlotPicker> | null>(null)

const minDate = computed(() => {
  if (!et.value) return undefined
  const today = todayInTz(tz.value)
  if (et.value.range_type === 'fixed' && et.value.start_date && et.value.start_date > today) return et.value.start_date
  return today
})
const maxDate = computed(() => {
  if (!et.value) return undefined
  if (et.value.range_type === 'fixed') return et.value.end_date ?? undefined
  return addDays(todayInTz(tz.value), et.value.range_days)
})

onMounted(async () => {
  try {
    et.value = await pubApi.eventType(username, slug)
  } catch (err) {
    errorCode.value = err instanceof ApiError ? err.code : 'network'
  } finally {
    loading.value = false
  }
})

function onSelect(s: SlotItem): void {
  slot.value = s
  step.value = 'form'
}

function onTzChange(z: string): void {
  tz.value = z
  slot.value = null
  if (step.value === 'form') step.value = 'pick'
}

function backToPick(): void {
  step.value = 'pick'
  slot.value = null
}

function validate(): boolean {
  const errs: { name?: string; email?: string } = {}
  if (!form.value.name.trim()) errs.name = t('pub.form.required')
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.value.email.trim())) errs.email = t('pub.form.invalidEmail')
  formErrors.value = errs
  return Object.keys(errs).length === 0
}

async function submit(): Promise<void> {
  if (!slot.value || !validate()) return
  submitting.value = true
  try {
    booked.value = await pubApi.book(username, slug, {
      start: slot.value.start,
      name: form.value.name.trim(),
      email: form.value.email.trim(),
      note: form.value.note.trim() || undefined,
      timezone: tz.value,
      company: form.value.company,
    })
    step.value = 'done'
  } catch (err) {
    if (err instanceof ApiError && err.code === 'booking/slot_unavailable') {
      showToast(t('pub.slotTaken'), 'error')
      backToPick()
      picker.value?.refresh()
    } else {
      showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
    }
  } finally {
    submitting.value = false
  }
}

function startOver(): void {
  step.value = 'pick'
  slot.value = null
  booked.value = null
  form.value = { name: '', email: '', note: '', company: '' }
  picker.value?.refresh()
}
</script>

<template>
  <PubShell width="680px">
    <div v-if="loading" class="bp__loading">{{ t('state.loading') }}</div>
    <ErrorPanel
      v-else-if="errorCode"
      :title="errorCode === 'booking/host_not_found' ? t('pub.hostNotFound.title')
        : errorCode === 'booking/event_type_not_found' || errorCode === 'booking/inactive' ? t('pub.typeNotFound.title')
        : apiErrorText({ code: errorCode, message: '' })"
      :desc="errorCode === 'booking/event_type_not_found' || errorCode === 'booking/inactive' ? t('pub.typeNotFound.desc')
        : errorCode === 'booking/host_not_found' ? t('pub.hostNotFound.desc') : ''"
    />

    <template v-else-if="et">
      <!-- 类型说明 -->
      <div class="bp__head" :style="{ '--et-color': et.color }">
        <div class="bp__head-row">
          <span class="bp__dot" />
          <h1 class="bp__title">{{ et.name }}</h1>
          <span class="bp__duration">{{ formatDuration(et.duration_minutes) }}</span>
        </div>
        <p v-if="et.description" class="bp__desc">{{ et.description }}</p>
      </div>

      <!-- 成功页 -->
      <div v-if="step === 'done' && booked" class="bp__done">
        <div class="bp__done-mark">✓</div>
        <h2 class="bp__done-title">{{ t('pub.booked.title') }}</h2>
        <p class="bp__done-desc">{{ t('pub.booked.desc') }}</p>
        <div class="bp__done-card">
          <div class="bp__done-line"><span class="bp__done-label">{{ et.name }}</span></div>
          <div class="bp__done-line bp__done-time">
            {{ humanDate(booked.booking.start, tz, getLocale()) }}
            · {{ fmtTime(booked.booking.start, tz) }}（{{ tz }}）
          </div>
        </div>
        <div class="bp__done-actions">
          <router-link class="btn btn-primary" :to="booked.manage_url">{{ t('pub.booked.manage') }}</router-link>
          <button class="btn" type="button" @click="startOver">{{ t('pub.booked.again') }}</button>
        </div>
      </div>

      <!-- 选择 + 表单 -->
      <template v-else>
        <div class="bp__panel">
          <h3 class="bp__panel-title">{{ step === 'pick' ? t('pub.step.date') : t('pub.step.slot') }}</h3>
          <SlotPicker
            v-show="step === 'pick'" ref="picker" :username="username" :slug="slug"
            :min-date="minDate" :max-date="maxDate" :selected-start="slot?.start ?? null"
            :initial-tz="tz" @select="onSelect" @tz-change="onTzChange"
          />
          <p v-if="step === 'pick'" class="bp__hint">{{ t('pub.step.form') }} →</p>
        </div>

        <div v-if="step === 'form' && slot" class="bp__panel">
          <h3 class="bp__panel-title">{{ t('pub.step.form') }}</h3>
          <div class="bp__slot-line">
            <span>{{ humanDate(slot.start, tz, getLocale()) }} · {{ fmtTime(slot.start, tz) }}–{{ fmtTime(slot.end, tz) }}</span>
            <button class="bp__slot-change" type="button" @click="backToPick">{{ t('pub.form.back') }}</button>
          </div>
          <form class="bp__form" novalidate @submit.prevent="submit">
            <label class="field">
              <span class="field__label">{{ t('pub.form.name') }} *</span>
              <input v-model="form.name" class="input" :placeholder="t('pub.form.namePh')" autocomplete="name" />
              <span v-if="formErrors.name" class="field__error">{{ formErrors.name }}</span>
            </label>
            <label class="field">
              <span class="field__label">{{ t('pub.form.email') }} *</span>
              <input v-model="form.email" class="input" type="email" :placeholder="t('pub.form.emailPh')" autocomplete="email" />
              <span v-if="formErrors.email" class="field__error">{{ formErrors.email }}</span>
            </label>
            <label class="field">
              <span class="field__label">{{ t('pub.form.note') }}</span>
              <textarea v-model="form.note" class="input" rows="3" :placeholder="t('pub.form.notePh')" />
            </label>
            <!-- 蜜罐：机器人填了才可见价值，人类不可见不可达 -->
            <div class="bp__honeypot" aria-hidden="true">
              <label>Company<input v-model="form.company" name="company" tabindex="-1" autocomplete="off" /></label>
            </div>
            <button class="btn btn-primary bp__submit" type="submit" :disabled="submitting">
              {{ submitting ? t('pub.form.submitting') : t('pub.form.submit') }}
            </button>
          </form>
        </div>
      </template>
    </template>
  </PubShell>
</template>

<style scoped>
.bp__loading { text-align: center; color: var(--color-text-dim); padding: var(--space-16) 0; }
.bp__head {
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l); padding: var(--space-6); box-shadow: var(--shadow-panel);
  border-top: 3px solid var(--et-color, var(--color-accent));
}
.bp__head-row { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
.bp__dot { width: 10px; height: 10px; border-radius: var(--radius-full); background: var(--et-color, var(--color-accent)); }
.bp__title { margin: 0; font-size: 20px; flex: 1; min-width: 0; }
.bp__duration { color: var(--color-text-secondary); font-size: 13px; }
.bp__desc { margin: var(--space-3) 0 0; color: var(--color-text-secondary); white-space: pre-wrap; }
.bp__panel {
  margin-top: var(--space-5); background: var(--color-bg-panel);
  border: 1px solid var(--color-border-subtle); border-radius: var(--radius-l); padding: var(--space-5) var(--space-6) var(--space-6);
}
.bp__panel-title { margin: 0 0 var(--space-4); font-size: 15px; }
.bp__hint { margin: var(--space-4) 0 0; color: var(--color-text-dim); font-size: 13px; }
.bp__slot-line {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-3);
  padding: var(--space-3) var(--space-4); background: var(--color-accent-soft);
  border-radius: var(--radius-m); margin-bottom: var(--space-4); flex-wrap: wrap;
}
.bp__slot-change { border: none; background: none; color: var(--color-accent); cursor: pointer; padding: 0; font-size: 13px; }
.bp__form { display: flex; flex-direction: column; gap: var(--space-4); }
.field { display: flex; flex-direction: column; gap: var(--space-1); }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
.field__error { color: var(--color-danger); font-size: 12px; }
.bp__submit { align-self: flex-start; padding: 10px 28px; }
.bp__honeypot { position: absolute; left: -9999px; top: -9999px; width: 1px; height: 1px; overflow: hidden; }
.bp__done { text-align: center; padding: var(--space-10) 0 var(--space-6); }
.bp__done-mark {
  width: 64px; height: 64px; margin: 0 auto; border-radius: var(--radius-full);
  background: var(--color-success-soft); color: var(--color-success);
  display: flex; align-items: center; justify-content: center; font-size: 28px;
}
.bp__done-title { margin: var(--space-4) 0 0; font-size: 20px; }
.bp__done-desc { margin: var(--space-2) 0 0; color: var(--color-text-secondary); }
.bp__done-card {
  margin: var(--space-6) auto 0; max-width: 420px; text-align: left;
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m); padding: var(--space-4) var(--space-5);
}
.bp__done-line { display: flex; justify-content: space-between; gap: var(--space-3); }
.bp__done-label { font-weight: 600; }
.bp__done-time { margin-top: var(--space-2); color: var(--color-text-secondary); }
.bp__done-actions { margin-top: var(--space-6); display: flex; gap: var(--space-3); justify-content: center; flex-wrap: wrap; }
</style>
