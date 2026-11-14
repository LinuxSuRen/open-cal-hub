<script setup lang="ts">
// 用户管理分区：搜索/状态与角色筛选/停用启用/角色调整/软删/发重置邮件/分页。
import { onMounted, ref } from 'vue'
import { ElButton, ElInput, ElMessage, ElMessageBox, ElOption, ElPagination, ElSelect, ElTable, ElTableColumn, ElTag } from 'element-plus'
import { adminApi } from '../../api'
import type { AdminUser } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { authState } from '../../stores/auth'
import { fmtLocal } from '../booking/slots'

const query = ref('')
const status = ref('')
const role = ref('')
const page = ref(1)
const pageSize = 20
const rows = ref<AdminUser[]>([])
const total = ref(0)
const loading = ref(false)

async function load(): Promise<void> {
  loading.value = true
  try {
    const res = await adminApi.users({ query: query.value.trim() || undefined, status: status.value || undefined, role: role.value || undefined, page: page.value, page_size: pageSize })
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

function errText(err: unknown): string {
  return apiErrorText(err as { code?: string; message?: string })
}

async function toggleStatus(u: AdminUser): Promise<void> {
  const disabling = u.status === 'active'
  try {
    await ElMessageBox.confirm(
      disabling ? t('admin.users.disableConfirm') : t('admin.users.enableConfirm'),
      disabling ? t('admin.users.disable') : t('admin.users.enable'),
      { type: disabling ? 'warning' : 'info' },
    )
  } catch { return }
  try {
    await adminApi.updateUser(u.id, { status: disabling ? 'disabled' : 'active' })
    ElMessage.success(t('admin.users.updated'))
    await load()
  } catch (err) { ElMessage.error(errText(err)) }
}

async function toggleRole(u: AdminUser): Promise<void> {
  const nextRole = u.role === 'admin' ? 'user' : 'admin'
  try {
    await ElMessageBox.confirm(
      t('admin.users.roleConfirm').replace('{r}', nextRole === 'admin' ? t('admin.role.admin') : t('admin.role.user')),
      t('admin.users.col.role'), { type: 'warning' },
    )
  } catch { return }
  try {
    await adminApi.updateUser(u.id, { role: nextRole })
    ElMessage.success(t('admin.users.updated'))
    await load()
  } catch (err) { ElMessage.error(errText(err)) }
}

async function sendReset(u: AdminUser): Promise<void> {
  try {
    await adminApi.sendReset(u.id)
    ElMessage.success(t('admin.users.resetSent'))
  } catch (err) { ElMessage.error(errText(err)) }
}

async function softDelete(u: AdminUser): Promise<void> {
  try {
    await ElMessageBox.confirm(t('admin.users.deleteConfirm'), t('admin.users.delete'), { type: 'error' })
  } catch { return }
  try {
    await adminApi.deleteUser(u.id)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (err) { ElMessage.error(errText(err)) }
}

function statusTag(u: AdminUser): { text: string; type: 'success' | 'info' | 'danger' } {
  if (u.status === 'active') return { text: t('admin.status.active'), type: 'success' }
  if (u.status === 'deleted') return { text: t('admin.status.deleted'), type: 'info' }
  return { text: t('admin.status.disabled'), type: 'danger' }
}
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.users') }}</h2>
    <div class="users__filters flt-bar">
      <ElInput v-model="query" :placeholder="t('admin.users.search')" clearable class="users__f-query" @keyup.enter="search" />
      <ElButton type="primary" @click="search">{{ t('admin.users.searchBtn') }}</ElButton>
      <ElSelect v-model="status" class="users__f-select" @change="search">
        <ElOption :label="t('admin.users.statusAll')" value="" />
        <ElOption :label="t('admin.status.active')" value="active" />
        <ElOption :label="t('admin.status.disabled')" value="disabled" />
        <ElOption :label="t('admin.status.deleted')" value="deleted" />
      </ElSelect>
      <ElSelect v-model="role" class="users__f-select" @change="search">
        <ElOption :label="t('admin.users.roleAll')" value="" />
        <ElOption :label="t('admin.role.user')" value="user" />
        <ElOption :label="t('admin.role.admin')" value="admin" />
      </ElSelect>
    </div>

    <ElTable v-loading="loading" :data="rows" :empty-text="t('state.empty')" stripe>
      <ElTableColumn prop="id" :label="t('admin.users.col.id')" width="64" />
      <ElTableColumn prop="username" :label="t('admin.users.col.username')" min-width="110" />
      <ElTableColumn prop="display_name" :label="t('admin.users.col.displayName')" min-width="110" />
      <ElTableColumn prop="email" :label="t('admin.users.col.email')" min-width="180" show-overflow-tooltip />
      <ElTableColumn :label="t('admin.users.col.role')" width="90">
        <template #default="{ row }">
          <ElTag :type="row.role === 'admin' ? 'warning' : 'info'" size="small">{{ row.role === 'admin' ? t('admin.role.admin') : t('admin.role.user') }}</ElTag>
        </template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.users.col.status')" width="90">
        <template #default="{ row }">
          <ElTag :type="statusTag(row as AdminUser).type" size="small">{{ statusTag(row as AdminUser).text }}</ElTag>
        </template>
      </ElTableColumn>
      <ElTableColumn prop="calendars_count" :label="t('admin.users.col.cals')" width="80" />
      <ElTableColumn :label="t('admin.users.col.created')" width="150">
        <template #default="{ row }">{{ fmtLocal(row.created_at) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.users.col.lastLogin')" width="150">
        <template #default="{ row }">{{ fmtLocal(row.last_login_at) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.users.col.actions')" width="300" fixed="right">
        <template #default="{ row }">
          <ElButton
            v-if="row.status !== 'deleted' && row.id !== authState.user?.id"
            size="small" :type="row.status === 'active' ? 'warning' : 'success'" @click="toggleStatus(row as AdminUser)"
          >{{ row.status === 'active' ? t('admin.users.disable') : t('admin.users.enable') }}</ElButton>
          <ElButton
            v-if="row.id !== authState.user?.id" size="small" @click="toggleRole(row as AdminUser)"
          >{{ row.role === 'admin' ? t('admin.users.makeUser') : t('admin.users.makeAdmin') }}</ElButton>
          <ElButton size="small" @click="sendReset(row as AdminUser)">{{ t('admin.users.sendReset') }}</ElButton>
          <ElButton
            v-if="row.id !== authState.user?.id" size="small" type="danger" @click="softDelete(row as AdminUser)"
          >{{ t('admin.users.delete') }}</ElButton>
        </template>
      </ElTableColumn>
    </ElTable>

    <div class="users__pager">
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
.users__filters { display: flex; gap: var(--space-3); margin-bottom: var(--space-4); flex-wrap: wrap; }
.users__f-query { width: 240px; }
.users__f-select { width: 140px; }
@media (max-width: 768px) {
  .users__f-query, .users__f-select { width: auto; flex: 1 1 140px; }
}
.users__pager { display: flex; justify-content: flex-end; margin-top: var(--space-4); }
</style>
