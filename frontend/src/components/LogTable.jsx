import AudioPlayer from './AudioPlayer'

function riskTag(summary = '') {
  const lower = summary.toLowerCase()
  if (lower.includes('critical')) return 'critical'
  if (lower.includes('high')) return 'high'
  if (lower.includes('medium')) return 'medium'
  return 'low'
}

export default function LogTable({ logs = [] }) {
  return (
    <div className="overflow-x-auto rounded-xl border border-zinc-800">
      <table className="min-w-full bg-zinc-950/40 text-sm">
        <thead className="bg-black/40 text-left text-xs uppercase tracking-wider text-zinc-400">
          <tr>
            <th className="px-3 py-2">When</th>
            <th className="px-3 py-2">Summary</th>
            <th className="px-3 py-2">Risk</th>
            <th className="px-3 py-2">Solana TX</th>
            <th className="px-3 py-2">Audio</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => (
            <tr key={`${log.solana_tx || log.hash}-${log.when}`} className="border-t border-zinc-800/80">
              <td className="px-3 py-2 text-xs text-zinc-500">{new Date(log.when).toLocaleString()}</td>
              <td className="px-3 py-2 text-zinc-100">{log.summary}</td>
              <td className="px-3 py-2">
                <span className="rounded border border-crimson/50 px-2 py-1 text-xs text-zinc-300">{riskTag(log.summary)}</span>
              </td>
              <td className="px-3 py-2 text-xs">
                {log.solana_tx && !String(log.solana_tx).startsWith('dev-stub-') ? (
                  <a
                    className="text-crimson hover:underline"
                    href={`https://explorer.solana.com/tx/${log.solana_tx}?cluster=devnet`}
                    rel="noreferrer"
                    target="_blank"
                  >
                    {log.solana_tx.slice(0, 14)}...
                  </a>
                ) : log.solana_tx && String(log.solana_tx).startsWith('dev-stub-') ? (
                  <span className="text-amber-500">stub</span>
                ) : (
                  <span className="text-zinc-600">pending</span>
                )}
              </td>
              <td className="min-w-52 px-3 py-2">
                <AudioPlayer src={log.audio_url} />
              </td>
            </tr>
          ))}
          {logs.length === 0 && (
            <tr>
              <td colSpan={5} className="px-3 py-6 text-center text-zinc-500">
                No logs available.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
