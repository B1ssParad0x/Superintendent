export default function NodeStatusCard({ node, onOpen }) {
  const dot =
    node.heartbeat === 'healthy'
      ? 'bg-emerald-400'
      : node.heartbeat === 'degraded'
        ? 'bg-amber-400'
        : 'bg-zinc-600'

  return (
    <button onClick={() => onOpen(node)} className="panel w-full rounded-lg p-3 text-left hover:border-crimson/60">
      <div className="mb-2 flex items-center justify-between">
        <span className="font-medium text-zinc-100">{node.node_id}</span>
        <span className={`h-2.5 w-2.5 rounded-full ${dot}`} />
      </div>
      <p className="text-xs text-zinc-500">
        {Number(node?.loc?.lat || 0).toFixed(4)}, {Number(node?.loc?.lon || 0).toFixed(4)}
      </p>
      <p className="mt-1 text-xs text-zinc-500">Heartbeat: {node.heartbeat}</p>
      <p className="mt-1 text-xs text-zinc-500">Last: {new Date(node.ts).toLocaleString()}</p>
    </button>
  )
}
