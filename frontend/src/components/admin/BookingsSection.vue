<script setup lang="ts">
// 预约管理分区：全平台预约列表 + 筛选 + 分页。
import { onMounted, ref } from 'vue'
import { ElButton, ElInput, ElMessage, ElOption, ElPagination, ElSelect, ElTable, ElTableColumn, ElTag } from 'element-plus'
import { adminApi } from '../../api'
import type { AdminBooking } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { fmtLocal } from '../booking/slots'

const query = ref('')
const status = ref('')
const page = ref(1)
const pageSize = 20
const rows = ref<AdminBooking[]>([])
const total = ref(0)
const loading = ref(false)

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await adminApi.bookings({ query: query.value.trim() || undefined, status: status.value || undefined, page: page.value, page_size: pageSize })
    rows.value = res.data
    total.value = res.total
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function search(): void {
  page.value = 1
  void load()
}
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.bookings') }}</h2>
    <div class="bks__filters">
      <ElInput v-model="query" :placeholder="t('admin.bookings.search')" clearable style="width: 240px" @keyup.enter="search" />
      <ElButton type="primary" @click="search">{{ t('admin.users.searchBtn') }}</ElButton>
      <ElSelect v-model="status" style="width: 140px" @change="search">
        <ElOption :label="t('admin.users.statusAll')" value="" />
        <ElOption :label="t('admin.booking.confirmed')" value="confirmed" />
        <ElOption :label="t('admin.booking.cancelled')" value="cancelled" />
      </ElSelect>
    </div>

    <ElTable v-loading="loading" :data="rows" :empty-text="t('admin.bookings.empty')" stripe>
      <ElTableColumn prop="id" :label="t('admin.bookings.col.id')" width="64" />
      <ElTableColumn prop="event_type_name" :label="t('admin.bookings.col.type')" min-width="130" />
      <ElTableColumn prop="host_display_name" :label="t('admin.bookings.col.host')" min-width="110" />
      <ElTableColumn :label="t('admin.bookings.col.guest')" min-width="180">
        <template #default="{ row }">{{ row.guest_name }} · {{ row.guest_email }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.bookings.col.time')" width="300">
        <template #default="{ row }">{{ fmtLocal(row.start) }} – {{ fmtLocal(row.end) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.bookings.col.status')" width="90">
        <template #default="{ row }">
          <ElTag :type="row.status === 'cancelled' ? 'info' : 'success'" size="small">
            {{ row.status === 'cancelled' ? t('admin.booking.cancelled') : t('admin.booking.confirmed') }}
          </ElTag>
        </template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.bookings.col.created')" width="150">
        <template #default="{ row }">{{ fmtLocal(row.created_at) }}</template>
      </ElTableColumn>
    </ElTable>

    <div class="bks__pager">
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
.bks__filters { display: flex; gap: var(--space-3); margin-bottom: var(--space-4); flex-wrap: wrap; }
.bks__pager { display: flex; justify-content: flex-end; margin-top: var(--space-4); }
</style>
