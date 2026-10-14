import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

// / is Today across every workspace; /calendar every workspace's calendar.
// A workspace lives under /w/:ws with one route per module; pages are
// /w/:ws/p/:page. The account and instance areas are as in the blueprint.
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
      { path: '', name: 'home', component: () => import('@/views/HomeView.vue') },
      { path: 'calendar', name: 'calendar', component: () => import('@/views/CalendarView.vue') },
      { path: 'notes', name: 'my-notes', component: () => import('@/views/NotesView.vue') },
      { path: 'w/:ws', name: 'ws', component: () => import('@/views/HomeView.vue') },
      { path: 'w/:ws/tasks/:list?', name: 'ws-tasks', component: () => import('@/views/TasksView.vue') },
      { path: 'w/:ws/board/:list?', name: 'ws-board', component: () => import('@/views/BoardView.vue') },
      { path: 'w/:ws/notes', name: 'ws-notes', component: () => import('@/views/NotesView.vue') },
      { path: 'w/:ws/timeline', name: 'ws-timeline', component: () => import('@/views/TimelineView.vue') },
      { path: 'w/:ws/charts', name: 'ws-charts', component: () => import('@/views/ChartsView.vue') },
      { path: 'w/:ws/calendar', name: 'ws-calendar', component: () => import('@/views/CalendarView.vue') },
      { path: 'w/:ws/pages', name: 'ws-pages', component: () => import('@/views/PagesView.vue') },
      { path: 'w/:ws/p/:page', name: 'ws-page', component: () => import('@/views/PageView.vue') },
      { path: 'w/:ws/trash', name: 'ws-trash', component: () => import('@/views/TrashView.vue') },
      { path: 'audit', name: 'audit', component: () => import('@/views/AuditView.vue'), meta: { privilege: 'audit.read' } },
      { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
      { path: 'admin', name: 'admin', component: () => import('@/views/AdminView.vue'), meta: { role: 'admin', area: 'admin' } },
      // Owner-only area: same view, owner tabs only (selected via meta.area).
      { path: 'owner', name: 'owner', component: () => import('@/views/AdminView.vue'), meta: { role: 'owner', area: 'owner' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'home' }
  }
  // Admin-only routes.
  if (to.meta.role === 'admin' && !auth.isAdminArea) {
    return { name: 'home' }
  }
  if (typeof to.meta.privilege === 'string' && !auth.can(to.meta.privilege)) {
    return { name: 'home' }
  }
  // Pages behind a feature flag the owner turned off.
  if (typeof to.meta.feature === 'string' && auth.features[to.meta.feature] === false) {
    return { name: 'home' }
  }
  // Owner-only area.
  if (to.meta.role === 'owner' && !auth.isOwner) {
    return { name: 'home' }
  }
})
