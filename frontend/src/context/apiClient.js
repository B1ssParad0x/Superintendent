import axios from 'axios'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8000'

let tokenGetter = null

export function setTokenGetter(fn) {
  tokenGetter = fn
}

export const api = axios.create({
  baseURL: API_URL,
  headers: { 'Content-Type': 'application/json' },
})

api.interceptors.request.use(async (config) => {
  if (!tokenGetter) return config
  try {
    const token = await tokenGetter()
    if (token) {
      config.headers = config.headers || {}
      config.headers.Authorization = `Bearer ${token}`
    }
  } catch {
    // Keep requests working in public mode even when token retrieval fails.
  }
  return config
})

export function getErrorMessage(err) {
  const apiMessage = err?.response?.data?.error || err?.response?.data?.message
  if (typeof apiMessage === 'string' && apiMessage.trim()) return apiMessage
  if (err?.response?.status === 401) return 'Unauthorized. Sign in again.'
  if (err?.response?.status === 403) return 'Forbidden. Admin role required.'
  if (typeof err?.message === 'string') return err.message
  return 'Request failed.'
}
