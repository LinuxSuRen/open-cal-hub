<script setup lang="ts">
// 邀请码分区：生成 / 列表 / 删除 / 复制。
import { onMounted, ref } from 'vue'
import { ElButton, ElInputNumber, ElMessage, ElMessageBox, ElTable, ElTableColumn, ElTag } from 'element-plus'
import { adminApi } from '../../api'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'
import { fmtLocal } from '../booking/slots'

interface InviteCode { code: string; created_at: string; expires_at: string | null; used_at: string | null }

const rows = ref<InviteCode[]>([])
const loading = ref(false)
const count = ref(1)
const expiresInHours = ref(168)
const generating = ref(false)
const generated = ref<string[]>([])

async function load(): Promise<void> {
  loading.value = true
  try {
    rows.value = (await adminApi.inviteCodes()) ?? []
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function generate(): Promise<void> {
  generating.value = true
  try {
    const res = (await adminApi.createInviteCodes(count.value, expiresInHours.value)) as unknown
    // 契约"→ 201 codes"两种可能形状都兼容
    generated.value = Array.isArray(res) ? (res as string[]) : ((res as { codes?: string[] })?.codes ?? [])
    ElMessage.success(t('admin.invites.generated').replace('{n}', String(generated.value.length)))
    await load()
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    generating.value = false
  }
}

async function copyCode(code: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(code)
    ElMessage.success(t('admin.invites.copied'))
  } catch {
    /* 手动复制 */
  }
}

async function remove(code: string): Promise<void> {
  try {
    await ElMessageBox.confirm(t('admin.invites.deleteConfirm'), t('admin.invites.delete'), { type: 'warning' })
  } catch { return }
  try {
    await adminApi.deleteInviteCode(code)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  }
}
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.invites') }}</h2>
    <div class="inv__gen">
      <div class="inv__field">
        <span class="inv__label">{{ t('admin.invites.count') }}</span>
        <ElInputNumber v-model="count" :min="1" :max="100" />
      </div>
      <div class="inv__field">
        <span class="inv__label">{{ t('admin.invites.expiresIn') }}</span>
        <ElInputNumber v-model="expiresInHours" :min="1" :max="24 * 365" />
      </div>
      <ElButton type="primary" :loading="generating" @click="generate">{{ t('admin.invites.generate') }}</ElButton>
    </div>

    <div v-if="generated.length" class="inv__fresh">
      <div v-for="c in generated" :key="c" class="inv__fresh-row">
        <code class="inv__code">{{ c }}</code>
        <ElButton size="small" @click="copyCode(c)">{{ t('admin.invites.copy') }}</ElButton>
      </div>
    </div>

    <ElTable v-loading="loading" :data="rows" :empty-text="t('admin.invites.empty')" stripe>
      <ElTableColumn :label="t('admin.invites.col.code')" min-width="200">
        <template #default="{ row }">
          <code class="inv__code">{{ row.code }}</code>
        </template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.invites.col.created')" width="160">
        <template #default="{ row }">{{ fmtLocal(row.created_at) }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.invites.col.expires')" width="160">
        <template #default="{ row }">{{ row.expires_at ? fmtLocal(row.expires_at) : '—' }}</template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.invites.col.used')" width="170">
        <template #default="{ row }">
          <ElTag v-if="!row.used_at" type="success" size="small">{{ t('admin.invites.unused') }}</ElTag>
          <span v-else>{{ fmtLocal(row.used_at) }}</span>
        </template>
      </ElTableColumn>
      <ElTableColumn :label="t('admin.users.col.actions')" width="180" fixed="right">
        <template #default="{ row }">
          <ElButton size="small" @click="copyCode(row.code)">{{ t('admin.invites.copy') }}</ElButton>
          <ElButton size="small" type="danger" @click="remove(row.code)">{{ t('admin.invites.delete') }}</ElButton>
        </template>
      </ElTableColumn>
    </ElTable>
  </div>
</template>

<style scoped>
.asec__title { margin: 0 0 var(--space-5); font-size: 17px; }
.inv__gen { display: flex; gap: var(--space-5); align-items: center; margin-bottom: var(--space-4); flex-wrap: wrap; }
.inv__field { display: flex; align-items: center; gap: var(--space-2); }
.inv__label { color: var(--color-text-secondary); font-size: 13px; }
.inv__fresh {
  margin-bottom: var(--space-4); padding: var(--space-4); border: 1px dashed var(--color-accent);
  border-radius: var(--radius-m); background: var(--color-accent-soft);
  display: flex; flex-direction: column; gap: var(--space-2); max-width: 480px;
}
.inv__fresh-row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
.inv__code { font-family: var(--font-mono); font-size: 13px; word-break: break-all; }
</style>
