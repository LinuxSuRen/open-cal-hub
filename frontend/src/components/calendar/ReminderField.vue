<script setup lang="ts">
// 提醒编辑：预设 + 自定义分钟数，去重升序保存。
import { computed, ref } from 'vue'
import { t } from '../../i18n'
import { reminderText } from './constants'

const props = defineProps<{ modelValue: number[]; disabled?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: number[]): void }>()

const REMINDER_PRESETS = [0, 10, 30, 60, 1440]

const custom = ref('')
const presets = computed(() => REMINDER_PRESETS.filter(n => !props.modelValue.includes(n)))

function add(n: number) {
  if (props.disabled || n < 0 || props.modelValue.includes(n)) return
  emit('update:modelValue', [...props.modelValue, n].sort((a, b) => a - b))
}

function addCustom() {
  const n = parseInt(custom.value, 10)
  if (!Number.isFinite(n) || n < 0) {
    custom.value = ''
    return
  }
  add(n)
  custom.value = ''
}

function remove(n: number) {
  if (props.disabled) return
  emit('update:modelValue', props.modelValue.filter(x => x !== n))
}
</script>

<template>
  <div class="rem">
    <ul v-if="modelValue.length" class="rem__list">
      <li v-for="n in modelValue" :key="n" class="rem__item">
        <span>{{ reminderText(n) }}</span>
        <button type="button" class="rem__rm" :disabled="disabled" :aria-label="t('action.delete')" @click="remove(n)">✕</button>
      </li>
    </ul>
    <div class="rem__add">
      <select v-if="presets.length" class="input rem__select" :disabled="disabled" @change="add(parseInt(($event.target as HTMLSelectElement).value, 10)); ($event.target as HTMLSelectElement).selectedIndex = 0">
        <option value="">{{ t('ev.reminders.add') }}</option>
        <option v-for="n in presets" :key="n" :value="n">{{ reminderText(n) }}</option>
      </select>
      <input
        v-model="custom"
        type="number"
        class="input rem__custom"
        min="0"
        :placeholder="t('ev.reminders.placeholder')"
        :disabled="disabled"
        @keyup.enter="addCustom"
      />
      <button type="button" class="btn rem__btn" :disabled="disabled" @click="addCustom">{{ t('ev.reminders.add') }}</button>
    </div>
  </div>
</template>

<style scoped>
.rem__list { list-style: none; margin: 0 0 var(--space-2); padding: 0; display: flex; flex-wrap: wrap; gap: var(--space-2); }
.rem__item {
  display: inline-flex; align-items: center; gap: var(--space-1);
  padding: 2px 4px 2px 8px; font-size: 12px;
  background: var(--color-accent-soft); color: var(--color-accent-hover);
  border-radius: var(--radius-full);
}
.rem__rm {
  border: none; background: none; cursor: pointer; padding: 2px 4px;
  color: var(--color-accent-hover); font-size: 10px; border-radius: var(--radius-full);
}
.rem__rm:hover { background: rgba(79, 110, 247, 0.18); }
.rem__add { display: flex; gap: var(--space-2); }
.rem__select { flex: 1; padding: 6px 8px; font-size: 13px; }
.rem__custom { width: 110px; padding: 6px 8px; font-size: 13px; }
.rem__btn { padding: 6px 12px; font-size: 13px; }
</style>
