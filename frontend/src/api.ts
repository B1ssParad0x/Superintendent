import axios, { AxiosError } from 'axios'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8000'

export const api = axios.create({
  baseURL: API_URL,
  headers: { 'Content-Type': 'application/json' },
})

/** Extract user-friendly error message from API or axios error */
export function getErrorMessage(err: unknown): string {
  if (err instanceof AxiosError) {
    const msg = err.response?.data?.error ?? err.response?.data?.message
    if (typeof msg === 'string') return msg
    if (err.response?.status === 401) return 'Unauthorized'
    if (err.response?.status === 403) return 'Access denied'
    if (err.message) return err.message
  }
  return err instanceof Error ? err.message : 'Request failed'
}

// Attach JWT from Auth0
export function setAuthToken(token: string) {
  api.defaults.headers.common['Authorization'] = `Bearer ${token}`
}

export interface CityState {
  status: string
  updated: string
  alerts: number
  summary?: string
}

export interface Decision {
  when: string
  summary: string
  hash: string
  audio_url: string
  solana_tx: string
}

export interface TelemetryPoint {
  node_id: string
  ts: string
  loc: { lat: number; lon: number }
  metrics?: Record<string, unknown>
}

export async function getTelemetry() {
  const { data } = await api.get<{ telemetry: TelemetryPoint[] }>('/api/telemetry')
  return data.telemetry || []
}

export async function getState() {
  const { data } = await api.get<CityState>('/api/state')
  return data
}

export async function getLogs(token: string) {
  const { data } = await api.get<{ decisions: Decision[] }>('/api/logs', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return data.decisions
}

export async function triggerReason(token: string) {
  const { data } = await api.post('/api/reason', {}, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return data
}

export async function commitDecision(token: string, summary: string, audioUrl?: string) {
  const { data } = await api.post('/api/commit', { summary, audio_url: audioUrl }, {
    headers: { Authorization: `Bearer ${token}` },
  })
  return data
}
