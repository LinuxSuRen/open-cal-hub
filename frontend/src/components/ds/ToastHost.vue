<script setup lang="ts">
// toast 渲染宿主：固定顶部居中；每页各挂一个即可。
import { toasts, dismissToast } from './toast'
</script>

<template>
  <div class="toast-host" aria-live="polite">
    <transition-group name="toast-fade">
      <div v-for="item in toasts" :key="item.id" class="toast" :class="`toast--${item.kind}`" @click="dismissToast(item.id)">
        <span v-if="item.kind === 'success'" class="toast__dot toast__dot--success">✓</span>
        <span v-else-if="item.kind === 'error'" class="toast__dot toast__dot--error">!</span>
        <span class="toast__text">{{ item.text }}</span>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.toast-host {
  position: fixed; top: 16px; left: 50%; transform: translateX(-50%);
  z-index: 1000; display: flex; flex-direction: column; align-items: center; gap: 8px;
  pointer-events: none; width: min(92vw, 480px);
}
.toast {
  pointer-events: auto; display: flex; align-items: center; gap: 10px;
  padding: 10px 16px; border-radius: var(--radius-m);
  background: var(--color-text-bright); color: #fff; font-size: 14px;
  box-shadow: var(--shadow-pop); cursor: pointer; max-width: 100%;
}
.toast__text { word-break: break-all; }
.toast__dot {
  flex: none; width: 18px; height: 18px; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 12px; font-weight: 700;
}
.toast__dot--success { background: var(--color-success); }
.toast__dot--error { background: var(--color-danger); }
.toast-fade-enter-active, .toast-fade-leave-active { transition: all 0.18s ease; }
.toast-fade-enter-from, .toast-fade-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
