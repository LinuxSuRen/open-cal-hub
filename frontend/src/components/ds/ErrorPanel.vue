<script setup lang="ts">
// 公开页错误面板：主人 404 / 类型 404 / 分享 404 / 分享过期 410 专用。
import { t } from '../../i18n'

defineProps<{ title: string; desc?: string }>()
</script>

<template>
  <div class="errpanel">
    <div class="errpanel__mark">40x</div>
    <h2 class="errpanel__title">{{ title }}</h2>
    <p v-if="desc" class="errpanel__desc">{{ desc }}</p>
    <router-link class="btn errpanel__home" to="/">{{ t('pub.backHome') }}</router-link>
  </div>
</template>

<style scoped>
.errpanel {
  display: flex; flex-direction: column; align-items: center; gap: var(--space-3);
  padding: var(--space-16) var(--space-6); text-align: center;
}
.errpanel__mark {
  font-family: var(--font-mono); font-size: 28px; font-weight: 700;
  color: var(--color-text-dim); background: var(--color-bg-sunken);
  border-radius: var(--radius-full); width: 72px; height: 72px;
  display: flex; align-items: center; justify-content: center;
}
.errpanel__title { margin: var(--space-2) 0 0; font-size: 18px; }
.errpanel__desc { margin: 0; color: var(--color-text-secondary); }
.errpanel__home { margin-top: var(--space-4); }
</style>
