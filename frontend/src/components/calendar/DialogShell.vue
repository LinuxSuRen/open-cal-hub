<script setup lang="ts">
// 对话框外壳：Teleport 到 body；Esc 关闭（监听随组件卸载清理）；默认按钮关闭，不透传点击遮罩关闭以防误触丢数据。
import { onMounted, onUnmounted } from 'vue'
import { t } from '../../i18n'

const props = defineProps<{ visible: boolean; title: string; width?: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

function close() {
  emit('update:visible', false)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) close()
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <Transition name="dlg">
      <div v-if="visible" class="dlg-overlay">
        <div class="dlg" role="dialog" :aria-label="title" :style="width ? { width } : undefined">
          <header class="dlg__head">
            <h3 class="dlg__title">{{ title }}</h3>
            <button type="button" class="dlg__close" :aria-label="t('aria.close')" @click="close">✕</button>
          </header>
          <div class="dlg__body">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="dlg__foot">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dlg-overlay {
  position: fixed; inset: 0; z-index: 60;
  display: flex; align-items: flex-start; justify-content: center;
  padding: 8vh 16px 16px;
  background: rgba(18, 22, 31, 0.45);
}
.dlg {
  width: min(520px, 100%);
  max-height: 84vh;
  display: flex; flex-direction: column;
  background: var(--color-bg-elevated);
  border-radius: var(--radius-m);
  box-shadow: var(--shadow-pop);
}
.dlg__head {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-4) var(--space-5) 0;
}
.dlg__title { margin: 0; font-size: 16px; font-weight: 600; }
.dlg__close {
  border: none; background: none; cursor: pointer;
  color: var(--color-text-dim); font-size: 14px; padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-s);
}
.dlg__close:hover { color: var(--color-text-primary); background: var(--color-bg-sunken); }
.dlg__body { padding: var(--space-4) var(--space-5); overflow-y: auto; }
.dlg__foot {
  display: flex; justify-content: flex-end; gap: var(--space-3);
  padding: 0 var(--space-5) var(--space-5);
}
.dlg-enter-active, .dlg-leave-active { transition: opacity 0.15s ease; }
.dlg-enter-active .dlg, .dlg-leave-active .dlg { transition: transform 0.15s ease; }
.dlg-enter-from, .dlg-leave-to { opacity: 0; }
.dlg-enter-from .dlg, .dlg-leave-to .dlg { transform: translateY(-8px); }
</style>
