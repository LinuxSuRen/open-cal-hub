<script setup lang="ts">
// 手写分页器：上一页/页码窗口/下一页；产品 UI 场景使用（管理台用 ElPagination）。
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  page: number
  pageSize: number
  total: number
  windowSize?: number
}>(), { windowSize: 5 })

const emit = defineEmits<{ (e: 'update:page', v: number): void }>()

const pageCount = computed(() => Math.max(1, Math.ceil(props.total / Math.max(1, props.pageSize))))

const numbers = computed<number[]>(() => {
  const n = pageCount.value
  const w = Math.min(props.windowSize, n)
  let start = Math.max(1, Math.min(props.page - Math.floor(w / 2), n - w + 1))
  return Array.from({ length: w }, (_, i) => start + i)
})

function go(p: number): void {
  if (p < 1 || p > pageCount.value || p === props.page) return
  emit('update:page', p)
}
</script>

<template>
  <nav v-if="total > 0" class="pager" aria-label="分页">
    <button class="pager__btn" type="button" :disabled="page <= 1" @click="go(page - 1)">‹</button>
    <button
      v-for="n in numbers" :key="n" class="pager__btn" type="button"
      :class="{ 'pager__btn--active': n === page }" @click="go(n)"
    >{{ n }}</button>
    <button class="pager__btn" type="button" :disabled="page >= pageCount" @click="go(page + 1)">›</button>
    <span class="pager__info">{{ page }} / {{ pageCount }}</span>
  </nav>
</template>

<style scoped>
.pager { display: flex; align-items: center; gap: var(--space-2); justify-content: center; padding: var(--space-4) 0; }
.pager__btn {
  min-width: 30px; height: 30px; padding: 0 6px; border-radius: var(--radius-s);
  border: 1px solid var(--color-border-subtle); background: var(--color-bg-panel);
  color: var(--color-text-secondary); cursor: pointer; font-size: 13px;
}
.pager__btn:hover:not(:disabled) { border-color: var(--color-accent); color: var(--color-accent); }
.pager__btn:disabled { opacity: 0.4; cursor: default; }
.pager__btn--active { background: var(--color-accent); border-color: var(--color-accent); color: #fff; }
.pager__info { margin-left: var(--space-2); color: var(--color-text-dim); font-size: 13px; }
</style>
