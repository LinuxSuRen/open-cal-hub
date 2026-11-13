<script setup lang="ts">
// 邮件出箱分区：列表 / 查看正文（e2e 依赖此处读取验证链接）/ 重发 / 分页。
import { onMounted, ref } from 'vue'
import { ElButton, ElDialog, ElMessage, ElPagination, ElTable, ElTableColumn, ElTag } from 'element-plus'
import { adminApi } from '../../api'
import type { OutboxMail } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { fmtLocal } from '../booking/slots'

const page = ref(1)
const pageSize = 20
const rows = ref<OutboxMail[]>([])
const total = ref(0)
const loading = ref(false)
const viewing = ref<OutboxMail | null>(null)

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await adminApi.emails(page.value, pageSize)
    rows.value = res.data
    total.value = res.total
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function retry(m: OutboxMail): Promise<void> {
  try {
    await adminApi.retryEmail(m.id)
    ElMessage.success(t('admin.emails.retried'))
    await load()
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  }
}
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.emails') }}</h2>
    <ElTable v-loading="loading" :data="rows" :empty-text="t('admin.emails.empty')" stripe>
      <ElTableColumn prop="id" :label="t('admin.emails.col.id')" width="70" />
      <ElTableColumn prop="kind" :label="t('admin.emails.col.kind')" min-width="130" />
      <ElTableColumn prop="to_addr" :label="t('admin.emails.col.to')" min-width="180" show-overflow-tooltip />
      <ElTableColumn prop="subject" :label="t('admin.emails.col.subject')" min-width="200" show-overflow-tooltip />
      <ElTableColumn :label="t('admin.emails.col.status')" width="100">
        <template #default="{ row }">
          <ElTag :type="row.status === 'sent' ? 'success' : row.status === 'failed' ? 'danger' : 'info'" size="small">{{ row.status }}</ElTag>
        </template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.emails.col.created')" width="150">
        <template #default="{ row }">{{ fmtLocal(row.created_at) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.users.col.actions')" width="200" fixed="right">
        <template #default="{ row }">
          <ElButton size="small" @click="viewing = row as OutboxMail">{{ t('admin.emails.view') }}</ElButton>
          <ElButton size="small" @click="retry(row as OutboxMail)">{{ t('admin.emails.retry') }}</ElButton>
        </template>
      </ElTableColumn>
    </ElTable>

    <div class="mails__pager">
      <ElPagination
        :current-page="page" :page-size="pageSize" :total="total"
        layout="total, prev, pager, next" background
        @current-change="(p: number) => { page = p; load() }"
      />
    </div>

    <ElDialog :model-value="!!viewing" :title="t('admin.emails.body')" width="640px" @update:model-value="(v: string | number | boolean) => { if (!v) viewing = null }">
      <div v-if="viewing" class="mails__body-wrap">
        <div class="mails__meta">{{ viewing.kind }} → {{ viewing.to_addr }} · {{ viewing.subject }}</div>
        <pre class="mails__body">{{ viewing.body_text }}</pre>
      </div>
    </ElDialog>
  </div>
</template>

<style scoped>
.asec__title { margin: 0 0 var(--space-5); font-size: 17px; }
.mails__pager { display: flex; justify-content: flex-end; margin-top: var(--space-4); }
.mails__meta { color: var(--color-text-secondary); font-size: 13px; margin-bottom: var(--space-3); word-break: break-all; }
.mails__body {
  margin: 0; padding: var(--space-4); background: var(--color-bg-sunken);
  border-radius: var(--radius-s); font-size: 12px; white-space: pre-wrap; word-break: break-all;
  max-height: 60vh; overflow-y: auto;
}
</style>
