import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { bootAuth, authState } from './stores/auth'
import './style.css'

// 引导：恢复会话 → 鉴权路由守卫 → 挂载
router.beforeEach(async (to) => {
  await bootAuth()
  const authed = authState.user !== null
  if (to.meta.guest && authed) return '/app'
  if (!to.meta.guest && !to.meta.public && !authed) {
    return to.fullPath === '/' ? '/login' : { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.admin && authState.user?.role !== 'admin') return '/app'
  return true
})

createApp(App).use(router).mount('#app')
