import { api } from './apiClient'

export async function getState() {
  const { data } = await api.get('/api/state')
  return data
}

export async function getStateByCity(city) {
  const params = city?.city_id ? { city_id: city.city_id, city_name: city.city_name, country_code: city.country_code } : undefined
  const { data } = await api.get('/api/state', { params })
  return data
}

export async function getTelemetry(city) {
  const params = city?.city_id ? { city_id: city.city_id } : undefined
  const { data } = await api.get('/api/telemetry', { params })
  return data?.telemetry || []
}

export async function getLogs(city) {
  const params = city?.city_id ? { city_id: city.city_id } : undefined
  const { data } = await api.get('/api/logs', { params })
  return data?.decisions || []
}

export async function triggerReason(city) {
  const params = city?.city_id ? { city_id: city.city_id } : undefined
  const { data } = await api.post('/api/reason', {}, { params })
  return data
}

export async function commitDecision(summary, audioUrl = '', city) {
  const payload = { summary, audio_url: audioUrl, city_id: city?.city_id || '' }
  const { data } = await api.post('/api/commit', payload)
  return data
}

function inferRisk(summary = '') {
  const s = summary.toLowerCase()
  if (s.includes('critical')) return 95
  if (s.includes('high')) return 80
  if (s.includes('medium')) return 55
  if (s.includes('low')) return 25
  return 35
}

export async function getNodes(city) {
  const telemetry = await getTelemetry(city)
  const byNode = new Map()
  for (const point of telemetry) {
    const existing = byNode.get(point.node_id)
    if (!existing || new Date(point.ts).getTime() > new Date(existing.ts).getTime()) {
      byNode.set(point.node_id, point)
    }
  }
  return Array.from(byNode.values()).map((node) => {
    const ageMs = Date.now() - new Date(node.ts).getTime()
    const healthy = ageMs < 3 * 60 * 1000
    return {
      node_id: node.node_id,
      ts: node.ts,
      loc: node.loc,
      metrics: node.metrics || {},
      city_name: node.city_name,
      country_code: node.country_code,
      healthy,
      heartbeat: healthy ? 'healthy' : ageMs < 15 * 60 * 1000 ? 'degraded' : 'stale',
      risk: inferRisk(JSON.stringify(node.metrics || {})),
      age_min: Math.max(0, Math.round(ageMs / 60000)),
    }
  }).sort((a, b) => new Date(b.ts).getTime() - new Date(a.ts).getTime())
}

export async function searchCities(query) {
  const { data } = await api.get('/api/cities/search', { params: { q: query } })
  return data?.cities || []
}

export async function getSessionCity() {
  const { data } = await api.get('/api/session/city')
  return data
}

export async function setSessionCity(city) {
  const { data } = await api.post('/api/session/city', city)
  return data
}

export async function listChatThreads() {
  const { data } = await api.get('/api/chat/threads')
  return data?.threads || []
}

export async function createChatThread(title = '') {
  const { data } = await api.post('/api/chat/thread', { title })
  return data
}

export async function getChatMessages(threadId) {
  const { data } = await api.get(`/api/chat/thread/${encodeURIComponent(threadId)}/messages`)
  return data?.messages || []
}

export async function sendChatMessage(threadId, content) {
  const { data } = await api.post(`/api/chat/thread/${encodeURIComponent(threadId)}/message`, { content })
  return data
}
