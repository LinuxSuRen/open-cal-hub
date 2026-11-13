<script setup lang="ts">
// 公开页外壳：品牌头 + 居中内容 + 页脚。手写令牌风格，不依赖组件库。
import { t } from '../../i18n'
import ToastHost from '../../components/ds/ToastHost.vue'

withDefaults(defineProps<{ width?: string }>(), { width: '720px' })
</script>

<template>
  <div class="pub">
    <header class="pub__header">
      <router-link class="pub__brand" to="/">{{ t('app.name') }}</router-link>
    </header>
    <main class="pub__main">
      <div class="pub__content" :style="{ maxWidth: width }"><slot /></div>
    </main>
    <footer class="pub__footer">{{ t('pub.poweredBy') }}</footer>
    <ToastHost />
  </div>
</template>

<style scoped>
.pub { min-height: 100vh; display: flex; flex-direction: column; background: var(--color-bg-base); }
.pub__header {
  display: flex; align-items: center; padding: 0 var(--space-6); height: 56px;
  background: var(--color-bg-panel); border-bottom: 1px solid var(--color-border-subtle);
}
.pub__brand { font-weight: 700; font-size: 16px; color: var(--color-text-bright); letter-spacing: 0.02em; }
.pub__main { flex: 1; display: flex; justify-content: center; padding: var(--space-8) var(--space-4); }
.pub__content { width: 100%; }
.pub__footer {
  padding: var(--space-4); text-align: center; color: var(--color-text-dim); font-size: 12px;
}
</style>
