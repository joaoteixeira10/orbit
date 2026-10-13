import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('@/components/AppShell.vue'),
    children: [
      { path: '', redirect: { name: 'proxies' } },
      { path: 'proxies/:name?', name: 'proxies', component: () => import('@/views/ProxiesView.vue') },
      { path: 'operations', redirect: { name: 'operations-services' } },
      { path: 'operations/services', name: 'operations-services', component: () => import('@/views/OperationsView.vue') },
      { path: 'operations/servers', name: 'operations-servers', component: () => import('@/views/OperationsView.vue') },
      { path: 'certificates', name: 'certificates', component: () => import('@/views/CertificatesView.vue'), meta: { privilege: 'certs.read', feature: 'certificates' } },
      { path: 'audit', name: 'audit', component: () => import('@/views/AuditView.vue'), meta: { privilege: 'audit.read' } },
      // "Your access": what you may see and change, and where.
      { path: 'access', name: 'dashboard', component: () => import('@/views/DashboardView.vue') },
      { path: 'dashboard', redirect: { name: 'dashboard' } },
      { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
      { path: 'admin', name: 'admin', component: () => import('@/views/AdminView.vue'), meta: { role: 'admin', area: 'admin' } },
      // Owner-only area: same view, owner tabs only (selected via meta.area).
      { path: 'owner', name: 'owner', component: () => import('@/views/AdminView.vue'), meta: { role: 'owner', area: 'owner' } },
    ],
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'proxies' }
  }
  // Admin-only routes.
  if (to.meta.role === 'admin' && !auth.isAdminArea) {
    return { name: 'dashboard' }
  }
  if (typeof to.meta.privilege === 'string' && !auth.can(to.meta.privilege)) {
    return { name: 'dashboard' }
  }
  // Pages behind a feature flag the owner turned off.
  if (typeof to.meta.feature === 'string' && auth.features[to.meta.feature] === false) {
    return { name: 'dashboard' }
  }
  // Owner-only area.
  if (to.meta.role === 'owner' && !auth.isOwner) {
    return { name: 'dashboard' }
  }
})
