<script setup lang="ts">
import { onMounted } from 'vue'
import { authState, logout, isAdmin } from './stores/auth'
import { ensureMeta, metaState } from './stores/meta'
import { useToast } from './composables/useToast'
import { t } from './i18n'

const { toasts, dismiss } = useToast()

const navItems = [
  { to: '/app', label: t('nav.today') },
  { to: '/app/calendar', label: t('nav.calendar') },
  { to: '/app/calendars', label: t('nav.calendars') },
  { to: '/app/booking', label: t('nav.booking') },
  { to: '/app/bookings', label: t('nav.bookings') },
  { to: '/app/settings', label: t('nav.settings') },
]

onMounted(ensureMeta)
</script>

<template>
  <div class="app-shell" v-if="authState.user">
    <header class="app-header">
      <router-link class="app-header__brand" to="/app">{{ metaState.siteName }}</router-link>
      <nav class="app-header__nav">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to">{{ item.label }}</router-link>
        <router-link v-if="isAdmin()" to="/admin">{{ t('nav.admin') }}</router-link>
      </nav>
      <div class="app-header__user">
        <span class="app-header__name">{{ authState.user.display_name || authState.user.username }}</span>
        <button type="button" class="app-header__logout" @click="logout()">{{ t('nav.logout') }}</button>
      </div>
    </header>
    <main class="app-main"><router-view /></main>
  </div>
  <router-view v-else />

  <div class="app-toasts" aria-live="polite">
    <div v-for="item in toasts" :key="item.id" class="app-toast" :class="`app-toast--${item.kind}`" @click="dismiss(item.id)">
      {{ item.text }}
    </div>
  </div>
</template>

<style scoped>
.app-shell { min-height: 100vh; display: flex; flex-direction: column; }
.app-header {
  position: sticky; top: 0; z-index: 20;
  display: flex; align-items: center; gap: var(--space-6);
  padding: 0 var(--space-6); height: 52px;
  background: var(--color-bg-panel); border-bottom: 1px solid var(--color-border-subtle);
}
.app-header__brand { font-weight: 700; font-size: 16px; color: var(--color-text-bright); letter-spacing: 0.2px; }
.app-header__nav { display: flex; gap: var(--space-4); flex: 1; overflow-x: auto; }
.app-header__nav a { color: var(--color-text-secondary); font-size: 14px; white-space: nowrap; padding: 4px 2px; }
.app-header__nav a:hover { color: var(--color-text-primary); }
.app-header__nav a.router-link-active { color: var(--color-accent); font-weight: 500; }
.app-header__user { display: flex; align-items: center; gap: var(--space-3); color: var(--color-text-secondary); }
.app-header__name { font-size: 13px; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.app-header__logout {
  border: none; background: none; cursor: pointer;
  color: var(--color-text-secondary); font-size: 13px; padding: 4px 8px; border-radius: var(--radius-s);
}
.app-header__logout:hover { color: var(--color-danger); background: var(--color-danger-soft); }
.app-main { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.app-toasts {
  position: fixed; top: 64px; left: 50%; transform: translateX(-50%);
  z-index: 80; display: flex; flex-direction: column; gap: var(--space-2);
  pointer-events: none;
}
.app-toast {
  pointer-events: auto; cursor: pointer;
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-s); box-shadow: var(--shadow-pop);
  font-size: 13px; background: var(--color-text-bright); color: #fff;
  animation: app-toast-in 0.18s ease;
}
.app-toast--error { background: var(--color-danger); }
@keyframes app-toast-in { from { opacity: 0; transform: translateY(-6px); } to { opacity: 1; transform: none; } }
</style>
