<script setup lang="ts">
// 系统设置分区：site_name / registration_mode / require_email_verification / booking_horizon_days。
import { onMounted, reactive, ref } from 'vue'
import { ElButton, ElForm, ElFormItem, ElInput, ElInputNumber, ElMessage, ElOption, ElSelect, ElSwitch } from 'element-plus'
import { adminApi } from '../../api'
import type { SiteSettings } from '../../api/admin'
import { t } from '../../i18n'
import { apiErrorText } from '../../i18n/pub'

const form = reactive<SiteSettings>({
  site_name: '',
  registration_mode: 'open',
  require_email_verification: true,
  booking_horizon_days: 365,
})
const loading = ref(true)
const saving = ref(false)

onMounted(async () => {
  try {
    const s = await adminApi.settings()
    Object.assign(form, s)
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    loading.value = false
  }
})

async function save(): Promise<void> {
  saving.value = true
  try {
    const s = await adminApi.updateSettings({ ...form })
    Object.assign(form, s)
    ElMessage.success(t('admin.settings.saved'))
  } catch (err) {
    ElMessage.error(apiErrorText(err as { code?: string; message?: string }))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <h2 class="asec__title">{{ t('admin.nav.settings') }}</h2>
    <p v-if="loading">{{ t('state.loading') }}</p>
    <ElForm v-else class="sysset__form" label-width="180px" @submit.prevent="save">
      <ElFormItem :label="t('admin.settings.siteName')">
        <ElInput v-model="form.site_name" style="width: 320px" />
      </ElFormItem>
      <ElFormItem :label="t('admin.settings.regMode')">
        <ElSelect v-model="form.registration_mode" style="width: 320px">
          <ElOption :label="t('admin.settings.regOpen')" value="open" />
          <ElOption :label="t('admin.settings.regInvite')" value="invite" />
        </ElSelect>
      </ElFormItem>
      <ElFormItem :label="t('admin.settings.requireVerify')">
        <ElSwitch v-model="form.require_email_verification" />
      </ElFormItem>
      <ElFormItem :label="t('admin.settings.horizon')">
        <ElInputNumber v-model="form.booking_horizon_days" :min="1" :max="3650" />
        <span class="sysset__hint">{{ t('admin.settings.horizonHint') }}</span>
      </ElFormItem>
      <ElFormItem>
        <ElButton type="primary" :loading="saving" @click="save">{{ t('action.save') }}</ElButton>
      </ElFormItem>
    </ElForm>
  </div>
</template>

<style scoped>
.asec__title { margin: 0 0 var(--space-5); font-size: 17px; }
.sysset__form { max-width: 640px; }
.sysset__hint { margin-left: var(--space-3); color: var(--color-text-dim); font-size: 12px; }
</style>
