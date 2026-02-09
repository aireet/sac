import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '../services/api'

export interface User {
  id: number
  username: string
  email: string
  display_name: string
  role: string
  created_at: string
  updated_at: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  const user = ref<User | null>(null)

  // Restore user from localStorage on init
  const savedUser = localStorage.getItem('user')
  if (savedUser) {
    try {
      user.value = JSON.parse(savedUser)
    } catch {
      localStorage.removeItem('user')
    }
  }

  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')
  const userId = computed(() => user.value?.id ?? 0)

  async function login(username: string, password: string) {
    const response = await api.post('/auth/login', { username, password })
    token.value = response.data.token
    user.value = response.data.user
    localStorage.setItem('token', response.data.token)
    localStorage.setItem('user', JSON.stringify(response.data.user))
  }

  async function register(data: { username: string; email: string; password: string; display_name?: string }) {
    const response = await api.post('/auth/register', data)
    token.value = response.data.token
    user.value = response.data.user
    localStorage.setItem('token', response.data.token)
    localStorage.setItem('user', JSON.stringify(response.data.user))
  }

  async function fetchCurrentUser() {
    const response = await api.get('/auth/me')
    user.value = response.data
    localStorage.setItem('user', JSON.stringify(response.data))
  }

  function logout() {
    token.value = null
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  return {
    token,
    user,
    isLoggedIn,
    isAdmin,
    userId,
    login,
    register,
    fetchCurrentUser,
    logout,
  }
})
