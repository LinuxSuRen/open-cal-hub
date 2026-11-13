<script setup lang="ts">
// 登录：邮箱或用户名 + 密码；错误码映射文案；邮箱未验证时提供重发验证邮件。
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { authApi } from '../../api/auth'
import { login } from '../../stores/auth'
import { errorText } from '../../composables/useApiError'
import { ApiError } from '../../api/client'
import { t } from '../../i18n'
import AuthCard from './AuthCard.vue'

const route = useRoute()
const router = useRouter()

const identifier = ref('')
const password = ref('')
const submitting = ref(false)
const errText = ref('')
const resendState = ref<'hidden' | 'sending' | 'sent'>('hidden')

const isEmail = computed(() => /.+@.+\..+/.test(identifier.value.trim()))
const needResend = computed(() => errText.value !== '' && lastCode.value === 'auth/email_not_verified')
const lastCode = ref('')

async function submit() {
  errText.value = ''
  resendState.value = 'hidden'
  lastCode.value = ''
  if (!identifier.value.trim() || !password.value) {
    errText.value = t('validation.required')
    return
  }
  submitting.value = true
  try {
    await login(identifier.value.trim(), password.value)
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/app'
    await router.replace(redirect)
  } catch (err) {
    if (err instanceof ApiError) lastCode.value = err.code
    errText.value = errorText(err)
  } finally {
    submitting.value = false
  }
}

async function resend() {
  if (!isEmail.value || resendState.value === 'sending') return
  resendState.value = 'sending'
  try {
    await authApi.resendVerification(identifier.value.trim())
    resendState.value = 'sent'
  } catch (err) {
    errText.value = errorText(err)
    resendState.value = 'hidden'
  }
}
</script>

<template>
  <AuthCard>
    <h1 class="login__title">{{ t('auth.login.title') }}</h1>
    <form class="form" @submit.prevent="submit">
      <label class="form__field">
        <span class="form__label">{{ t('auth.login.identifier') }}</span>
        <input v-model="identifier" class="input" type="text" autocomplete="username" autofocus />
      </label>
      <label class="form__field">
        <span class="form__label">{{ t('auth.login.password') }}</span>
        <input v-model="password" class="input" type="password" autocomplete="current-password" />
      </label>

      <div v-if="errText" class="form__error" role="alert">
        <span>{{ errText }}</span>
        <button v-if="needResend && resendState !== 'sent'" type="button" class="login__resend" @click="resend">
          {{ resendState === 'sending' ? t('state.loading') : t('auth.login.resend') }}
        </button>
      </div>
      <p v-if="resendState === 'sent'" class="form__ok">{{ t('auth.login.verificationSent') }}</p>

      <button class="btn btn-primary form__submit" type="submit" :disabled="submitting">
        {{ submitting ? t('state.loading') : t('auth.login.submit') }}
      </button>
    </form>
    <div class="login__links">
      <router-link to="/forgot">{{ t('auth.login.toForgot') }}</router-link>
      <router-link to="/register">{{ t('auth.login.toRegister') }}</router-link>
    </div>
  </AuthCard>
</template>

<style scoped>
.login__title { margin: 0 0 var(--space-5); font-size: 18px; font-weight: 600; }
.form { display: flex; flex-direction: column; gap: var(--space-4); }
.form__field { display: flex; flex-direction: column; gap: var(--space-1); }
.form__label { font-size: 13px; color: var(--color-text-secondary); }
.form__error {
  display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-s);
  background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px;
}
.form__ok { margin: 0; padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-success-soft); color: var(--color-success); font-size: 13px; }
.form__submit { justify-content: center; margin-top: var(--space-1); }
.login__resend { border: none; background: none; color: var(--color-danger); cursor: pointer; font-size: 13px; text-decoration: underline; padding: 0; }
.login__links { display: flex; justify-content: space-between; margin-top: var(--space-5); font-size: 13px; }
</style>
