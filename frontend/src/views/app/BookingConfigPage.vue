<script setup lang="ts">
// 预约设置 /app/booking：预约类型 / 可用时间集 / 外部日历源 三个分区（手写令牌风格）。
import { computed, onMounted, ref } from 'vue'
import { bookingApi, type AvailabilitySet, type EventType, type ExternalFeed } from '../../api/booking'
import { calApi } from '../../api/calendars'
import type { Calendar } from '../../api/calendars'
import { ApiError } from '../../api'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { authState } from '../../stores/auth'
import { fmtLocal, formatDuration, minutesToHHmm } from '../../components/booking/slots'
import EventTypeDialog from '../../components/booking/EventTypeDialog.vue'
import AvailabilitySetDialog from '../../components/booking/AvailabilitySetDialog.vue'
import Modal from '../../components/ds/Modal.vue'
import ToastHost from '../../components/ds/ToastHost.vue'
import { showToast } from '../../components/ds/toast'

type Tab = 'types' | 'avail' | 'feeds'
const tab = ref<Tab>('types')

const types = ref<EventType[]>([])
const sets = ref<AvailabilitySet[]>([])
const feeds = ref<ExternalFeed[]>([])
const calendars = ref<Calendar[]>([])
const loading = ref(true)

// 目标日历：仅本人可写（owner/editor）的
const writableCals = computed(() => calendars.value.filter(c => c.role === 'owner' || c.role === 'editor'))
const setById = (id: number | null): AvailabilitySet | undefined => sets.value.find(s => s.id === id)
const calById = (id: number): Calendar | undefined => calendars.value.find(c => c.id === id)

async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const [ts, ss, cs, fs] = await Promise.all([
      bookingApi.eventTypes(), bookingApi.availabilitySets(), calApi.list(), bookingApi.feeds(),
    ])
    types.value = ts ?? []
    sets.value = ss ?? []
    calendars.value = cs ?? []
    feeds.value = fs ?? []
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    loading.value = false
  }
}
onMounted(loadAll)

// ---- 预约类型 ----
const typeDialogVisible = ref(false)
const editingType = ref<EventType | null>(null)
const deleteTypeTarget = ref<EventType | null>(null)
const deletingType = ref(false)

function openTypeNew(): void {
  editingType.value = null
  typeDialogVisible.value = true
}
function openTypeEdit(et: EventType): void {
  editingType.value = et
  typeDialogVisible.value = true
}
async function toggleType(et: EventType): Promise<void> {
  try {
    const updated = await bookingApi.updateEventType(et.id, { active: !et.active })
    Object.assign(et, updated)
    showToast(updated.active ? t('booking.types.toggleOn') : t('booking.types.toggleOff'), 'success')
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  }
}
async function copyBookingLink(et: EventType): Promise<void> {
  const url = `${location.origin}${et.booking_url}`
  try {
    await navigator.clipboard.writeText(url)
    showToast(t('booking.types.copied'), 'success')
  } catch {
    showToast(`${t('booking.types.copyManual')}: ${url}`, 'info')
  }
}
async function doDeleteType(): Promise<void> {
  if (!deleteTypeTarget.value) return
  deletingType.value = true
  try {
    await bookingApi.deleteEventType(deleteTypeTarget.value.id)
    showToast(t('common.deleted'), 'success')
    deleteTypeTarget.value = null
    await loadAll()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    deletingType.value = false
  }
}
function typeSummary(et: EventType): string {
  const setName = setById(et.availability_id)?.name ?? '—'
  const calName = calById(et.calendar_id)?.name ?? '—'
  return `${setName} · ${calName}`
}

// ---- 可用时间集 ----
const setDialogVisible = ref(false)
const editingSet = ref<AvailabilitySet | null>(null)
const deleteSetTarget = ref<AvailabilitySet | null>(null)
const deletingSet = ref(false)

function openSetNew(): void {
  editingSet.value = null
  setDialogVisible.value = true
}
async function doDeleteSet(): Promise<void> {
  if (!deleteSetTarget.value) return
  deletingSet.value = true
  try {
    await bookingApi.deleteAvailabilitySet(deleteSetTarget.value.id)
    showToast(t('common.deleted'), 'success')
    deleteSetTarget.value = null
    await loadAll()
  } catch (err) {
    // 被引用时后端返回 validation_failed
    showToast(err instanceof ApiError && err.code === 'common/validation_failed'
      ? t('booking.avail.inUse')
      : err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
    if (err instanceof ApiError && err.code === 'common/validation_failed') deleteSetTarget.value = null
  } finally {
    deletingSet.value = false
  }
}
function setRulesSummary(s: AvailabilitySet): string {
  if (s.rules.length === 0) return t('booking.rulesEmpty')
  const byWd = new Map<number, [number, number][]>()
  for (const r of s.rules) {
    const list = byWd.get(r.weekday) ?? []
    list.push([r.start_minute, r.end_minute])
    byWd.set(r.weekday, list)
  }
  return [...byWd.entries()]
    .sort((a, b) => ((a[0] + 6) % 7) - ((b[0] + 6) % 7))
    .map(([wd, spans]) => `${t(`booking.weekday.${wd}`)} ${spans.map(([a, b]) => `${minutesToHHmm(a)}-${minutesToHHmm(b)}`).join(' ')}`)
    .join('；')
}

// ---- 外部日历源 ----
const feedForm = ref({ name: '', url: '' })
const addingFeed = ref(false)
const syncingFeedId = ref(0)

async function addFeed(): Promise<void> {
  if (!feedForm.value.name.trim() || !/^https?:\/\//.test(feedForm.value.url.trim())) {
    showToast(t('booking.feeds.invalidUrl'), 'error')
    return
  }
  addingFeed.value = true
  try {
    await bookingApi.createFeed({ name: feedForm.value.name.trim(), url: feedForm.value.url.trim() })
    feedForm.value = { name: '', url: '' }
    feeds.value = (await bookingApi.feeds()) ?? []
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    addingFeed.value = false
  }
}
async function syncFeed(f: ExternalFeed): Promise<void> {
  syncingFeedId.value = f.id
  try {
    const res = await bookingApi.syncFeed(f.id)
    if (res?.last_error) showToast(`${t('booking.feeds.syncFailed')}：${res.last_error}`, 'error')
    else showToast(t('booking.feeds.syncOk').replace('{n}', String(res?.events ?? 0)), 'success')
    feeds.value = (await bookingApi.feeds()) ?? []
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    syncingFeedId.value = 0
  }
}
async function deleteFeed(f: ExternalFeed): Promise<void> {
  try {
    await bookingApi.deleteFeed(f.id)
    showToast(t('common.deleted'), 'success')
    feeds.value = (await bookingApi.feeds()) ?? []
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  }
}

const userTz = computed(() => authState.user?.timezone ?? 'UTC')
</script>

<template>
  <div class="page">
    <header class="bcfg__head">
      <div>
        <h1 class="bcfg__title">{{ t('booking.cfg.title') }}</h1>
        <p class="bcfg__desc">{{ t('booking.cfg.desc') }}</p>
      </div>
      <nav class="bcfg__tabs">
        <button
          v-for="tb in (['types', 'avail', 'feeds'] as Tab[])" :key="tb" type="button"
          class="bcfg__tab" :class="{ 'bcfg__tab--on': tab === tb }" @click="tab = tb"
        >{{ t(`booking.cfg.tab${tb === 'types' ? 'Types' : tb === 'avail' ? 'Avail' : 'Feeds'}`) }}</button>
      </nav>
    </header>

    <p v-if="loading">{{ t('state.loading') }}</p>

    <!-- 预约类型 -->
    <section v-else-if="tab === 'types'">
      <div class="bcfg__bar">
        <button class="btn btn-primary" type="button" @click="openTypeNew">{{ t('booking.types.new') }}</button>
      </div>
      <p v-if="types.length === 0" class="bcfg__empty">{{ t('booking.types.empty') }}</p>
      <div v-else class="bcfg__cards">
        <article v-for="et in types" :key="et.id" class="etcard" :style="{ '--et-color': et.color }">
          <div class="etcard__main">
            <div class="etcard__row">
              <span class="etcard__dot" />
              <span class="etcard__name">{{ et.name }}</span>
              <span class="etcard__duration">{{ formatDuration(et.duration_minutes) }}</span>
              <span v-if="!et.active" class="etcard__off">{{ t('booking.types.inactive') }}</span>
            </div>
            <div class="etcard__meta">
              <span class="etcard__slug">/u/{{ authState.user?.username }}/{{ et.slug }}</span>
              <span>{{ typeSummary(et) }}</span>
            </div>
          </div>
          <div class="etcard__actions">
            <label class="etcard__switch" :aria-label="t('booking.types.active')">
              <input type="checkbox" :checked="et.active" @change="toggleType(et)" />
              <span class="etcard__switch-track"><span class="etcard__switch-thumb" /></span>
            </label>
            <button class="btn etcard__btn" type="button" @click="copyBookingLink(et)">{{ t('booking.types.copyLink') }}</button>
            <button class="btn etcard__btn" type="button" @click="openTypeEdit(et)">{{ t('action.edit') }}</button>
            <button class="btn btn-danger etcard__btn" type="button" @click="deleteTypeTarget = et">{{ t('action.delete') }}</button>
          </div>
        </article>
      </div>
    </section>

    <!-- 可用时间集 -->
    <section v-else-if="tab === 'avail'">
      <div class="bcfg__bar">
        <button class="btn btn-primary" type="button" @click="openSetNew">{{ t('booking.avail.new') }}</button>
      </div>
      <p v-if="sets.length === 0" class="bcfg__empty">{{ t('booking.avail.empty') }}</p>
      <div v-else class="bcfg__cards">
        <article v-for="s in sets" :key="s.id" class="setcard">
          <div class="setcard__main">
            <div class="setcard__row">
              <span class="setcard__name">{{ s.name }}</span>
              <span v-if="s.is_default" class="setcard__default">{{ t('booking.avail.default') }}</span>
              <span class="setcard__tz">{{ s.timezone }}</span>
            </div>
            <p class="setcard__rules">{{ setRulesSummary(s) }}</p>
            <p v-if="s.dates.length > 0" class="setcard__dates">
              {{ t('booking.avail.dates') }}: {{ s.dates.length }}
            </p>
          </div>
          <div class="setcard__actions">
            <button class="btn" type="button" @click="editingSet = s; setDialogVisible = true">{{ t('action.edit') }}</button>
            <button class="btn btn-danger" type="button" @click="deleteSetTarget = s">{{ t('action.delete') }}</button>
          </div>
        </article>
      </div>
    </section>

    <!-- 外部日历源 -->
    <section v-else>
      <form class="feedform" @submit.prevent="addFeed">
        <input v-model="feedForm.name" class="input feedform__name" :placeholder="t('booking.feeds.namePh')" />
        <input v-model="feedForm.url" class="input feedform__url" :placeholder="t('booking.feeds.urlPh')" />
        <button class="btn btn-primary" type="submit" :disabled="addingFeed">{{ t('booking.feeds.add') }}</button>
      </form>
      <p v-if="feeds.length === 0" class="bcfg__empty">{{ t('booking.feeds.empty') }}</p>
      <div v-else class="bcfg__cards">
        <article v-for="f in feeds" :key="f.id" class="feedcard">
          <div class="feedcard__main">
            <div class="feedcard__row">
              <span class="feedcard__name">{{ f.name }}</span>
              <span class="feedcard__sync">{{ t('booking.feeds.lastSync') }}: {{ f.last_synced_at ? fmtLocal(f.last_synced_at) : t('booking.feeds.never') }}</span>
            </div>
            <p class="feedcard__url">{{ f.url }}</p>
            <p v-if="f.last_error" class="feedcard__error">{{ t('booking.feeds.lastError') }}: {{ f.last_error }}</p>
          </div>
          <div class="feedcard__actions">
            <button class="btn" type="button" :disabled="syncingFeedId === f.id" @click="syncFeed(f)">
              {{ syncingFeedId === f.id ? t('state.loading') : t('booking.feeds.sync') }}
            </button>
            <button class="btn btn-danger" type="button" @click="deleteFeed(f)">{{ t('action.delete') }}</button>
          </div>
        </article>
      </div>
    </section>

    <div class="bcfg__pager-note"><span>{{ t('settings.profile.timezone') }}: {{ userTz }}</span></div>

    <EventTypeDialog
      v-model:visible="typeDialogVisible" :editing="editingType"
      :calendars="writableCals" :sets="sets" @saved="loadAll"
    />
    <AvailabilitySetDialog v-model:visible="setDialogVisible" :editing="editingSet" @saved="loadAll" />

    <Modal :visible="!!deleteTypeTarget" :title="t('booking.types.deleteTitle')" width="420px" @update:visible="deleteTypeTarget = null">
      <p class="bcfg__confirm-desc">{{ t('booking.types.deleteDesc') }}</p>
      <template #footer>
        <button class="btn" type="button" @click="deleteTypeTarget = null">{{ t('action.cancel') }}</button>
        <button class="btn btn-danger" type="button" :disabled="deletingType" @click="doDeleteType">{{ t('action.delete') }}</button>
      </template>
    </Modal>
    <Modal :visible="!!deleteSetTarget" :title="t('booking.avail.deleteTitle')" width="420px" @update:visible="deleteSetTarget = null">
      <p class="bcfg__confirm-desc">{{ t('booking.avail.deleteDesc') }}</p>
      <template #footer>
        <button class="btn" type="button" @click="deleteSetTarget = null">{{ t('action.cancel') }}</button>
        <button class="btn btn-danger" type="button" :disabled="deletingSet" @click="doDeleteSet">{{ t('action.delete') }}</button>
      </template>
    </Modal>

    <ToastHost />
  </div>
</template>

<style scoped>
.bcfg__head { display: flex; align-items: flex-end; justify-content: space-between; gap: var(--space-4); flex-wrap: wrap; }
.bcfg__title { margin: 0; font-size: 20px; }
.bcfg__desc { margin: var(--space-1) 0 0; color: var(--color-text-secondary); }
.bcfg__tabs {
  display: inline-flex; border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-s); overflow: hidden; background: var(--color-bg-panel);
}
.bcfg__tab { border: none; background: none; padding: 8px 16px; cursor: pointer; color: var(--color-text-secondary); font-size: 13px; }
.bcfg__tab--on { background: var(--color-accent); color: #fff; }
.bcfg__bar { display: flex; justify-content: flex-end; margin: var(--space-5) 0 var(--space-4); }
.bcfg__empty {
  text-align: center; color: var(--color-text-dim); padding: var(--space-10);
  background: var(--color-bg-panel); border: 1px dashed var(--color-border-subtle); border-radius: var(--radius-l);
}
.bcfg__cards { display: flex; flex-direction: column; gap: var(--space-3); }
.bcfg__confirm-desc { margin: 0; }
.bcfg__pager-note { margin-top: var(--space-6); color: var(--color-text-dim); font-size: 12px; text-align: center; }

.etcard, .setcard, .feedcard {
  display: flex; justify-content: space-between; gap: var(--space-4); align-items: center;
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m); padding: var(--space-4) var(--space-5); flex-wrap: wrap;
}
.etcard { border-left: 4px solid var(--et-color, var(--color-accent)); }
.etcard__main, .setcard__main, .feedcard__main { min-width: 0; flex: 1; }
.etcard__row, .setcard__row, .feedcard__row { display: flex; align-items: center; gap: var(--space-3); flex-wrap: wrap; }
.etcard__dot { width: 10px; height: 10px; border-radius: var(--radius-full); background: var(--et-color, var(--color-accent)); }
.etcard__name { font-weight: 600; }
.etcard__duration { color: var(--color-text-secondary); font-size: 13px; }
.etcard__off { font-size: 12px; color: var(--color-text-dim); background: var(--color-bg-sunken); padding: 2px 8px; border-radius: var(--radius-full); }
.etcard__meta { display: flex; gap: var(--space-3); margin-top: var(--space-2); color: var(--color-text-dim); font-size: 12px; flex-wrap: wrap; }
.etcard__slug { font-family: var(--font-mono); }
.etcard__actions, .setcard__actions, .feedcard__actions { display: flex; gap: var(--space-2); align-items: center; flex-wrap: wrap; }
.etcard__btn { padding: 5px 12px; font-size: 13px; }

.etcard__switch { position: relative; display: inline-flex; cursor: pointer; }
.etcard__switch input { position: absolute; opacity: 0; inset: 0; margin: 0; cursor: pointer; }
.etcard__switch-track {
  width: 36px; height: 20px; border-radius: var(--radius-full);
  background: var(--color-bg-sunken); display: inline-flex; align-items: center; padding: 2px;
  transition: background 0.15s;
}
.etcard__switch-thumb { width: 16px; height: 16px; border-radius: var(--radius-full); background: #fff; transition: transform 0.15s; box-shadow: 0 1px 2px rgba(0,0,0,0.2); }
.etcard__switch input:checked + .etcard__switch-track { background: var(--color-accent); }
.etcard__switch input:checked + .etcard__switch-track .etcard__switch-thumb { transform: translateX(16px); }

.setcard__name { font-weight: 600; }
.setcard__default {
  font-size: 12px; color: var(--color-accent); background: var(--color-accent-soft);
  padding: 2px 8px; border-radius: var(--radius-full);
}
.setcard__tz { color: var(--color-text-dim); font-size: 12px; }
.setcard__rules { margin: var(--space-2) 0 0; color: var(--color-text-secondary); font-size: 13px; }
.setcard__dates { margin: var(--space-1) 0 0; color: var(--color-text-dim); font-size: 12px; }

.feedform { display: flex; gap: var(--space-3); margin: var(--space-5) 0 var(--space-4); flex-wrap: wrap; }
.feedform__name { width: 200px; }
.feedform__url { flex: 1; min-width: 240px; font-family: var(--font-mono); font-size: 13px; }
.feedcard__name { font-weight: 600; }
.feedcard__sync { color: var(--color-text-dim); font-size: 12px; }
.feedcard__url { margin: var(--space-2) 0 0; color: var(--color-text-secondary); font-size: 13px; word-break: break-all; font-family: var(--font-mono); }
.feedcard__error { margin: var(--space-2) 0 0; color: var(--color-danger); font-size: 12px; word-break: break-all; }
</style>
