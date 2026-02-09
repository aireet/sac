import axios from 'axios'
import router from '../router'

const getApiBaseUrl = () => {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL
  }
  const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:'
  const host = window.location.hostname
  const port = window.location.port
  // Production (standard ports): use same-origin path-based routing via Istio
  if (!port || port === '80' || port === '443') {
    return `${protocol}//${host}/api`
  }
  // Development: use separate port
  return `${protocol}//${host}:8080/api`
}

const api = axios.create({
  baseURL: getApiBaseUrl(),
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor: attach JWT token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor: handle 401 → redirect to login
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      // Only redirect if not already on login/register page
      const currentPath = router.currentRoute.value.path
      if (currentPath !== '/login' && currentPath !== '/register') {
        router.push('/login')
      }
    }
    return Promise.reject(error)
  }
)

export default api

// Re-export the base URL helper for WebSocket usage
export const getWsBaseUrl = () => {
  if (import.meta.env.VITE_WS_URL) {
    return import.meta.env.VITE_WS_URL
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.hostname
  const port = window.location.port
  // Production (standard ports): use same-origin via Istio
  if (!port || port === '80' || port === '443') {
    return `${protocol}//${host}`
  }
  // Development: use separate port
  return `${protocol}//${host}:8081`
}
