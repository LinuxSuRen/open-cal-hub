<script setup lang="ts">
// 审计日志分区：分页列表 + 展开行详情 JSON。
import { onMounted, ref } from 'vue'
import { ElMessage, ElPagination, ElTable, ElTableColumn } from 'element-plus'
import { adminApi } from '../../api'
import type { AuditLog } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { fmtLocal } from '../booking/slots'

const page = ref(1)
const pageSize = 20
const rows = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await adminApi.auditLogs(page.value, pageSize)
    rows.value = res.data
    total.value = res.total
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.audit') }}</h2>
    <ElTable v-loading="loading" :data="rows" :empty-text="t('admin.audit.empty')" stripe>
      <ElTableColumn type="expand">
        <template #default="{ row }">
          <div class="audit__detail">
            <div class="audit__detail-title">{{ t('admin.audit.detail') }}</div>
            <pre class="audit__pre">{{ JSON.stringify(row.detail ?? {}, null, 2) }}</pre>
          </div>
        </template>
      </ElTableColumn>
      <ElTableColumn prop="id" :label="t('admin.audit.col.id')" width="70" />
      <ElTableColumn :label="t('admin.audit.col.time')" width="160">
        <template #default="{ row }">{{ fmtLocal(row.created_at) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.audit.col.actor')" width="90">
        <template #default="{ row }">{{ row.actor_id ?? t('admin.audit.system') }}</template>
      </ElTableColumn>
      <ElTableColumn prop="action" :label="t('admin.audit.col.action')" min-width="140" />
      <ElTableColumn :label="t('admin.audit.col.target')" min-width="130">
        <template #default="{ row }">{{ row.target_type }}#{{ row.target_id }}</template>
      </ElTableColumn>
      <ElTableColumn prop="ip" :label="t('admin.audit.col.ip')" width="130" />
    </ElTable>

    <div class="audit__pager">
      <ElPagination
        :current-page="page" :page-size="pageSize" :total="total"
        layout="total, prev, pager, next" background
        @current-change="(p: number) => { page = p; load() }"
      />
    </div>
  </div>
</template>

<style scoped>
.asec__title { margin: 0 0 var(--space-5); font-size: 17px; }
.audit__detail { padding: var(--space-2) var(--space-4); }
.audit__detail-title { color: var(--color-text-dim); font-size: 12px; margin-bottom: var(--space-2); }
.audit__pre {
  margin: 0; padding: var(--space-3); background: var(--color-bg-sunken);
  border-radius: var(--radius-s); font-size: 12px; max-width: 720px; overflow-x: auto;
}
.audit__pager { display: flex; justify-content: flex-end; margin-top: var(--space-4); }
</style>
