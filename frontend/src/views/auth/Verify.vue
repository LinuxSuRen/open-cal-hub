<script setup lang="ts">
// 邮箱验证：从 URL query 取 token 调接口，展示成功/失败态。
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { authApi } from '../../api/auth'
import { errorText } from '../../composables/useApiError'
import { t } from '../../i18n'
import AuthCard from './AuthCard.vue'

const route = useRoute()
const state = ref<'processing' | 'success' | 'failed'>('processing')
const errText = ref('')

onMounted(async () => {
  const token = typeof route.query.token === 'string' ? route.query.token : ''
  if (!token) {
    state.value = 'failed'
    errText.value = t('error.code.auth/invalid_token')
    return
  }
  try {
    await authApi.verifyEmail(token)
    state.value = 'success'
  } catch (err) {
    state.value = 'failed'
    errText.value = errorText(err)
  }
})
</script>

<template>
  <AuthCard>
    <h1 class="vfy__title">{{ t('auth.verify.title') }}</h1>

    <div v-if="state === 'processing'" class="vfy__state">
      <span class="vfy__spinner" />
      <p>{{ t('auth.verify.processing') }}</p>
    </div>

    <div v-else-if="state === 'success'" class="vfy__state">
      <span class="vfy__icon vfy__icon--ok">✓</span>
      <p class="vfy__ok">{{ t('auth.verify.success') }}</p>
      <router-link class="btn btn-primary" to="/login">{{ t('auth.verify.toLogin') }}</router-link>
    </div>

    <div v-else class="vfy__state">
      <span class="vfy__icon vfy__icon--err">✕</span>
      <p class="vfy__err">{{ t('auth.verify.failed') }}</p>
      <p v-if="errText" class="vfy__detail">{{ errText }}</p>
      <router-link class="btn" to="/login">{{ t('auth.verify.toLogin') }}</router-link>
    </div>
  </AuthCard>
</template>

<style scoped>
.vfy__title { margin: 0 0 var(--space-5); font-size: 18px; font-weight: 600; }
.vfy__state { display: flex; flex-direction: column; align-items: flex-start; gap: var(--space-4); }
.vfy__state p { margin: 0; font-size: 14px; }
.vfy__ok { color: var(--color-success); }
.vfy__err { color: var(--color-danger); }
.vfy__detail { color: var(--color-text-secondary); font-size: 13px; }
.vfy__icon { font-size: 18px; width: 32px; height: 32px; border-radius: var(--radius-full); display: inline-flex; align-items: center; justify-content: center; color: #fff; }
.vfy__icon--ok { background: var(--color-success); }
.vfy__icon--err { background: var(--color-danger); }
.vfy__spinner {
  width: 22px; height: 22px; border-radius: 50%;
  border: 2px solid var(--color-accent-soft); border-top-color: var(--color-accent);
  animation: vfy-spin 0.8s linear infinite;
}
@keyframes vfy-spin { to { transform: rotate(360deg); } }
</style>
