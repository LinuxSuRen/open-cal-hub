<script setup lang="ts">
// 忘记密码：无论邮箱是否存在都提示已发送（防枚举）。
import { ref } from 'vue'
import { authApi } from '../../api/auth'
import { errorText } from '../../composables/useApiError'
import { t } from '../../i18n'
import AuthCard from './AuthCard.vue'

const email = ref('')
const submitting = ref(false)
const errText = ref('')
const sent = ref(false)

async function submit() {
  errText.value = ''
  if (!/.+@.+\..+/.test(email.value.trim())) {
    errText.value = t('validation.email')
    return
  }
  submitting.value = true
  try {
    await authApi.forgotPassword(email.value.trim())
    sent.value = true
  } catch (err) {
    errText.value = errorText(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard>
    <h1 class="fg__title">{{ t('auth.forgot.title') }}</h1>

    <div v-if="sent" class="fg__done">
      <p class="fg__done-text">{{ t('auth.forgot.sent') }}</p>
      <router-link class="btn btn-primary" to="/login">{{ t('auth.forgot.back') }}</router-link>
    </div>

    <form v-else class="form" @submit.prevent="submit">
      <p class="fg__hint">{{ t('auth.forgot.hint') }}</p>
      <label class="form__field">
        <span class="form__label">{{ t('auth.forgot.email') }}</span>
        <input v-model="email" class="input" type="email" autocomplete="email" autofocus />
      </label>
      <div v-if="errText" class="form__error" role="alert">{{ errText }}</div>
      <button class="btn btn-primary form__submit" type="submit" :disabled="submitting">
        {{ submitting ? t('state.loading') : t('auth.forgot.submit') }}
      </button>
      <router-link class="fg__back" to="/login">{{ t('auth.forgot.back') }}</router-link>
    </form>
  </AuthCard>
</template>

<style scoped>
.fg__title { margin: 0 0 var(--space-5); font-size: 18px; font-weight: 600; }
.fg__hint { margin: 0 0 var(--space-2); font-size: 13px; color: var(--color-text-secondary); }
.form { display: flex; flex-direction: column; gap: var(--space-4); }
.form__field { display: flex; flex-direction: column; gap: var(--space-1); }
.form__label { font-size: 13px; color: var(--color-text-secondary); }
.form__error { padding: var(--space-2) var(--space-3); border-radius: var(--radius-s); background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px; }
.form__submit { justify-content: center; }
.fg__back { font-size: 13px; text-align: center; }
.fg__done { display: flex; flex-direction: column; gap: var(--space-4); align-items: flex-start; }
.fg__done-text { margin: 0; font-size: 14px; color: var(--color-success); }
</style>
