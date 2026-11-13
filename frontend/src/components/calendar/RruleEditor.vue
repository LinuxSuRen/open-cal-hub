<script setup lang="ts">
// 重复规则编辑器：频率 / 间隔 / 每周按星期 / 按日期或次数结束。null = 不重复。
import { watch } from 'vue'
import dayjs from 'dayjs'
import { weekdayNumToByday, type RruleFreq, type RruleModel } from '../../composables/rrule'
import { t } from '../../i18n'
import { weekdayLabel } from './constants'

const props = defineProps<{ modelValue: RruleModel | null; startKey?: string; disabled?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: RruleModel | null): void }>()

const FREQS: RruleFreq[] = ['DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY']

function patch(p: Partial<RruleModel>) {
  if (!props.modelValue) return
  emit('update:modelValue', { ...props.modelValue, ...p })
}

function setFreq(freq: RruleFreq | 'NONE') {
  if (freq === 'NONE') {
    emit('update:modelValue', null)
    return
  }
  const m = props.modelValue ?? { freq, interval: 1, byday: [], endMode: 'never' as const, until: '', count: 10 }
  emit('update:modelValue', { ...m, freq })
}

// 切到每周且未选星期时，默认选中开始日期所在星期
watch(
  () => props.modelValue?.freq,
  (freq, old) => {
    if (freq === 'WEEKLY' && old !== 'WEEKLY' && props.modelValue && props.modelValue.byday.length === 0) {
      const base = props.startKey ? dayjs(props.startKey) : dayjs()
      patch({ byday: [weekdayNumToByday(base.day())] })
    }
  },
)

function toggleByday(code: string) {
  if (!props.modelValue) return
  const has = props.modelValue.byday.includes(code)
  patch({ byday: has ? props.modelValue.byday.filter(x => x !== code) : [...props.modelValue.byday, code] })
}
</script>

<template>
  <div class="rr">
    <select class="input" :disabled="disabled" :value="modelValue?.freq ?? 'NONE'" @change="setFreq(($event.target as HTMLSelectElement).value as RruleFreq | 'NONE')">
      <option value="NONE">{{ t('ev.repeat.none') }}</option>
      <option v-for="f in FREQS" :key="f" :value="f">{{ t('ev.repeat.' + f) }}</option>
    </select>

    <template v-if="modelValue">
      <div class="rr__row">
        <span class="rr__label">{{ t('ev.repeat.every') }}</span>
        <input
          class="input rr__num"
          type="number"
          min="1"
          max="99"
          :value="modelValue.interval"
          :disabled="disabled"
          @change="patch({ interval: Math.max(1, parseInt(($event.target as HTMLInputElement).value, 10) || 1) })"
        />
        <span class="rr__label">{{ t('ev.repeat.unit.' + modelValue.freq) }}</span>
      </div>

      <div v-if="modelValue.freq === 'WEEKLY'" class="rr__row">
        <span class="rr__label">{{ t('ev.repeat.byday') }}</span>
        <div class="rr__days">
          <button
            v-for="n in [1, 2, 3, 4, 5, 6, 0]"
            :key="n"
            type="button"
            class="rr__day"
            :class="{ 'rr__day--on': modelValue.byday.includes(weekdayNumToByday(n)) }"
            :disabled="disabled"
            @click="toggleByday(weekdayNumToByday(n))"
          >
            {{ weekdayLabel(n).slice(1) }}
          </button>
        </div>
      </div>

      <div class="rr__row">
        <span class="rr__label">{{ t('ev.repeat.end') }}</span>
        <label class="rr__radio"><input type="radio" value="never" :disabled="disabled" :checked="modelValue.endMode === 'never'" @change="patch({ endMode: 'never' })" />{{ t('ev.repeat.never') }}</label>
        <label class="rr__radio">
          <input type="radio" value="until" :disabled="disabled" :checked="modelValue.endMode === 'until'" @change="patch({ endMode: 'until' })" />
          {{ t('ev.repeat.until') }}
          <input
            v-if="modelValue.endMode === 'until'"
            class="input rr__date"
            type="date"
            :value="modelValue.until"
            :disabled="disabled"
            @change="patch({ until: ($event.target as HTMLInputElement).value })"
          />
        </label>
        <label class="rr__radio">
          <input type="radio" value="count" :disabled="disabled" :checked="modelValue.endMode === 'count'" @change="patch({ endMode: 'count' })" />
          {{ t('ev.repeat.count') }}
          <input
            v-if="modelValue.endMode === 'count'"
            class="input rr__num"
            type="number"
            min="1"
            max="999"
            :value="modelValue.count"
            :disabled="disabled"
            @change="patch({ count: Math.max(1, parseInt(($event.target as HTMLInputElement).value, 10) || 1) })"
          />
          {{ t('ev.repeat.countUnit') }}
        </label>
      </div>
    </template>
  </div>
</template>

<style scoped>
.rr { display: flex; flex-direction: column; gap: var(--space-2); }
.rr__row { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-2); }
.rr__label { font-size: 13px; color: var(--color-text-secondary); }
.rr__num { width: 72px; padding: 5px 8px; font-size: 13px; }
.rr__date { width: 150px; padding: 5px 8px; font-size: 13px; }
.rr__days { display: flex; gap: 4px; }
.rr__day {
  width: 30px; height: 26px; border-radius: var(--radius-s);
  border: 1px solid var(--color-border-subtle); background: var(--color-bg-panel);
  font-size: 12px; color: var(--color-text-secondary); cursor: pointer;
}
.rr__day--on { background: var(--color-accent); border-color: var(--color-accent); color: #fff; }
.rr__radio { display: inline-flex; align-items: center; gap: var(--space-1); font-size: 13px; }
.rr__radio input[type='radio'] { accent-color: var(--color-accent); }
</style>
