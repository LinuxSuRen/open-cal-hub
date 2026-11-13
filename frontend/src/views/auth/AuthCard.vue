<script setup lang="ts">
// 认证流程页共享外壳：居中卡片 + 站点名与标语。
import { ensureMeta, metaState } from '../../stores/meta'
import { t } from '../../i18n'

ensureMeta()
</script>

<template>
  <div class="auth">
    <div class="auth__card">
      <div class="auth__brand">
        <span class="auth__site">{{ metaState.siteName }}</span>
        <span class="auth__tag">{{ t('app.tagline') }}</span>
      </div>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.auth {
  min-height: 100vh;
  display: flex; align-items: center; justify-content: center;
  padding: var(--space-6) var(--space-4);
  background:
    radial-gradient(1200px 500px at 20% -10%, var(--color-accent-soft), transparent 60%),
    var(--color-bg-base);
}
.auth__card {
  width: 400px; max-width: 100%;
  background: var(--color-bg-panel);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l);
  box-shadow: var(--shadow-panel);
  padding: var(--space-8) var(--space-8) var(--space-6);
}
.auth__brand { display: flex; flex-direction: column; gap: var(--space-1); margin-bottom: var(--space-6); }
.auth__site { font-size: 22px; font-weight: 700; color: var(--color-text-bright); letter-spacing: 0.2px; }
.auth__tag { font-size: 13px; color: var(--color-text-secondary); }
</style>
