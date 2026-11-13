<script setup lang="ts">
// 日历管理：列表分组、新建/编辑、分享链接、成员、ICS 导入（文本粘贴与文件上传）。
import { computed, onMounted, reactive, ref } from 'vue'
import dayjs from 'dayjs'
import { calApi, type Calendar, type CalendarMember, type ShareLink } from '../../api/calendars'
import { errorText } from '../../composables/useApiError'
import { copyText } from '../../composables/useClipboard'
import { guessTimezone, timezoneOptions, type TzOption } from '../../composables/useTimezones'
import { toast, toastError } from '../../composables/useToast'
import { ApiError } from '../../api/client'
import { t } from '../../i18n'
import { CALENDAR_COLORS, fillParam } from '../../components/calendar/constants'
import DialogShell from '../../components/calendar/DialogShell.vue'

const calendars = ref<Calendar[]>([])
const loading = ref(false)

const mine = computed(() => calendars.value.filter(c => c.role === 'owner'))
const shared = computed(() => calendars.value.filter(c => c.role !== 'owner'))

async function load() {
  loading.value = true
  try {
    const list = await calApi.list()
    calendars.value = Array.isArray(list) ? list : []
  } catch (err) {
    toastError(err)
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 新建 / 编辑 ----
const formVisible = ref(false)
const formEditing = ref<Calendar | null>(null)
const form = reactive({ name: '', color: CALENDAR_COLORS[0], description: '', timezone: 'UTC' })
const formSubmitting = ref(false)
const formErr = ref('')
const tzOptions = ref<TzOption[]>([])

function openForm(cal: Calendar | null) {
  formEditing.value = cal
  form.name = cal?.name ?? ''
  form.color = cal?.color ?? CALENDAR_COLORS[0]
  form.description = cal?.description ?? ''
  form.timezone = cal?.timezone ?? guessTimezone()
  tzOptions.value = timezoneOptions(form.timezone)
  formErr.value = ''
  formVisible.value = true
}

async function submitForm() {
  if (!form.name.trim()) {
    formErr.value = t('validation.nameRequired')
    return
  }
  formSubmitting.value = true
  formErr.value = ''
  try {
    const payload = { name: form.name.trim(), color: form.color, description: form.description.trim(), timezone: form.timezone }
    if (formEditing.value) {
      await calApi.update(formEditing.value.id, payload)
      toast(t('cals.saved'))
    } else {
      await calApi.create(payload)
      toast(t('cals.created'))
    }
    formVisible.value = false
    await load()
  } catch (err) {
    formErr.value = errorText(err)
  } finally {
    formSubmitting.value = false
  }
}

// ---- 分享链接 ----
const shareVisible = ref(false)
const shareCal = ref<Calendar | null>(null)
const shareLinks = ref<ShareLink[]>([])
const shareLoading = ref(false)
const shareLabel = ref('')
const shareExpiry = ref<'none' | '7' | '30'>('none')
const shareCreating = ref(false)

function shareIcsUrl(token: string): string {
  return `${location.origin}/api/v1/pub/share/${encodeURIComponent(token)}/ics`
}
function sharePageUrl(url: string): string {
  return url.startsWith('http') ? url : `${location.origin}${url}`
}

async function openShare(cal: Calendar) {
  shareCal.value = cal
  shareLabel.value = ''
  shareExpiry.value = 'none'
  shareVisible.value = true
  await reloadLinks(cal.id)
}

async function reloadLinks(calId: number) {
  shareLoading.value = true
  try {
    const list = await calApi.shareLinks(calId)
    shareLinks.value = Array.isArray(list) ? list : []
  } catch (err) {
    toastError(err)
  } finally {
    shareLoading.value = false
  }
}

async function createShare() {
  if (!shareCal.value || shareCreating.value) return
  shareCreating.value = true
  try {
    const expires =
      shareExpiry.value === 'none' ? undefined : dayjs().add(parseInt(shareExpiry.value, 10), 'day').utc().format()
    await calApi.createShareLink(shareCal.value.id, { label: shareLabel.value.trim() || undefined, expires_at: expires })
    toast(t('cals.share.created'))
    shareLabel.value = ''
    await reloadLinks(shareCal.value.id)
  } catch (err) {
    toastError(err)
  } finally {
    shareCreating.value = false
  }
}

async function revokeShare(id: number) {
  if (!shareCal.value) return
  try {
    await calApi.revokeShareLink(id)
    toast(t('cals.share.revoked'))
    await reloadLinks(shareCal.value.id)
  } catch (err) {
    toastError(err)
  }
}

async function copy(text: string) {
  const ok = await copyText(text)
  if (ok) toast(t('cals.share.copied'))
  else toast(t('error.fallback'), 'error')
}

// ---- 成员 ----
const memVisible = ref(false)
const memCal = ref<Calendar | null>(null)
const members = ref<CalendarMember[]>([])
const memLoading = ref(false)
const memAccount = ref('')
const memRole = ref<'viewer' | 'editor'>('viewer')
const memAdding = ref(false)

async function openMembers(cal: Calendar) {
  memCal.value = cal
  memAccount.value = ''
  memRole.value = 'viewer'
  memVisible.value = true
  await reloadMembers(cal.id)
}

async function reloadMembers(calId: number) {
  memLoading.value = true
  try {
    const list = await calApi.members(calId)
    members.value = Array.isArray(list) ? list : []
  } catch (err) {
    toastError(err)
  } finally {
    memLoading.value = false
  }
}

async function addMember() {
  if (!memCal.value || !memAccount.value.trim() || memAdding.value) return
  memAdding.value = true
  try {
    await calApi.addMember(memCal.value.id, { username_or_email: memAccount.value.trim(), role: memRole.value })
    toast(t('cals.members.added'))
    memAccount.value = ''
    await reloadMembers(memCal.value.id)
  } catch (err) {
    toastError(err)
  } finally {
    memAdding.value = false
  }
}

async function removeMember(userId: number) {
  if (!memCal.value) return
  try {
    await calApi.removeMember(memCal.value.id, userId)
    toast(t('cals.members.removed'))
    await reloadMembers(memCal.value.id)
  } catch (err) {
    toastError(err)
  }
}

// ---- ICS 导入 ----
const importVisible = ref(false)
const importCal = ref<Calendar | null>(null)
const importText = ref('')
const importFile = ref('')
const importing = ref(false)

function openImport(cal: Calendar) {
  importCal.value = cal
  importText.value = ''
  importFile.value = ''
  importVisible.value = true
}

function onImportFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    importText.value = String(reader.result ?? '')
  }
  reader.onerror = () => toast(t('error.fallback'), 'error')
  reader.readAsText(file)
}

async function submitImport() {
  if (!importCal.value) return
  const ics = importText.value.trim()
  if (!ics) {
    toast(t('cals.import.needContent'), 'error')
    return
  }
  importing.value = true
  try {
    const res = await calApi.importIcs(importCal.value.id, ics)
    toast(fillParam(fillParam(t('cals.import.result'), 'a', res?.imported ?? 0), 'b', res?.skipped ?? 0))
    importVisible.value = false
  } catch (err) {
    toastError(err)
  } finally {
    importing.value = false
  }
}

// ---- 删除 ----
const delVisible = ref(false)
const delCal = ref<Calendar | null>(null)
const delSubmitting = ref(false)

function openDelete(cal: Calendar) {
  delCal.value = cal
  delVisible.value = true
}

async function submitDelete() {
  if (!delCal.value || delSubmitting.value) return
  delSubmitting.value = true
  try {
    await calApi.remove(delCal.value.id)
    toast(t('cals.deleted'))
    delVisible.value = false
    await load()
  } catch (err) {
    if (err instanceof ApiError && err.code === 'common/validation_failed') {
      toast(t('cals.defaultUndeletable'), 'error')
    } else {
      toastError(err)
    }
  } finally {
    delSubmitting.value = false
  }
}
</script>

<template>
  <div class="page calsp">
    <header class="calsp__head">
      <h1 class="calsp__title">{{ t('cals.title') }}</h1>
      <button type="button" class="btn btn-primary" @click="openForm(null)">{{ t('cals.new') }}</button>
    </header>

    <section class="calsp__section">
      <h2 class="calsp__section-title">{{ t('cals.mine') }}</h2>
      <p v-if="loading && calendars.length === 0">{{ t('state.loading') }}</p>
      <ul class="calsp__list">
        <li v-for="c in mine" :key="c.id" class="calsp__item">
          <span class="calsp__dot" :style="{ background: c.color }" />
          <div class="calsp__info">
            <div class="calsp__name-line">
              <span class="calsp__name">{{ c.name }}</span>
              <span v-if="c.is_default" class="calsp__tag">{{ t('cals.default') }}</span>
              <span class="calsp__tag calsp__tag--role">{{ t('cals.role.' + c.role) }}</span>
            </div>
            <p v-if="c.description" class="calsp__desc">{{ c.description }}</p>
          </div>
          <div class="calsp__actions">
            <button type="button" class="btn calsp__action" @click="openForm(c)">{{ t('action.edit') }}</button>
            <button type="button" class="btn calsp__action" @click="openShare(c)">{{ t('cals.share') }}</button>
            <button type="button" class="btn calsp__action" @click="openMembers(c)">{{ t('cals.members') }}</button>
            <button type="button" class="btn calsp__action" @click="openImport(c)">{{ t('cals.import') }}</button>
            <button type="button" class="btn btn-danger calsp__action" @click="openDelete(c)">{{ t('cals.delete') }}</button>
          </div>
        </li>
      </ul>
      <p v-if="!loading && mine.length === 0" class="calsp__empty">{{ t('state.empty') }}</p>
    </section>

    <section class="calsp__section">
      <h2 class="calsp__section-title">{{ t('cals.shared') }}</h2>
      <ul class="calsp__list">
        <li v-for="c in shared" :key="c.id" class="calsp__item">
          <span class="calsp__dot" :style="{ background: c.color }" />
          <div class="calsp__info">
            <div class="calsp__name-line">
              <span class="calsp__name">{{ c.name }}</span>
              <span class="calsp__tag calsp__tag--role">{{ t('cals.role.' + c.role) }}</span>
            </div>
            <p class="calsp__desc">{{ fillParam(t('cals.ownerBy'), 'name', c.owner?.display_name || c.owner?.username || '') }}</p>
            <p v-if="c.description" class="calsp__desc">{{ c.description }}</p>
          </div>
          <div class="calsp__actions">
            <button v-if="c.role === 'editor'" type="button" class="btn calsp__action" @click="openImport(c)">{{ t('cals.import') }}</button>
          </div>
        </li>
      </ul>
      <p v-if="!loading && shared.length === 0" class="calsp__empty">{{ t('state.empty') }}</p>
    </section>

    <!-- 新建 / 编辑 -->
    <DialogShell v-model:visible="formVisible" :title="t(formEditing ? 'cals.editTitle' : 'cals.new')" width="480px">
      <div v-if="formErr" class="calsp__err" role="alert">{{ formErr }}</div>
      <div class="cform">
        <label class="cform__field">
          <span class="cform__label">{{ t('cals.name') }}</span>
          <input v-model="form.name" class="input" type="text" />
        </label>
        <div class="cform__field">
          <span class="cform__label">{{ t('cals.color') }}</span>
          <div class="cform__colors">
            <button
              v-for="c in CALENDAR_COLORS"
              :key="c"
              type="button"
              class="cform__color"
              :class="{ 'cform__color--on': form.color === c }"
              :style="{ background: c }"
              :aria-label="c"
              @click="form.color = c"
            />
            <label class="cform__color-custom">
              <input v-model="form.color" type="color" :aria-label="t('cals.color')" />
            </label>
          </div>
        </div>
        <label class="cform__field">
          <span class="cform__label">{{ t('cals.description') }}</span>
          <textarea v-model="form.description" class="input" rows="2" />
        </label>
        <label class="cform__field">
          <span class="cform__label">{{ t('cals.timezone') }}</span>
          <select v-model="form.timezone" class="input">
            <option v-for="o in tzOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>
      </div>
      <template #footer>
        <button type="button" class="btn" @click="formVisible = false">{{ t('action.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="formSubmitting" @click="submitForm">{{ t('action.save') }}</button>
      </template>
    </DialogShell>

    <!-- 分享 -->
    <DialogShell v-model:visible="shareVisible" :title="t('cals.share')" width="560px">
      <div class="cshare">
        <div class="cshare__new">
          <input v-model="shareLabel" class="input cshare__label" type="text" :placeholder="t('cals.share.labelPlaceholder')" :aria-label="t('cals.share.label')" />
          <select v-model="shareExpiry" class="input cshare__expiry" :aria-label="t('cals.share.expiry')">
            <option value="none">{{ t('cals.share.expiry.none') }}</option>
            <option value="7">{{ t('cals.share.expiry.7d') }}</option>
            <option value="30">{{ t('cals.share.expiry.30d') }}</option>
          </select>
          <button type="button" class="btn btn-primary" :disabled="shareCreating" @click="createShare">{{ t('cals.share.new') }}</button>
        </div>

        <p v-if="shareLoading">{{ t('state.loading') }}</p>
        <p v-else-if="shareLinks.length === 0" class="calsp__empty">{{ t('cals.share.empty') }}</p>
        <ul v-else class="cshare__list">
          <li v-for="l in shareLinks" :key="l.id" class="cshare__item">
            <div class="cshare__meta">
              <span class="cshare__link-label">{{ l.label || sharePageUrl(l.url) }}</span>
              <span class="cshare__date">{{ l.created_at.slice(0, 10) }}<template v-if="l.expires_at"> · {{ l.expires_at.slice(0, 10) }}</template></span>
            </div>
            <div class="cshare__ops">
              <button type="button" class="btn calsp__action" @click="copy(sharePageUrl(l.url))">{{ t('cals.share.copy') }}</button>
              <button type="button" class="btn calsp__action" @click="copy(shareIcsUrl(l.token))">{{ t('cals.share.copyIcs') }}</button>
              <button type="button" class="btn btn-danger calsp__action" @click="revokeShare(l.id)">{{ t('cals.share.revoke') }}</button>
            </div>
          </li>
        </ul>
        <p class="cshare__hint">{{ t('cals.share.icsHint') }}</p>
      </div>
    </DialogShell>

    <!-- 成员 -->
    <DialogShell v-model:visible="memVisible" :title="t('cals.members')" width="520px">
      <div class="cmem">
        <div class="cmem__add">
          <input v-model="memAccount" class="input cmem__account" type="text" :placeholder="t('cals.members.account')" />
          <select v-model="memRole" class="input cmem__role" :aria-label="t('cals.members.role')">
            <option value="viewer">{{ t('cals.role.viewer') }}</option>
            <option value="editor">{{ t('cals.role.editor') }}</option>
          </select>
          <button type="button" class="btn btn-primary" :disabled="memAdding || !memAccount.trim()" @click="addMember">{{ t('cals.members.add') }}</button>
        </div>
        <p v-if="memLoading">{{ t('state.loading') }}</p>
        <p v-else-if="members.length === 0" class="calsp__empty">{{ t('cals.members.empty') }}</p>
        <ul v-else class="cmem__list">
          <li v-for="m in members" :key="m.user_id" class="cmem__item">
            <span class="cmem__name">{{ m.display_name || m.username }}</span>
            <span class="calsp__tag calsp__tag--role">{{ t('cals.role.' + m.role) }}</span>
            <button type="button" class="btn btn-danger calsp__action" @click="removeMember(m.user_id)">{{ t('cals.members.remove') }}</button>
          </li>
        </ul>
      </div>
    </DialogShell>

    <!-- 导入 -->
    <DialogShell v-model:visible="importVisible" :title="t('cals.import')" width="560px">
      <div class="cimp">
        <label class="cform__field">
          <span class="cform__label">{{ t('cals.import.paste') }}</span>
          <textarea v-model="importText" class="input cimp__ta" rows="6" :placeholder="t('cals.import.pastePlaceholder')" />
        </label>
        <div class="cimp__file">
          <span class="cform__label">{{ t('cals.import.file') }}</span>
          <label class="btn cimp__file-btn">
            {{ importFile || t('cals.import.fileButton') }}
            <input type="file" accept=".ics,text/calendar,text/plain" class="cimp__file-input" @change="onImportFile" />
          </label>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn" @click="importVisible = false">{{ t('action.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="importing" @click="submitImport">{{ t('cals.import') }}</button>
      </template>
    </DialogShell>

    <!-- 删除确认 -->
    <DialogShell v-model:visible="delVisible" :title="t('cals.delete')" width="440px">
      <p class="cdel__text">{{ t('cals.deleteConfirm') }}</p>
      <p class="cdel__name">{{ delCal?.name }}</p>
      <template #footer>
        <button type="button" class="btn" @click="delVisible = false">{{ t('action.cancel') }}</button>
        <button type="button" class="btn btn-danger" :disabled="delSubmitting" @click="submitDelete">{{ t('action.delete') }}</button>
      </template>
    </DialogShell>
  </div>
</template>

<style scoped>
.calsp { display: flex; flex-direction: column; gap: var(--space-5); }
.calsp__head { display: flex; align-items: center; justify-content: space-between; }
.calsp__title { margin: 0; font-size: 20px; }
.calsp__section-title { margin: 0 0 var(--space-2); font-size: 14px; color: var(--color-text-secondary); }
.calsp__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.calsp__item {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m);
}
.calsp__dot { width: 12px; height: 12px; border-radius: var(--radius-full); flex-shrink: 0; }
.calsp__info { flex: 1; min-width: 0; }
.calsp__name-line { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.calsp__name { font-size: 14px; font-weight: 500; }
.calsp__tag {
  font-size: 11px; padding: 1px 8px; border-radius: var(--radius-full);
  background: var(--color-accent-soft); color: var(--color-accent-hover);
}
.calsp__tag--role { background: var(--color-bg-sunken); color: var(--color-text-secondary); }
.calsp__desc { margin: 2px 0 0; font-size: 12px; color: var(--color-text-dim); overflow: hidden; text-overflow: ellipsis; }
.calsp__actions { display: flex; gap: var(--space-2); flex-wrap: wrap; justify-content: flex-end; }
.calsp__action { padding: 5px 12px; font-size: 13px; }
.calsp__empty { color: var(--color-text-dim); font-size: 13px; }
.calsp__err { margin-bottom: var(--space-3); padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px; }
.cform { display: flex; flex-direction: column; gap: var(--space-3); }
.cform__field { display: flex; flex-direction: column; gap: var(--space-1); }
.cform__label { font-size: 12px; color: var(--color-text-secondary); }
.cform__colors { display: flex; gap: var(--space-2); align-items: center; }
.cform__color { width: 26px; height: 26px; border-radius: var(--radius-full); border: 2px solid transparent; cursor: pointer; }
.cform__color--on { border-color: var(--color-text-primary); }
.cform__color-custom input { width: 26px; height: 26px; border: none; background: none; padding: 0; cursor: pointer; }
.cshare { display: flex; flex-direction: column; gap: var(--space-3); }
.cshare__new { display: flex; gap: var(--space-2); }
.cshare__label { flex: 1; }
.cshare__expiry { width: 120px; }
.cshare__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.cshare__item {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s);
}
.cshare__meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.cshare__link-label { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cshare__date { font-size: 11px; color: var(--color-text-dim); font-family: var(--font-mono); }
.cshare__ops { display: flex; gap: var(--space-2); }
.cshare__hint { margin: 0; font-size: 12px; color: var(--color-text-dim); }
.cmem { display: flex; flex-direction: column; gap: var(--space-3); }
.cmem__add { display: flex; gap: var(--space-2); }
.cmem__account { flex: 1; }
.cmem__role { width: 110px; }
.cmem__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-2); }
.cmem__item {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s);
}
.cmem__name { flex: 1; font-size: 14px; }
.cimp { display: flex; flex-direction: column; gap: var(--space-3); }
.cimp__ta { font-family: var(--font-mono); font-size: 12px; }
.cimp__file { display: flex; align-items: center; gap: var(--space-3); }
.cimp__file-btn { position: relative; overflow: hidden; }
.cimp__file-input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
.cdel__text { margin: 0; font-size: 14px; }
.cdel__name { margin: var(--space-2) 0 0; font-size: 14px; font-weight: 600; }
</style>
