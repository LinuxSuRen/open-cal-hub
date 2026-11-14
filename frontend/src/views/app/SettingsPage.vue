<script setup lang="ts">
// 设置 /app/settings：资料 / 改密码 / 会话管理 / 应用密码（一次性明文展示）。
import { computed, onMounted, ref, watch } from 'vue'
import { authApi, ApiError, type User } from '../../api'
import { t, setLocale } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { authState } from '../../stores/auth'
import { fmtLocal } from '../../components/booking/slots'
import { timezoneOptions, tzOffsetLabel } from '../../components/booking/timezone'
import Modal from '../../components/ds/Modal.vue'
import ToastHost from '../../components/ds/ToastHost.vue'
import { showToast } from '../../components/ds/toast'

type Tab = 'profile' | 'password' | 'sessions' | 'appPasswords'
const tab = ref<Tab>('profile')
const TABS: { key: Tab; i18n: string }[] = [
  { key: 'profile', i18n: 'settings.tab.profile' },
  { key: 'password', i18n: 'settings.tab.password' },
  { key: 'sessions', i18n: 'settings.tab.sessions' },
  { key: 'appPasswords', i18n: 'settings.tab.appPasswords' },
]

// ---- 资料 ----
const profile = ref({ display_name: '', timezone: '', locale: 'zh-CN', bio: '' })
const savingProfile = ref(false)
watch(() => authState.user, u => {
  if (u) profile.value = { display_name: u.display_name, timezone: u.timezone, locale: u.locale, bio: u.bio }
}, { immediate: true })

async function saveProfile(): Promise<void> {
  savingProfile.value = true
  try {
    const u = await authApi.updateMe({
      display_name: profile.value.display_name.trim(),
      timezone: profile.value.timezone,
      locale: profile.value.locale,
      bio: profile.value.bio,
    })
    authState.user = u as User
    setLocale(u.locale === 'en' ? 'en' : 'zh-CN')
    showToast(t('settings.saved'), 'success')
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    savingProfile.value = false
  }
}

// ---- 密码 ----
const pw = ref({ current: '', next: '', confirm: '' })
const pwError = ref('')
const changingPw = ref(false)

async function changePassword(): Promise<void> {
  pwError.value = ''
  if (!/^(?=.*[A-Za-z])(?=.*\d).{8,}$/.test(pw.value.next)) {
    pwError.value = t('settings.pw.weak')
    return
  }
  if (pw.value.next !== pw.value.confirm) {
    pwError.value = t('settings.pw.mismatch')
    return
  }
  changingPw.value = true
  try {
    await authApi.changePassword(pw.value.current, pw.value.next)
    pw.value = { current: '', next: '', confirm: '' }
    showToast(t('settings.pw.changed'), 'success')
    await loadSessions()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    changingPw.value = false
  }
}

// ---- 会话 ----
interface SessionRow { id: number; user_agent: string; ip: string; created_at: string; last_used_at: string | null; current: boolean }
const sessions = ref<SessionRow[]>([])
const sessionsLoading = ref(false)

async function loadSessions(): Promise<void> {
  sessionsLoading.value = true
  try {
    sessions.value = (await authApi.sessions()) ?? []
  } catch {
    sessions.value = []
  } finally {
    sessionsLoading.value = false
  }
}

async function revokeSession(id: number): Promise<void> {
  try {
    await authApi.revokeSession(id)
    showToast(t('settings.sess.revoked'), 'success')
    await loadSessions()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  }
}

const revokingOthers = ref(false)
async function revokeOthers(): Promise<void> {
  const others = sessions.value.filter(s => !s.current)
  if (others.length === 0) return
  revokingOthers.value = true
  try {
    for (const s of others) await authApi.revokeSession(s.id)
    showToast(t('settings.sess.allRevoked'), 'success')
    await loadSessions()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
    await loadSessions()
  } finally {
    revokingOthers.value = false
  }
}

// ---- 应用密码 ----
interface AppPasswordRow { id: number; label: string; created_at: string; last_used_at: string | null }
const appPasswords = ref<AppPasswordRow[]>([])
const apLabel = ref('')
const creatingAp = ref(false)
const freshAp = ref<{ label: string; password: string } | null>(null)
const deleteApTarget = ref<AppPasswordRow | null>(null)

async function loadAppPasswords(): Promise<void> {
  try {
    appPasswords.value = (await authApi.appPasswords()) ?? []
  } catch {
    appPasswords.value = []
  }
}

async function createAppPassword(): Promise<void> {
  if (!apLabel.value.trim()) return
  creatingAp.value = true
  try {
    const res = await authApi.createAppPassword(apLabel.value.trim())
    freshAp.value = { label: apLabel.value.trim(), password: res.password }
    apLabel.value = ''
    showToast(t('settings.ap.created'), 'success')
    await loadAppPasswords()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  } finally {
    creatingAp.value = false
  }
}

async function copyFresh(): Promise<void> {
  if (!freshAp.value) return
  try {
    await navigator.clipboard.writeText(freshAp.value.password)
    showToast(t('admin.invites.copied'), 'success')
  } catch {
    /* 手动选择复制 */
  }
}

async function deleteAppPassword(): Promise<void> {
  if (!deleteApTarget.value) return
  try {
    await authApi.deleteAppPassword(deleteApTarget.value.id)
    showToast(t('settings.ap.deleted'), 'success')
    deleteApTarget.value = null
    await loadAppPasswords()
  } catch (err) {
    showToast(err instanceof ApiError ? apiErrorText(err) : apiErrorText(null), 'error')
  }
}

onMounted(() => { void loadSessions(); void loadAppPasswords() })

const hasOtherSessions = computed(() => sessions.value.some(s => !s.current))
</script>

<template>
  <div class="page">
    <header class="st__head">
      <h1 class="st__title">{{ t('settings.title') }}</h1>
      <nav class="st__tabs">
        <button
          v-for="tb in TABS" :key="tb.key" type="button" class="st__tab"
          :class="{ 'st__tab--on': tab === tb.key }" @click="tab = tb.key"
        >{{ t(tb.i18n) }}</button>
      </nav>
    </header>

    <!-- 资料 -->
    <section v-if="tab === 'profile'" class="st__panel">
      <form class="st__form" @submit.prevent="saveProfile">
        <label class="field">
          <span class="field__label">{{ t('settings.profile.displayName') }}</span>
          <input v-model="profile.display_name" class="input st__input" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('settings.profile.timezone') }}</span>
          <select v-model="profile.timezone" class="input st__input">
            <option v-for="z in timezoneOptions(profile.timezone)" :key="z" :value="z">{{ z }}（{{ tzOffsetLabel(z) }}）</option>
          </select>
        </label>
        <label class="field">
          <span class="field__label">{{ t('settings.profile.locale') }}</span>
          <select v-model="profile.locale" class="input st__input">
            <option value="zh-CN">简体中文</option>
            <option value="en">English</option>
          </select>
        </label>
        <label class="field">
          <span class="field__label">{{ t('settings.profile.bio') }}</span>
          <textarea v-model="profile.bio" class="input" rows="4" :placeholder="t('settings.profile.bioPh')" />
        </label>
        <button class="btn btn-primary" type="submit" :disabled="savingProfile">{{ t('action.save') }}</button>
      </form>
    </section>

    <!-- 密码 -->
    <section v-else-if="tab === 'password'" class="st__panel">
      <form class="st__form" @submit.prevent="changePassword">
        <label class="field">
          <span class="field__label">{{ t('settings.pw.current') }}</span>
          <input v-model="pw.current" class="input st__input" type="password" autocomplete="current-password" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('settings.pw.new') }}</span>
          <input v-model="pw.next" class="input st__input" type="password" autocomplete="new-password" />
        </label>
        <label class="field">
          <span class="field__label">{{ t('settings.pw.confirm') }}</span>
          <input v-model="pw.confirm" class="input st__input" type="password" autocomplete="new-password" />
        </label>
        <p v-if="pwError" class="st__error">{{ pwError }}</p>
        <button class="btn btn-primary" type="submit" :disabled="changingPw || !pw.current || !pw.next">{{ t('settings.pw.change') }}</button>
      </form>
    </section>

    <!-- 会话 -->
    <section v-else-if="tab === 'sessions'" class="st__panel">
      <div class="st__bar">
        <button class="btn" type="button" :disabled="!hasOtherSessions || revokingOthers" @click="revokeOthers">
          {{ t('settings.sess.revokeOthers') }}
        </button>
      </div>
      <p v-if="sessionsLoading">{{ t('state.loading') }}</p>
      <p v-else-if="sessions.length === 0" class="st__empty">{{ t('state.empty') }}</p>
      <div v-else class="st__rows">
        <div v-for="s in sessions" :key="s.id" class="st__row" :class="{ 'st__row--current': s.current }">
          <div class="st__row-main">
            <span class="st__row-title">{{ s.user_agent || '—' }}</span>
            <span class="st__row-sub">{{ s.ip || '—' }} · {{ t('settings.sess.created') }} {{ fmtLocal(s.created_at) }} · {{ t('settings.sess.lastUsed') }} {{ s.last_used_at ? fmtLocal(s.last_used_at) : t('settings.ap.never') }}</span>
          </div>
          <span v-if="s.current" class="st__current-badge">{{ t('settings.sess.current') }}</span>
          <button v-else class="btn btn-danger st__revoke" type="button" @click="revokeSession(s.id)">{{ t('settings.sess.revoke') }}</button>
        </div>
      </div>
    </section>

    <!-- 应用密码 -->
    <section v-else class="st__panel">
      <p class="st__hint">{{ t('settings.ap.hint') }}</p>
      <form class="st__apform" @submit.prevent="createAppPassword">
        <input v-model="apLabel" class="input st__ap-input" :placeholder="t('settings.ap.labelPh')" />
        <button class="btn btn-primary" type="submit" :disabled="creatingAp || !apLabel.trim()">{{ t('settings.ap.create') }}</button>
      </form>
      <p v-if="appPasswords.length === 0" class="st__empty">{{ t('state.empty') }}</p>
      <div v-else class="st__rows">
        <div v-for="ap in appPasswords" :key="ap.id" class="st__row">
          <div class="st__row-main">
            <span class="st__row-title">{{ ap.label }}</span>
            <span class="st__row-sub">{{ t('settings.ap.lastUsed') }}: {{ ap.last_used_at ? fmtLocal(ap.last_used_at) : t('settings.ap.never') }} · {{ fmtLocal(ap.created_at) }}</span>
          </div>
          <button class="btn btn-danger" type="button" @click="deleteApTarget = ap">{{ t('action.delete') }}</button>
        </div>
      </div>
    </section>

    <!-- 新建应用密码：一次性明文 -->
    <Modal :visible="!!freshAp" :title="t('settings.ap.created')" width="440px" @update:visible="freshAp = null">
      <div v-if="freshAp" class="st__fresh">
        <p class="st__hint">{{ t('settings.ap.once') }}</p>
        <div class="st__fresh-pw" @click="copyFresh">{{ freshAp.password }}</div>
        <button class="btn st__fresh-copy" type="button" @click="copyFresh">{{ t('settings.ap.copy') }}</button>
      </div>
      <template #footer>
        <button class="btn btn-primary" type="button" @click="freshAp = null">{{ t('action.confirm') }}</button>
      </template>
    </Modal>

    <Modal :visible="!!deleteApTarget" :title="t('action.delete')" width="420px" @update:visible="deleteApTarget = null">
      <p>{{ t('settings.ap.deleteConfirm') }}</p>
      <template #footer>
        <button class="btn" type="button" @click="deleteApTarget = null">{{ t('action.cancel') }}</button>
        <button class="btn btn-danger" type="button" @click="deleteAppPassword">{{ t('action.delete') }}</button>
      </template>
    </Modal>

    <ToastHost />
  </div>
</template>

<style scoped>
.st__head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-4); flex-wrap: wrap; }
.st__title { margin: 0; font-size: 20px; }
.st__tabs { display: inline-flex; border: 1px solid var(--color-border-subtle); border-radius: var(--radius-s); overflow: hidden; background: var(--color-bg-panel); flex-wrap: wrap; }
.st__tab { border: none; background: none; padding: 8px 14px; cursor: pointer; color: var(--color-text-secondary); font-size: 13px; }
.st__tab--on { background: var(--color-accent); color: #fff; }
.st__panel {
  margin-top: var(--space-5); background: var(--color-bg-panel);
  border: 1px solid var(--color-border-subtle); border-radius: var(--radius-l); padding: var(--space-6);
  max-width: 560px;
}
.st__form { display: flex; flex-direction: column; gap: var(--space-4); }
.st__input { max-width: 360px; }
.st__error { margin: 0; color: var(--color-danger); font-size: 13px; }
.st__hint { margin: 0 0 var(--space-4); color: var(--color-text-secondary); }
.st__bar { display: flex; justify-content: flex-end; margin-bottom: var(--space-4); }
.st__empty { color: var(--color-text-dim); text-align: center; padding: var(--space-6) 0; margin: 0; }
.st__rows { display: flex; flex-direction: column; gap: var(--space-2); }
.st__row {
  display: flex; align-items: center; gap: var(--space-4); padding: var(--space-3) var(--space-4);
  border: 1px solid var(--color-border-subtle); border-radius: var(--radius-m); flex-wrap: wrap;
}
.st__row--current { border-color: var(--color-accent); background: var(--color-accent-soft); }
.st__row-main { flex: 1; min-width: 200px; display: flex; flex-direction: column; gap: 2px; }
.st__row-title { font-weight: 500; word-break: break-all; }
.st__row-sub { color: var(--color-text-dim); font-size: 12px; }
.st__current-badge { font-size: 12px; color: var(--color-accent); font-weight: 600; }
.st__revoke { padding: 4px 12px; font-size: 13px; }
.st__apform { display: flex; gap: var(--space-3); margin-bottom: var(--space-4); flex-wrap: wrap; }
.st__ap-input { width: 240px; }
@media (max-width: 560px) {
  .st__input, .st__ap-input { max-width: 100%; width: 100%; }
  .st__panel { padding: var(--space-4); }
}
.st__fresh { display: flex; flex-direction: column; gap: var(--space-3); align-items: stretch; }
.st__fresh-pw {
  font-family: var(--font-mono); font-size: 16px; padding: var(--space-3) var(--space-4);
  background: var(--color-bg-sunken); border-radius: var(--radius-s);
  word-break: break-all; cursor: pointer; user-select: all; text-align: center;
}
.st__fresh-copy { align-self: center; }
.field { display: flex; flex-direction: column; gap: var(--space-1); }
.field__label { font-size: 13px; color: var(--color-text-secondary); }
</style>
