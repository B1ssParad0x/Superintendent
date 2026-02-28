import { api } from './apiClient'

export async function getState() {
  const { data } = await api.get('/api/state')
  return data
}

export async function getTelemetry() {
  const { data } = await api.get('/api/telemetry')
  return data?.telemetry || []
}

export async function getLogs() {
  const { data } = await api.get('/api/logs')
  return data?.decisions || []
}

export async function triggerReason() {
  const { data } = await api.post('/api/reason', {})
  return data
}

export async function commitDecision(summary, audioUrl = '') {
  const { data } = await api.post('/api/commit', { summary, audio_url: audioUrl })
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

export async function getNodes() {
  const telemetry = await getTelemetry()
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
      healthy,
      heartbeat: healthy ? 'healthy' : ageMs < 15 * 60 * 1000 ? 'degraded' : 'stale',
      risk: inferRisk(JSON.stringify(node.metrics || {})),
    }
  })
}
