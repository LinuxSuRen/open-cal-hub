<script setup lang="ts">
// 我的预约 /app/bookings：即将开始 / 已结束分组、取消（填原因）、来客信息、分页。
import { computed, onMounted, ref, watch } from 'vue'
import { bookingApi, type MyBooking } from '../../api/booking'
import { ApiError } from '../../api'
import { t, getLocale } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { authState } from '../../stores/auth'
import { fmtLocal, fmtTimeRange, humanDate } from '../../components/booking/slots'
import Modal from '../../components/ds/Modal.vue'
import Pager from '../../components/ds/Pager.vue'
import ToastHost from '../../components/ds/ToastHost.vue'
import { showToast } from '../../components/ds/toast'

type Tab = 'upcoming' | 'past'
const tab = ref<Tab>('upcoming')
const PAGE_SIZE = 10

const items = ref<MyBooking[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)

const cancelTarget = ref<MyBooking | null>(null)
const cancelReason = ref('')
const cancelling = ref(false)

const userTz = computed(() => authState.user?.timezone ?? 'UTC')

async function load(): Promise<void> {
  loading.value = true
  try {
    const nowIso = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z')
    const params = tab.value === 'upcoming'
      ? { from: nowIso, status: 'confirmed', page: page.value, page_size: PAGE_SIZE }
      : { to: nowIso, page: page.value, page_size: PAGE_SIZE }
    const res = await bookingApi.myBookings(params)
    items.value = [...res.data].sort((a, b) => tab.value === 'upcoming' ? a.start.localeCompare(b.start) : b.start.localeCompare(a.start))
    total.value = res.total
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(tab, () => { page.value = 1; void load() })

async function doCancel(): Promise<void> {
  if (!cancelTarget.value) return
  cancelling.value = true
  try {
    await bookingApi.cancelBooking(cancelTarget.value.id, cancelReason.value.trim() || undefined)
    showToast(t('mybook.cancelOk'), 'success')
    cancelTarget.value = null
    cancelReason.value = ''
    await load()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    cancelling.value = false
  }
}

function statusLabel(b: MyBooking): string {
  return b.status === 'cancelled' ? t('mybook.cancelled') : t('mybook.confirmed')
}
</script>

<template>
  <div class="page">
    <header class="mbk__head">
      <h1 class="mbk__title">{{ t('mybook.title') }}</h1>
      <nav class="mbk__tabs">
        <button type="button" class="mbk__tab" :class="{ 'mbk__tab--on': tab === 'upcoming' }" @click="tab = 'upcoming'">{{ t('mybook.upcoming') }}</button>
        <button type="button" class="mbk__tab" :class="{ 'mbk__tab--on': tab === 'past' }" @click="tab = 'past'">{{ t('mybook.past') }}</button>
      </nav>
    </header>

    <p v-if="loading" class="mbk__hint">{{ t('state.loading') }}</p>
    <p v-else-if="items.length === 0" class="mbk__empty">
      {{ tab === 'upcoming' ? t('mybook.emptyUpcoming') : t('mybook.emptyPast') }}
    </p>
    <div v-else class="mbk__list">
      <article
        v-for="b in items" :key="b.id" class="bkcard"
        :class="{ 'bkcard--cancelled': b.status === 'cancelled' }"
      >
        <div class="bkcard__time">
          <div class="bkcard__date">{{ humanDate(b.start, userTz, getLocale()) }}</div>
          <div class="bkcard__range">{{ fmtTimeRange(b.start, b.end, userTz) }}</div>
          <div class="bkcard__tz">{{ userTz }}</div>
        </div>
        <div class="bkcard__main">
          <div class="bkcard__row">
            <span class="bkcard__type">{{ b.event_type?.name ?? '—' }}</span>
            <span class="bkcard__status" :class="b.status === 'cancelled' ? 'bkcard__status--cancelled' : 'bkcard__status--ok'">{{ statusLabel(b) }}</span>
          </div>
          <dl class="bkcard__guest">
            <div class="bkcard__grow"><dt>{{ t('mybook.guest') }}</dt><dd>{{ b.guest_name }} · {{ b.guest_email }}</dd></div>
            <div v-if="b.guest_note" class="bkcard__grow"><dt>{{ t('mybook.note') }}</dt><dd class="bkcard__note">{{ b.guest_note }}</dd></div>
            <div class="bkcard__grow"><dt>{{ t('mybook.bookedAt') }}</dt><dd>{{ fmtLocal(b.created_at) }}</dd></div>
          </dl>
        </div>
        <div v-if="tab === 'upcoming' && b.status === 'confirmed'" class="bkcard__actions">
          <button class="btn btn-danger" type="button" @click="cancelTarget = b">{{ t('mybook.cancel') }}</button>
        </div>
      </article>
    </div>

    <Pager v-if="!loading" :page="page" :page-size="PAGE_SIZE" :total="total" @update:page="page = $event; load()" />

    <Modal :visible="!!cancelTarget" :title="t('mybook.cancelTitle')" width="440px" @update:visible="cancelTarget = null">
      <label class="field">
        <span class="field__label">{{ t('mybook.cancelReason') }}</span>
        <textarea v-model="cancelReason" class="input" rows="3" :placeholder="t('mybook.cancelReasonPh')" />
      </label>
      <template #footer>
        <button class="btn" type="button" @click="cancelTarget = null">{{ t('action.cancel') }}</button>
        <button class="btn btn-danger" type="button" :disabled="cancelling" @click="doCancel">
          {{ cancelling ? t('pub.form.submitting') : t('mybook.cancel') }}
        </button>
      </template>
    </Modal>

    <ToastHost />
  </div>
</template>

<style scoped>
.mbk__head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-4); flex-wrap: wrap; }
.mbk__title { margin: 0; font-size: 20px; }
.mbk__tabs { display: inline-flex; border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s); overflow: hidden; background: var(--color-bg-panel); }
.mbk__tab { border: none; background: none; padding: 8px 16px; cursor: pointer; color: var(--color-text-secondary); font-size: 13px; }
.mbk__tab--on { background: var(--color-accent); color: #fff; }
.mbk__hint, .mbk__empty { text-align: center; color: var(--color-text-dim); padding: var(--space-10) 0; margin: var(--space-4) 0 0; }
.mbk__empty {
  background: var(--color-bg-panel); border: 1px dashed var(--color-border-subtle);
  border-radius: var(--radius-l);
}
.mbk__list { display: flex; flex-direction: column; gap: var(--space-3); margin-top: var(--space-5); }
.bkcard {
  display: flex; gap: var(--space-5); align-items: flex-start;
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m); padding: var(--space-4) var(--space-5); flex-wrap: wrap;
}
.bkcard--cancelled { opacity: 0.65; }
.bkcard__time { min-width: 132px; }
.bkcard__date { font-weight: 600; }
.bkcard__range { font-family: var(--font-mono); color: var(--color-text-secondary); margin-top: 2px; }
.bkcard__tz { color: var(--color-text-dim); font-size: 12px; margin-top: 2px; }
.bkcard__main { flex: 1; min-width: 220px; }
.bkcard__row { display: flex; align-items: center; gap: var(--space-3); }
.bkcard__type { font-weight: 600; }
.bkcard__status { font-size: 12px; padding: 2px 9px; border-radius: var(--radius-full); }
.bkcard__status--ok { background: var(--color-success-soft); color: var(--color-success); }
.bkcard__status--cancelled { background: var(--color-bg-sunken); color: var(--color-text-dim); }
.bkcard__guest { margin: var(--space-3) 0 0; display: flex; flex-direction: column; gap: var(--space-2); }
.bkcard__grow { display: grid; grid-template-columns: 56px 1fr; gap: var(--space-3); }
.bkcard__grow dt { color: var(--color-text-dim); font-size: 12px; }
.bkcard__grow dd { margin: 0; font-size: 13px; word-break: break-all; }
.bkcard__note { white-space: pre-wrap; color: var(--color-text-secondary); }
.bkcard__actions { display: flex; align-items: center; }
.field { display: flex; flex-direction: column; gap: var(--space-1); }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
@media (max-width: 640px) {
  .bkcard { flex-direction: column; gap: var(--space-3); }
}
</style>
