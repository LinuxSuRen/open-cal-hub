<script setup lang="ts">
import { authState, logout, isAdmin } from './stores/auth'
import { t } from './i18n'
const navItems = [
  { to: '/app', label: t('nav.today') },
  { to: '/app/calendar', label: t('nav.calendar') },
  { to: '/app/calendars', label: t('nav.calendars') },
  { to: '/app/booking', label: t('nav.booking') },
  { to: '/app/bookings', label: t('nav.bookings') },
  { to: '/app/settings', label: t('nav.settings') },
]
</script>

<template>
  <div class="app-shell" v-if="authState.user">
    <header class="app-header">
      <span class="app-header__brand">{{ t('app.name') }}</span>
      <nav class="app-header__nav">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to">{{ item.label }}</router-link>
        <router-link v-if="isAdmin()" to="/admin">{{ t('nav.admin') }}</router-link>
      </nav>
      <div class="app-header__user">
        <span>{{ authState.user.display_name }}</span>
        <a href="#" @click.prevent="logout()">{{ t('nav.logout') }}</a>
      </div>
    </header>
    <main class="app-main"><router-view /></main>
  </div>
  <router-view v-else />
</template>

<style scoped>
.app-shell { min-height: 100vh; display: flex; flex-direction: column; }
.app-header {
  display: flex; align-items: center; gap: var(--space-6);
  padding: 0 var(--space-6); height: 52px;
  background: var(--color-bg-panel); border-bottom: 1px solid var(--color-border-subtle);
}
.app-header__brand { font-weight: 600; font-size: 16px; }
.app-header__nav { display: flex; gap: var(--space-4); flex: 1; }
.app-header__nav a { color: var(--color-text-secondary); }
.app-header__nav a.router-link-active { color: var(--color-accent); font-weight: 500; }
.app-header__user { display: flex; gap: var(--space-3); color: var(--color-text-secondary); }
.app-main { flex: 1; display: flex; flex-direction: column; }
</style>
