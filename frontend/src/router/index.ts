import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
  { path: '/config', name: 'config', component: () => import('../views/ConfigView.vue') },
  { path: '/sensitivity', name: 'sensitivity', component: () => import('../views/SensitivityView.vue') },
  { path: '/replay', name: 'replay', component: () => import('../views/ReplayView.vue') },
  { path: '/report/:runId', name: 'report', component: () => import('../views/ReportView.vue'), props: true },
  { path: '/:pathMatch(.*)*', redirect: '/' }
]

export default createRouter({
  history: createWebHistory(),
  routes,
  linkActiveClass: 'text-glow-cyan border-glow-cyan'
})
