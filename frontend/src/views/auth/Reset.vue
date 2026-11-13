<script setup lang="ts">
// 重置密码：token 取自 URL query；成功后引导登录，链接失效可重新发起。
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { authApi } from '../../api/auth'
import { errorText } from '../../composables/useApiError'
import { t } from '../../i18n'
import AuthCard from './AuthCard.vue'

const route = useRoute()
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))

const password = ref('')
const confirm = ref('')
const submitting = ref(false)
const errText = ref('')
const done = ref(false)

async function submit() {
  errText.value = ''
  if (!(password.value.length >= 8 && /[a-zA-Z]/.test(password.value) && /\d/.test(password.value))) {
    errText.value = t('validation.password')
    return
  }
  if (password.value !== confirm.value) {
    errText.value = t('validation.passwordMismatch')
    return
  }
  submitting.value = true
  try {
    await authApi.resetPassword(token.value, password.value)
    done.value = true
  } catch (err) {
    errText.value = errorText(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard>
    <h1 class="rst__title">{{ t('auth.reset.title') }}</h1>

    <div v-if="!token" class="rst__done">
      <p class="rst__err">{{ t('error.code.auth/invalid_token') }}</p>
      <router-link class="btn btn-primary" to="/forgot">{{ t('auth.reset.toForgot') }}</router-link>
    </div>

    <div v-else-if="done" class="rst__done">
      <p class="rst__ok">{{ t('auth.reset.success') }}</p>
      <router-link class="btn btn-primary" to="/login">{{ t('auth.verify.toLogin') }}</router-link>
    </div>

    <form v-else class="form" @submit.prevent="submit">
      <label class="form__field">
        <span class="form__label">{{ t('auth.reset.password') }}</span>
        <input v-model="password" class="input" type="password" autocomplete="new-password" autofocus />
      </label>
      <label class="form__field">
        <span class="form__label">{{ t('auth.reset.confirm') }}</span>
        <input v-model="confirm" class="input" type="password" autocomplete="new-password" />
      </label>
      <div v-if="errText" class="form__error" role="alert">
        <span>{{ errText }}</span>
        <router-link v-if="errText === t('error.code.auth/token_expired') || errText === t('error.code.auth/invalid_token')" to="/forgot" class="rst__relink">
          {{ t('auth.reset.toForgot') }}
        </router-link>
      </div>
      <button class="btn btn-primary form__submit" type="submit" :disabled="submitting">
        {{ submitting ? t('state.loading') : t('auth.reset.submit') }}
      </button>
    </form>
  </AuthCard>
</template>

<style scoped>
.rst__title { margin: 0 0 var(--space-5); font-size: 18px; font-weight: 600; }
.form { display: flex; flex-direction: column; gap: var(--space-4); }
.form__field { display: flex; flex-direction: column; gap: var(--space-1); }
.form__label { font-size: 13px; color: var(--color-text-secondary); }
.form__error {
  display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-s);
  background: var(--color-danger-soft); color: var(--color-danger); font-size: 13px;
}
.rst__relink { color: var(--color-danger); text-decoration: underline; font-size: 13px; }
.form__submit { justify-content: center; }
.rst__done { display: flex; flex-direction: column; gap: var(--space-4); align-items: flex-start; }
.rst__ok { margin: 0; font-size: 14px; color: var(--color-success); }
.rst__err { margin: 0; font-size: 14px; color: var(--color-danger); }
</style>
