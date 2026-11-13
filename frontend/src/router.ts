import { createRouter, createWebHistory } from 'vue-router'

// 路由契约见 docs/api-contract.md §8。
// 视图组件由各前端 agent 实现；此处先以懒加载占位模块接线。
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    // 认证流程
    { path: '/login', component: () => import('./views/auth/Login.vue'), meta: { guest: true } },
    { path: '/register', component: () => import('./views/auth/Register.vue'), meta: { guest: true } },
    { path: '/forgot', component: () => import('./views/auth/Forgot.vue'), meta: { guest: true } },
    { path: '/reset', component: () => import('./views/auth/Reset.vue'), meta: { guest: true } },
    { path: '/verify', component: () => import('./views/auth/Verify.vue'), meta: { guest: true } },

    // 用户端
    { path: '/app', component: () => import('./views/app/AppHome.vue') },
    { path: '/app/calendar', component: () => import('./views/app/CalendarPage.vue') },
    { path: '/app/calendars', component: () => import('./views/app/CalendarsPage.vue') },
    { path: '/app/booking', component: () => import('./views/app/BookingConfigPage.vue') },
    { path: '/app/bookings', component: () => import('./views/app/MyBookingsPage.vue') },
    { path: '/app/settings', component: () => import('./views/app/SettingsPage.vue') },

    // 管理端
    { path: '/admin', component: () => import('./views/admin/AdminHome.vue'), meta: { admin: true } },
    { path: '/admin/:section', component: () => import('./views/admin/AdminHome.vue'), meta: { admin: true } },

    // 公开页
    { path: '/u/:username', component: () => import('./views/pub/BookingProfile.vue'), meta: { public: true } },
    { path: '/u/:username/:slug', component: () => import('./views/pub/BookingPage.vue'), meta: { public: true } },
    { path: '/s/:token', component: () => import('./views/pub/ShareView.vue'), meta: { public: true } },
    { path: '/b/:token', component: () => import('./views/pub/BookingManage.vue'), meta: { public: true } },

    { path: '/', redirect: '/app' },
    { path: '/:pathMatch(.*)*', component: () => import('./views/NotFound.vue') },
  ],
})
