import { createRouter, createWebHistory } from 'vue-router'
import MainView from '../views/MainView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: MainView,
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/RegisterView.vue'),
    },
    {
      path: '/admin',
      name: 'admin',
      component: () => import('../views/AdminView.vue'),
      meta: { requiresAdmin: true },
    },
  ],
})

// Navigation guard
router.beforeEach((to) => {
  const token = localStorage.getItem('token')
  const isLoggedIn = !!token

  // Public pages
  if (to.path === '/login' || to.path === '/register') {
    if (isLoggedIn) return '/'
    return true
  }

  // Protected pages
  if (!isLoggedIn) {
    return '/login'
  }

  // Admin pages
  if (to.meta.requiresAdmin) {
    const savedUser = localStorage.getItem('user')
    if (savedUser) {
      try {
        const user = JSON.parse(savedUser)
        if (user.role !== 'admin') return '/'
      } catch {
        return '/'
      }
    } else {
      return '/'
    }
  }

  return true
})

export default router
