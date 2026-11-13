<script setup lang="ts">
// 手写模态对话框：v-model:visible + 标题 + 默认/底部插槽；ESC 与遮罩点击关闭，监听自清理。
import { watch, onUnmounted } from 'vue'

const props = withDefaults(defineProps<{
  visible: boolean
  title: string
  width?: string
  closeOnBackdrop?: boolean
}>(), { width: '560px', closeOnBackdrop: true })

const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

function close(): void {
  emit('update:visible', false)
}

function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape' && props.visible) close()
}

watch(() => props.visible, v => {
  if (v) document.addEventListener('keydown', onKey)
  else document.removeEventListener('keydown', onKey)
}, { immediate: true })

onUnmounted(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <teleport to="body">
    <div v-if="visible" class="modal" @keydown.esc="close">
      <div class="modal__backdrop" @click="closeOnBackdrop && close()" />
      <div class="modal__panel" role="dialog" aria-modal="true" :aria-label="title" :style="{ maxWidth: width }">
        <header class="modal__header">
          <h3 class="modal__title">{{ title }}</h3>
          <button class="modal__close" type="button" aria-label="关闭" @click="close">✕</button>
        </header>
        <div class="modal__body"><slot /></div>
        <footer v-if="$slots.footer" class="modal__footer"><slot name="footer" /></footer>
      </div>
    </div>
  </teleport>
</template>

<style scoped>
.modal { position: fixed; inset: 0; z-index: 900; display: flex; align-items: flex-start; justify-content: center; padding: 8vh 16px 16px; overflow-y: auto; }
.modal__backdrop { position: fixed; inset: 0; background: rgba(18, 22, 31, 0.45); }
.modal__panel {
  position: relative; width: 100%; background: var(--color-bg-panel);
  border-radius: var(--radius-l); box-shadow: var(--shadow-pop);
  display: flex; flex-direction: column; max-height: 84vh;
}
.modal__header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-5) var(--space-6) var(--space-4); border-bottom: 1px solid var(--color-border-subtle);
}
.modal__title { margin: 0; font-size: 16px; font-weight: 600; }
.modal__close {
  border: none; background: none; cursor: pointer; color: var(--color-text-dim);
  font-size: 14px; padding: 4px 8px; border-radius: var(--radius-s);
}
.modal__close:hover { color: var(--color-text-primary); background: var(--color-bg-sunken); }
.modal__body { padding: var(--space-5) var(--space-6); overflow-y: auto; }
.modal__footer {
  display: flex; justify-content: flex-end; gap: var(--space-3);
  padding: var(--space-4) var(--space-6) var(--space-5); border-top: 1px solid var(--color-border-subtle);
}
</style>
