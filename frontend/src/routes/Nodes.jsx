import { useState } from 'react'
import MapView from '../components/MapView'
import NodeStatusCard from '../components/NodeStatusCard'
import { getNodes, getTelemetry } from '../context/appApi'
import { useFetch } from '../hooks/useFetch'

export default function Nodes() {
  const nodesQuery = useFetch(getNodes, 12_000, [])
  const telemetryQuery = useFetch(getTelemetry, 12_000, [])
  const [selected, setSelected] = useState(null)

  return (
    <main className="mx-auto grid max-w-7xl gap-4 px-4 py-4 lg:grid-cols-[1.5fr_1fr]">
      <section className="panel rounded-xl p-3">
        <MapView telemetry={telemetryQuery.data || []} nodes={nodesQuery.data || []} />
      </section>
      <section className="space-y-3">
        {(nodesQuery.data || []).map((node) => (
          <NodeStatusCard key={node.node_id} node={node} onOpen={setSelected} />
        ))}
        {!nodesQuery.data?.length && <p className="panel rounded-xl p-4 text-sm text-zinc-500">No active nodes detected.</p>}
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
