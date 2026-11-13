<script setup lang="ts">
// 注册：时区下拉用 Intl 的 IANA 清单；邀请码字段按 meta.registration_mode 显隐。
import { computed, onMounted, ref } from 'vue'
import { authApi } from '../../api/auth'
import { ensureMeta, metaState } from '../../stores/meta'
import { errorText } from '../../composables/useApiError'
import { guessTimezone, timezoneOptions, type TzOption } from '../../composables/useTimezones'
import { getLocale, t } from '../../i18n'
import AuthCard from './AuthCard.vue'

const email = ref('')
const username = ref('')
const displayName = ref('')
const password = ref('')
const timezone = ref('UTC')
const inviteCode = ref('')
const submitting = ref(false)
const errText = ref('')
const done = ref<'verify' | 'login' | null>(null)

const inviteMode = computed(() => metaState.registrationMode === 'invite')
const tzOptions = ref<TzOption[]>([])

onMounted(async () => {
  await ensureMeta()
  timezone.value = guessTimezone()
  tzOptions.value = timezoneOptions(timezone.value)
})

function validEmail(v: string): boolean {
  return /.+@.+\..+/.test(v)
}
function validPassword(v: string): boolean {
  return v.length >= 8 && /[a-zA-Z]/.test(v) && /\d/.test(v)
}
function validUsername(v: string): boolean {
  return /^[a-z0-9][a-z0-9_-]{1,30}$/.test(v)
}

async function submit() {
  errText.value = ''
  if (!validEmail(email.value.trim())) {
    errText.value = t('validation.email')
    return
  }
  if (username.value.trim() && !validUsername(username.value.trim())) {
    errText.value = t('validation.username')
    return
  }
  if (!validPassword(password.value)) {
    errText.value = t('validation.password')
    return
  }
  if (inviteMode.value && !inviteCode.value.trim()) {
    errText.value = t('validation.inviteRequired')
    return
  }
  submitting.value = true
  try {
    const res = await authApi.register({
      email: email.value.trim(),
      password: password.value,
      username: username.value.trim() || undefined,
      display_name: displayName.value.trim() || undefined,
      timezone: timezone.value,
      locale: getLocale(),
      invite_code: inviteMode.value ? inviteCode.value.trim() : undefined,
    })
    done.value = res?.verification_required ? 'verify' : 'login'
  } catch (err) {
    errText.value = errorText(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard>
    <h1 class="reg__title">{{ t('auth.register.title') }}</h1>

    <div v-if="done" class="reg__done">
      <p class="reg__done-text">{{ t(done === 'verify' ? 'auth.register.successVerify' : 'auth.register.successNoVerify') }}</p>
      <router-link class="btn btn-primary reg__done-btn" to="/login">{{ t('auth.verify.toLogin') }}</router-link>
    </div>

    <form v-else class="form" @submit.prevent="submit">
      <label class="form__field">
        <span class="form__label">{{ t('auth.register.email') }}</span>
        <input v-model="email" class="input" type="email" autocomplete="email" />
      </label>
      <div class="reg__row">
        <label class="form__field reg__half">
          <span class="form__label">{{ t('auth.register.username') }}</span>
          <input v-model="username" class="input" type="text" :placeholder="t('auth.register.usernamePlaceholder')" autocomplete="username" />
        </label>
        <label class="form__field reg__half">
          <span class="form__label">{{ t('auth.register.displayName') }}</span>
          <input v-model="displayName" class="input" type="text" :placeholder="t('auth.register.displayNamePlaceholder')" />
        </label>
      </div>
      <label class="form__field">
        <span class="form__label">{{ t('auth.register.password') }}</span>
        <input v-model="password" class="input" type="password" autocomplete="new-password" />
        <span class="form__hint">{{ t('auth.register.passwordHint') }}</span>
      </label>
      <label class="form__field">
        <span class="form__label">{{ t('auth.register.timezone') }}</span>
        <select v-model="timezone" class="input">
          <option v-for="o in tzOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
        </select>
      </label>
      <label v-if="inviteMode" class="form__field">
        <span class="form__label">{{ t('auth.register.inviteCode') }}</span>
        <input v-model="inviteCode" class="input" type="text" />
      </label>

      <div v-if="errText" class="form__error" role="alert">{{ errText }}</div>

      <button class="btn btn-primary form__submit" type="submit" :disabled="submitting">
        {{ submitting ? t('state.loading') : t('auth.register.submit') }}
      </button>
    </form>

    <div v-if="!done" class="reg__links">
      <router-link to="/login">{{ t('auth.register.toLogin') }}</router-link>
    </div>
  </AuthCard>
</template>

<style scoped>
.reg__title { margin: 0 0 var(--space-5); font-size: 18px; font-weight: 600; }
.reg__row { display: flex; gap: var(--space-3); }
.reg__half { flex: 1; min-width: 0; }
.form { display: flex; flex-direction: column; gap: var(--space-4); }
.form__field { display: flex; flex-direction: column; gap: var(--space-1); }
.form__label { font-size: 13px; color: var(--color-text-secondary); }
.form__hint { font-size: 12px; color: var(--color-text-dim); }
.form__error { padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px; }
.form__submit { justify-content: center; margin-top: var(--space-1); }
.reg__done { display: flex; flex-direction: column; gap: var(--space-4); align-items: flex-start; }
.reg__done-text { margin: 0; font-size: 14px; color: var(--color-success); }
.reg__done-btn { text-decoration: none; }
.reg__links { margin-top: var(--space-5); font-size: 13px; }
</style>
