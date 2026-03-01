import { useEffect, useMemo, useState } from 'react'
import MapView from '../components/MapView'
import NodeStatusCard from '../components/NodeStatusCard'
import { getNodes, getSessionCity, getTelemetry } from '../context/appApi'
import { useFetch } from '../hooks/useFetch'

export default function Nodes() {
  const [activeCity, setActiveCity] = useState(null)
  const nodesQuery = useFetch(() => getNodes(activeCity), 8_000, [activeCity?.city_id])
  const telemetryQuery = useFetch(() => getTelemetry(activeCity), 8_000, [activeCity?.city_id])
  const [selected, setSelected] = useState(null)

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const city = await getSessionCity()
        if (mounted) setActiveCity(city)
      } catch {
        // keep default behavior
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  const stats = useMemo(() => {
    const nodes = nodesQuery.data || []
    const telemetry = telemetryQuery.data || []
    return {
      nodes: nodes.length,
      telemetry: telemetry.length,
      healthy: nodes.filter((n) => n.heartbeat === 'healthy').length,
      degraded: nodes.filter((n) => n.heartbeat === 'degraded').length,
      stale: nodes.filter((n) => n.heartbeat === 'stale').length,
    }
  }, [nodesQuery.data, telemetryQuery.data])

  const liveStream = useMemo(() => {
    return (telemetryQuery.data || []).slice(0, 24)
  }, [telemetryQuery.data])

  return (
    <main className="mx-auto grid max-w-7xl gap-4 px-4 py-4 lg:grid-cols-[1.5fr_1fr]">
      <section className="panel rounded-xl p-3">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 border-b border-zinc-800 pb-3">
          <div>
            <p className="text-xs uppercase tracking-widest text-zinc-500">Field network</p>
            <h2 className="font-display text-lg text-zinc-100">
              {activeCity?.city_name || 'Default City'} {activeCity?.country_code ? `(${activeCity.country_code})` : ''}
            </h2>
          </div>
          <button onClick={() => Promise.all([nodesQuery.refresh(), telemetryQuery.refresh()])} className="rounded border border-zinc-700 px-3 py-1 text-xs text-zinc-200">
            Refresh now
          </button>
        </div>
        <div className="mb-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
          <p className="rounded border border-zinc-800 bg-black/30 px-3 py-2 text-xs text-zinc-400">Nodes: <span className="text-zinc-100">{stats.nodes}</span></p>
          <p className="rounded border border-zinc-800 bg-black/30 px-3 py-2 text-xs text-zinc-400">Telemetry: <span className="text-zinc-100">{stats.telemetry}</span></p>
          <p className="rounded border border-emerald-700/40 bg-emerald-950/20 px-3 py-2 text-xs text-emerald-200">Healthy: {stats.healthy}</p>
          <p className="rounded border border-amber-700/40 bg-amber-950/20 px-3 py-2 text-xs text-amber-200">Degraded: {stats.degraded}</p>
          <p className="rounded border border-zinc-700/40 bg-zinc-900/30 px-3 py-2 text-xs text-zinc-300">Stale: {stats.stale}</p>
        </div>
        <MapView telemetry={telemetryQuery.data || []} nodes={nodesQuery.data || []} />
        {telemetryQuery.error && <p className="mt-2 text-xs text-red-400">{telemetryQuery.error}</p>}
      </section>
      <section className="space-y-3">
        {(nodesQuery.data || []).map((node) => (
          <NodeStatusCard key={node.node_id} node={node} onOpen={setSelected} />
        ))}
        {!nodesQuery.data?.length && <p className="panel rounded-xl p-4 text-sm text-zinc-500">No active nodes detected.</p>}
        <section className="panel rounded-xl p-4">
          <h3 className="mb-2 text-xs uppercase tracking-widest text-zinc-500">Live telemetry stream</h3>
          <div className="max-h-56 space-y-2 overflow-auto pr-1">
            {liveStream.length === 0 ? (
              <p className="text-xs text-zinc-500">No telemetry points yet.</p>
            ) : (
              liveStream.map((item) => (
                <div key={`${item.node_id}-${item.ts}`} className="rounded border border-zinc-800 bg-black/30 px-2 py-2">
                  <p className="text-xs text-zinc-400">
                    {item.node_id} · {new Date(item.ts).toLocaleTimeString()}
                  </p>
                  <p className="truncate text-xs text-zinc-300">{Object.keys(item.metrics || {}).slice(0, 4).join(' · ') || 'No metrics'}</p>
                </div>
              ))
            )}
          </div>
        </section>
      </section>

      {selected && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="panel w-full max-w-lg rounded-xl p-5">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="font-display text-lg">{selected.node_id}</h3>
              <button className="text-sm text-zinc-400" onClick={() => setSelected(null)}>
                close
              </button>
            </div>
            <pre className="max-h-80 overflow-auto rounded bg-black/40 p-3 text-xs text-zinc-300">
              {JSON.stringify(selected, null, 2)}
            </pre>
          </div>
        </div>
      )}
    </main>
  )
}
