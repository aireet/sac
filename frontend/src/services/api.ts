import axios from 'axios'
import router from '../router'

const getApiBaseUrl = () => {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL
  }
  const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:'
  const host = window.location.hostname
  const port = window.location.port
  // Development: localhost with separate backend port
  if (host === 'localhost' || host === '127.0.0.1') {
    return `${protocol}//${host}:8080/api`
  }
  // Production / Gateway: same-origin path-based routing (works with any port)
  return `${protocol}//${window.location.host}/api`
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
  // Development: localhost with separate ws-proxy port
  if (host === 'localhost' || host === '127.0.0.1') {
    return `${protocol}//${host}:8081`
  }
  // Production / Gateway: same-origin routing (works with any port)
  return `${protocol}//${window.location.host}`
}
