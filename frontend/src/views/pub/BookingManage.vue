<script setup lang="ts">
// 预约管理页 /b/{token}：详情卡 + 取消（原因）+ 改期（复用 SlotPicker；缺 host/slug 字段时回退手动时间）。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pubApi, type GuestBookingManage } from '../../api/pub'
import { ApiError } from '../../api'
import { t, getLocale } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { fmtTimeRange, humanDate, rfc3339FromLocal, type SlotItem } from '../../components/booking/slots'
import SlotPicker from '../../components/booking/SlotPicker.vue'
import ErrorPanel from '../../components/ds/ErrorPanel.vue'
import Modal from '../../components/ds/Modal.vue'
import { showToast } from '../../components/ds/toast'
import PubShell from './PubShell.vue'

const route = useRoute()
const token = String(route.params.token ?? '')

const detail = ref<GuestBookingManage | null>(null)
const loading = ref(true)
const errorCode = ref('')

const mode = ref<'view' | 'reschedule'>('view')
const picker = ref<InstanceType<typeof SlotPicker> | null>(null)
const reschedSlot = ref<SlotItem | null>(null)
const reschedSubmitting = ref(false)

const cancelVisible = ref(false)
const cancelReason = ref('')
const cancelSubmitting = ref(false)

// 回退路径：响应缺 host_username / slug 时手动指定（后端重校验）
const fallback = reactive({ date: '', time: '' })

const canReschedule = computed(() => detail.value?.status === 'confirmed')
const hasSlotApi = computed(() => !!(detail.value?.host_username && detail.value?.event_type.slug))
const bookingTz = computed(() => detail.value?.timezone || 'UTC')

async function load(): Promise<void> {
  loading.value = true
  try {
    detail.value = await pubApi.bookingByToken(token)
  } catch (err) {
    errorCode.value = err instanceof ApiError ? err.code : 'network'
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function doCancel(): Promise<void> {
  cancelSubmitting.value = true
  try {
    await pubApi.cancelByToken(token, cancelReason.value.trim() || undefined)
    showToast(t('mybook.cancelOk'), 'success')
    cancelVisible.value = false
    cancelReason.value = ''
    await load()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    cancelSubmitting.value = false
  }
}

async function doReschedule(): Promise<void> {
  const start = hasSlotApi.value
    ? reschedSlot.value?.start
    : fallback.date && fallback.time ? rfc3339FromLocal(fallback.date, fallback.time, bookingTz.value) : ''
  if (!start) return
  reschedSubmitting.value = true
  try {
    await pubApi.rescheduleByToken(token, start)
    showToast(t('pub.manage.reschedOk'), 'success')
    mode.value = 'view'
    reschedSlot.value = null
    fallback.date = ''
    fallback.time = ''
    await load()
  } catch (err) {
    if (err instanceof ApiError && err.code === 'booking/slot_unavailable') {
      showToast(t('pub.slotTaken'), 'error')
      reschedSlot.value = null
      picker.value?.refresh()
    } else {
      showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
    }
  } finally {
    reschedSubmitting.value = false
  }
}
</script>

<template>
  <PubShell width="640px">
    <div v-if="loading" class="bm__loading">{{ t('state.loading') }}</div>
    <ErrorPanel
      v-else-if="errorCode === 'booking/not_found'"
      :title="t('pub.typeNotFound.title')" :desc="t('pub.typeNotFound.desc')"
    />
    <ErrorPanel v-else-if="errorCode" :title="apiErrorText({ code: errorCode, message: '' })" />

    <template v-else-if="detail">
      <div class="bm__card">
        <div class="bm__head">
          <h1 class="bm__title">{{ detail.event_type.name }}</h1>
          <span class="bm__status" :class="detail.status === 'cancelled' ? 'bm__status--cancelled' : 'bm__status--ok'">
            {{ detail.status === 'cancelled' ? t('mybook.cancelled') : t('mybook.confirmed') }}
          </span>
        </div>
        <dl class="bm__meta">
          <div class="bm__row"><dt>{{ t('pub.manage.host') }}</dt><dd>{{ detail.host_display_name }}</dd></div>
          <div class="bm__row"><dt>{{ t('pub.manage.time') }}</dt>
            <dd>
              {{ humanDate(detail.start, bookingTz, getLocale()) }}
              {{ fmtTimeRange(detail.start, detail.end, bookingTz) }}（{{ bookingTz }}）
            </dd>
          </div>
          <div class="bm__row"><dt>{{ t('pub.manage.guest') }}</dt><dd>{{ detail.guest_name }}</dd></div>
          <div v-if="detail.guest_note" class="bm__row"><dt>{{ t('pub.manage.note') }}</dt><dd class="bm__note">{{ detail.guest_note }}</dd></div>
        </dl>

        <div v-if="detail.status === 'cancelled'" class="bm__cancelled-banner">{{ t('pub.manage.cancelled') }}</div>
        <div v-else class="bm__actions">
          <button class="btn" type="button" @click="mode = mode === 'reschedule' ? 'view' : 'reschedule'">
            {{ mode === 'reschedule' ? t('pub.form.back') : t('pub.manage.reschedule') }}
          </button>
          <button class="btn btn-danger" type="button" @click="cancelVisible = true">{{ t('pub.manage.cancel') }}</button>
        </div>
      </div>

      <!-- 改期 -->
      <div v-if="mode === 'reschedule' && canReschedule" class="bm__resched">
        <h3 class="bm__resched-title">{{ t('pub.manage.reschedPick') }}</h3>
        <template v-if="hasSlotApi">
          <SlotPicker
            ref="picker" :username="detail.host_username!" :slug="detail.event_type.slug!"
            :selected-start="reschedSlot?.start ?? null" :initial-tz="bookingTz"
            @select="s => (reschedSlot = s)"
          />
          <button
            class="btn btn-primary bm__resched-go" type="button"
            :disabled="!reschedSlot || reschedSubmitting" @click="doReschedule"
          >
            {{ reschedSubmitting ? t('pub.form.submitting') : t('pub.manage.reschedConfirm') }}
          </button>
        </template>
        <template v-else>
          <p class="bm__fallback-hint">{{ t('pub.manage.fallbackTitle') }}</p>
          <div class="bm__fallback">
            <label class="field">
              <span class="field__label">{{ t('pub.manage.fallbackDate') }}</span>
              <input v-model="fallback.date" type="date" class="input bm__fallback-input" />
            </label>
            <label class="field">
              <span class="field__label">{{ t('pub.manage.fallbackTime') }}</span>
              <input v-model="fallback.time" type="time" step="300" class="input bm__fallback-input" />
            </label>
          </div>
          <button
            class="btn btn-primary bm__resched-go" type="button"
            :disabled="!fallback.date || !fallback.time || reschedSubmitting" @click="doReschedule"
          >
            {{ reschedSubmitting ? t('pub.form.submitting') : t('pub.manage.reschedConfirm') }}
          </button>
        </template>
      </div>

      <!-- 取消对话框 -->
      <Modal v-model:visible="cancelVisible" :title="t('pub.manage.cancel')">
        <label class="field">
          <span class="field__label">{{ t('pub.manage.cancelReason') }}</span>
          <textarea v-model="cancelReason" class="input" rows="3" :placeholder="t('pub.manage.cancelReasonPh')" />
        </label>
        <template #footer>
          <button class="btn" type="button" @click="cancelVisible = false">{{ t('action.cancel') }}</button>
          <button class="btn btn-danger" type="button" :disabled="cancelSubmitting" @click="doCancel">
            {{ cancelSubmitting ? t('pub.form.submitting') : t('pub.manage.cancelConfirm') }}
          </button>
        </template>
      </Modal>
    </template>
  </PubShell>
</template>

<style scoped>
.bm__loading { text-align: center; color: var(--color-text-dim); padding: var(--space-16) 0; }
.bm__card {
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l); padding: var(--space-6); box-shadow: var(--shadow-panel);
}
.bm__head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
.bm__title { margin: 0; font-size: 20px; }
.bm__status { font-size: 12px; padding: 3px 10px; border-radius: var(--radius-full); }
.bm__status--ok { background: var(--color-success-soft); color: var(--color-success); }
.bm__status--cancelled { background: var(--color-bg-sunken); color: var(--color-text-dim); }
.bm__meta { margin: var(--space-5) 0 0; display: flex; flex-direction: column; gap: var(--space-3); }
.bm__row { display: grid; grid-template-columns: 64px 1fr; gap: var(--space-3); }
.bm__row dt { color: var(--color-text-dim); font-size: 13px; }
.bm__row dd { margin: 0; }
.bm__note { white-space: pre-wrap; color: var(--color-text-secondary); }
.bm__cancelled-banner {
  margin-top: var(--space-5); padding: var(--space-3) var(--space-4);
  background: var(--color-bg-sunken); color: var(--color-text-dim); border-radius: var(--radius-m); text-align: center;
}
.bm__actions { display: flex; gap: var(--space-3); margin-top: var(--space-6); flex-wrap: wrap; }
.bm__resched {
  margin-top: var(--space-5); background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l); padding: var(--space-5) var(--space-6) var(--space-6);
}
.bm__resched-title { margin: 0 0 var(--space-4); font-size: 15px; }
.bm__resched-go { margin-top: var(--space-5); }
.bm__fallback-hint { color: var(--color-text-secondary); margin: 0 0 var(--space-3); }
.bm__fallback { display: flex; gap: var(--space-4); flex-wrap: wrap; }
.bm__fallback-input { font-family: var(--font-mono); }
.field { display: flex; flex-direction: column; gap: var(--space-1); }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
</style>
