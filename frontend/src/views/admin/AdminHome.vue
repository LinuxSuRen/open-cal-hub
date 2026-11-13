<script setup lang="ts">
// 管理台 /admin：左侧分区导航 + 各分区组件。Element Plus 全量样式在本视图引入一次；
// 各分区组件按需 import 具体组件。分区由 /admin/:section 路由参数驱动。
import 'element-plus/dist/index.css'
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { t } from '../../i18n'
import DashboardSection from '../../components/admin/DashboardSection.vue'
import UsersSection from '../../components/admin/UsersSection.vue'
import BookingsSection from '../../components/admin/BookingsSection.vue'
import AuditSection from '../../components/admin/AuditSection.vue'
import SettingsSection from '../../components/admin/SettingsSection.vue'
import EmailsSection from '../../components/admin/EmailsSection.vue'
import InvitesSection from '../../components/admin/InvitesSection.vue'

const SECTIONS = {
  dashboard: DashboardSection,
  users: UsersSection,
  bookings: BookingsSection,
  audit: AuditSection,
  settings: SettingsSection,
  emails: EmailsSection,
  invites: InvitesSection,
} as const
type SectionKey = keyof typeof SECTIONS
const NAV: { key: SectionKey; i18n: string }[] = [
  { key: 'dashboard', i18n: 'admin.nav.dashboard' },
  { key: 'users', i18n: 'admin.nav.users' },
  { key: 'bookings', i18n: 'admin.nav.bookings' },
  { key: 'audit', i18n: 'admin.nav.audit' },
  { key: 'settings', i18n: 'admin.nav.settings' },
  { key: 'emails', i18n: 'admin.nav.emails' },
  { key: 'invites', i18n: 'admin.nav.invites' },
]

const route = useRoute()
const router = useRouter()

const active = computed<SectionKey>(() => {
  const raw = Array.isArray(route.params.section) ? route.params.section[0] : route.params.section
  return raw && raw in SECTIONS ? (raw as SectionKey) : 'dashboard'
})

function go(key: SectionKey): void {
  void router.push(`/admin/${key}`)
}
</script>

<template>
  <div class="admin">
    <aside class="admin__aside">
      <div class="admin__brand">{{ t('admin.title') }}</div>
      <nav class="admin__nav">
        <button
          v-for="item in NAV" :key="item.key" type="button" class="admin__nav-btn"
          :class="{ 'admin__nav-btn--on': active === item.key }" @click="go(item.key)"
        >{{ t(item.i18n) }}</button>
      </nav>
    </aside>
    <main class="admin__main">
      <component :is="SECTIONS[active]" />
    </main>
  </div>
</template>

<style scoped>
.admin { display: grid; grid-template-columns: 184px 1fr; min-height: 100vh; }
.admin__aside {
  background: var(--color-bg-panel); border-right: 1px solid var(--color-border-subtle);
  padding: var(--space-5) var(--space-3); display: flex; flex-direction: column; gap: var(--space-4);
}
.admin__brand { font-weight: 700; font-size: 15px; padding: 0 var(--space-2); }
.admin__nav { display: flex; flex-direction: column; gap: 2px; }
.admin__nav-btn {
  border: none; background: none; text-align: left; padding: 8px 12px; border-radius: var(--radius-s);
  cursor: pointer; color: var(--color-text-secondary); font-size: 13px;
}
.admin__nav-btn:hover { background: var(--color-bg-sunken); color: var(--color-text-primary); }
.admin__nav-btn--on { background: var(--color-accent-soft); color: var(--color-accent); font-weight: 600; }
.admin__main { padding: var(--space-6); min-width: 0; overflow-x: auto; }
@media (max-width: 720px) {
  .admin { grid-template-columns: 1fr; }
  .admin__aside { flex-direction: row; align-items: center; overflow-x: auto; border-right: none; border-bottom: 1px solid var(--color-border-subtle); }
  .admin__nav { flex-direction: row; }
  .admin__nav-btn { white-space: nowrap; }
}
</style>
