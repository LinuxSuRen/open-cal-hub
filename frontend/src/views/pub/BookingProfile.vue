<script setup lang="ts">
// 公开预约主页 /u/{username}：主人名片 + 可预约类型卡片。
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { pubApi, type PublicHost } from '../../api/pub'
import { ApiError } from '../../api'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { formatDuration } from '../../components/booking/slots'
import ErrorPanel from '../../components/ds/ErrorPanel.vue'
import PubShell from './PubShell.vue'

const route = useRoute()
const username = String(route.params.username ?? '')

const host = ref<PublicHost | null>(null)
const loading = ref(true)
const errorCode = ref('')
const errorDesc = ref('')

onMounted(async () => {
  try {
    host.value = await pubApi.host(username)
  } catch (err) {
    if (err instanceof ApiError) {
      errorCode.value = err.code
      errorDesc.value = apiErrorText(err)
    } else {
      errorCode.value = 'network'
      errorDesc.value = apiErrorText(null)
    }
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <PubShell width="640px">
    <div v-if="loading" class="pro__loading">{{ t('state.loading') }}</div>
    <ErrorPanel
      v-else-if="errorCode"
      :title="errorCode === 'booking/host_not_found' ? t('pub.hostNotFound.title') : apiErrorText({ code: errorCode, message: errorDesc })"
      :desc="errorCode === 'booking/host_not_found' ? t('pub.hostNotFound.desc') : ''"
    />
    <template v-else-if="host">
      <div class="pro__card">
        <div class="pro__avatar">{{ host.display_name.slice(0, 1).toUpperCase() }}</div>
        <div class="pro__who">
          <h1 class="pro__name">{{ host.display_name }}</h1>
          <p v-if="host.bio" class="pro__bio">{{ host.bio }}</p>
          <p class="pro__tz">{{ t('pub.profile.hostTz') }} · {{ host.timezone }}</p>
        </div>
      </div>

      <h2 class="pro__section">{{ t('pub.profile.bookWith') }} {{ host.display_name }}</h2>
      <div v-if="host.event_types.length === 0" class="pro__empty">{{ t('pub.profile.noTypes') }}</div>
      <div v-else class="pro__types">
        <router-link
          v-for="et in host.event_types" :key="et.slug" class="pro__type"
          :to="`/u/${encodeURIComponent(username)}/${encodeURIComponent(et.slug)}`"
        >
          <span class="pro__type-color" :style="{ background: et.color }" />
          <span class="pro__type-name">{{ et.name }}</span>
          <span class="pro__type-meta">{{ formatDuration(et.duration_minutes) }}</span>
          <span class="pro__type-arrow">→</span>
        </router-link>
      </div>
    </template>
  </PubShell>
</template>

<style scoped>
.pro__loading { text-align: center; color: var(--color-text-dim); padding: var(--space-16) 0; }
.pro__card {
  display: flex; gap: var(--space-4); align-items: center; padding: var(--space-6);
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-l); box-shadow: var(--shadow-panel);
}
.pro__avatar {
  flex: none; width: 56px; height: 56px; border-radius: var(--radius-full);
  background: var(--color-accent-soft); color: var(--color-accent);
  display: flex; align-items: center; justify-content: center; font-size: 24px; font-weight: 700;
}
.pro__who { min-width: 0; }
.pro__name { margin: 0; font-size: 20px; }
.pro__bio { margin: var(--space-2) 0 0; color: var(--color-text-secondary); white-space: pre-wrap; word-break: break-word; }
.pro__tz { margin: var(--space-2) 0 0; color: var(--color-text-dim); font-size: 12px; }
.pro__section { margin: var(--space-8) 0 var(--space-4); font-size: 15px; }
.pro__empty {
  text-align: center; color: var(--color-text-dim); padding: var(--space-10);
  background: var(--color-bg-panel); border: 1px dashed var(--color-border-subtle); border-radius: var(--radius-l);
}
.pro__types { display: flex; flex-direction: column; gap: var(--space-3); }
.pro__type {
  display: flex; align-items: center; gap: var(--space-3); padding: var(--space-4) var(--space-5);
  background: var(--color-bg-panel); border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-m); color: var(--color-text-primary);
  transition: border-color 0.12s, box-shadow 0.12s;
}
.pro__type:hover { border-color: var(--color-accent); box-shadow: var(--shadow-panel); color: var(--color-text-primary); }
.pro__type-color { flex: none; width: 10px; height: 10px; border-radius: var(--radius-full); }
.pro__type-name { font-weight: 600; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pro__type-meta { color: var(--color-text-secondary); font-size: 13px; }
.pro__type-arrow { color: var(--color-text-dim); }
</style>
