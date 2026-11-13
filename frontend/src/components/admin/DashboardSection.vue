<script setup lang="ts">
// 仪表盘分区：统计卡片。
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { adminApi } from '../../api'
import type { AdminStats } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'

const stats = ref<AdminStats | null>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    stats.value = await adminApi.stats()
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
})

const CARDS: { key: keyof AdminStats; i18n: string }[] = [
  { key: 'users', i18n: 'admin.stats.users' },
  { key: 'active_users_30d', i18n: 'admin.stats.active30' },
  { key: 'calendars', i18n: 'admin.stats.calendars' },
  { key: 'events', i18n: 'admin.stats.events' },
  { key: 'bookings_30d', i18n: 'admin.stats.bookings30' },
  { key: 'pending_outbox', i18n: 'admin.stats.pendingOutbox' },
]
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.dashboard') }}</h2>
    <p v-if="loading">{{ t('state.loading') }}</p>
    <div v-else-if="stats" class="dash">
      <div v-for="c in CARDS" :key="c.key" class="dash__card">
        <div class="dash__num">{{ stats[c.key] }}</div>
        <div class="dash__label">{{ t(c.i18n) }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.asec__title { margin: 0 0 var(--space-5); font-size: 17px; }
.dash { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: var(--space-4); }
.dash__card {
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m); padding: var(--space-5);
}
.dash__num { font-size: 28px; font-weight: 700; font-family: var(--font-mono); }
.dash__label { margin-top: var(--space-2); color: var(--color-text-secondary); font-size: 13px; }
</style>
