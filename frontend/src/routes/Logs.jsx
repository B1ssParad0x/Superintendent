import { useEffect, useMemo, useState } from 'react'
import AudioPlayer from '../components/AudioPlayer'
import LogTable from '../components/LogTable'
import { getLogs, getSessionCity } from '../context/appApi'
import { useFetch } from '../hooks/useFetch'

function riskFromSummary(summary = '') {
  const s = summary.toLowerCase()
  if (s.includes('critical')) return 'critical'
  if (s.includes('high')) return 'high'
  if (s.includes('medium')) return 'medium'
  return 'low'
}

function hasRealSolanaCommit(tx) {
  const value = String(tx || '').trim()
  return Boolean(value) && !value.startsWith('dev-stub-')
}

export default function Logs() {
  const [activeCity, setActiveCity] = useState(null)
  const { data = [], error, loading, refresh } = useFetch(() => getLogs(activeCity), 12_000, [activeCity?.city_id])
  const [risk, setRisk] = useState('all')
  const [date, setDate] = useState('')
  const [node, setNode] = useState('all')
  const [tx, setTx] = useState('all')
  const [query, setQuery] = useState('')
  const rows = Array.isArray(data) ? data : []

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const city = await getSessionCity()
        if (mounted) setActiveCity(city)
      } catch {
        // default city still works
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  const nodes = useMemo(() => {
    const names = new Set()
    rows.forEach((d) => {
      const match = d.summary?.match(/node[\s:-]+([a-zA-Z0-9-_]+)/i)
      if (match?.[1]) names.add(match[1])
    })
    return Array.from(names)
  }, [rows])

  const filtered = useMemo(() => {
    return rows.filter((entry) => {
      const byRisk = risk === 'all' ? true : riskFromSummary(entry.summary) === risk
      const byDate = date ? new Date(entry.when).toDateString() === new Date(date).toDateString() : true
      const nodeMatch = entry.summary?.match(/node[\s:-]+([a-zA-Z0-9-_]+)/i)?.[1]
      const byNode = node === 'all' ? true : nodeMatch === node
      const byTx = tx === 'all' ? true : tx === 'committed' ? hasRealSolanaCommit(entry.solana_tx) : !hasRealSolanaCommit(entry.solana_tx)
      const byQuery = query.trim() ? entry.summary?.toLowerCase().includes(query.trim().toLowerCase()) : true
      return byRisk && byDate && byNode && byTx && byQuery
    })
  }, [rows, risk, date, node, tx, query])

  const totals = useMemo(() => {
    return {
      total: rows.length,
      committed: rows.filter((x) => hasRealSolanaCommit(x.solana_tx)).length,
      withAudio: rows.filter((x) => Boolean(x.audio_url)).length,
    }
  }, [rows])

  return (
    <main className="mx-auto max-w-7xl space-y-4 px-4 py-4">
      <section className="panel rounded-xl p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 border-b border-zinc-800 pb-3">
          <h2 className="font-display text-lg text-zinc-100">
            Decision logs · {activeCity?.city_name || 'Default City'} {activeCity?.country_code ? `(${activeCity.country_code})` : ''}
          </h2>
          <button onClick={refresh} className="rounded border border-zinc-700 px-3 py-1 text-xs text-zinc-200">
            Refresh
          </button>
        </div>
        <div className="mb-3 grid gap-2 sm:grid-cols-3">
          <p className="rounded border border-zinc-800 bg-black/30 px-3 py-2 text-xs text-zinc-300">Total decisions: {totals.total}</p>
          <p className="rounded border border-crimson/40 bg-crimson/10 px-3 py-2 text-xs text-zinc-100">Committed to Solana: {totals.committed}</p>
          <p className="rounded border border-zinc-700 bg-zinc-900/40 px-3 py-2 text-xs text-zinc-300">Voice advisories: {totals.withAudio}</p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <label className="text-xs text-zinc-400">
            Risk
            <select className="ml-2 rounded border border-zinc-700 bg-black px-2 py-1" value={risk} onChange={(e) => setRisk(e.target.value)}>
              <option value="all">all</option>
              <option value="critical">critical</option>
              <option value="high">high</option>
              <option value="medium">medium</option>
              <option value="low">low</option>
            </select>
          </label>
          <label className="text-xs text-zinc-400">
            Date
            <input className="ml-2 rounded border border-zinc-700 bg-black px-2 py-1" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </label>
          <label className="text-xs text-zinc-400">
            Node
            <select className="ml-2 rounded border border-zinc-700 bg-black px-2 py-1" value={node} onChange={(e) => setNode(e.target.value)}>
              <option value="all">all</option>
              {nodes.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </select>
          </label>
          <label className="text-xs text-zinc-400">
            TX
            <select className="ml-2 rounded border border-zinc-700 bg-black px-2 py-1" value={tx} onChange={(e) => setTx(e.target.value)}>
              <option value="all">all</option>
              <option value="committed">committed</option>
              <option value="pending">pending</option>
            </select>
          </label>
          <label className="text-xs text-zinc-400">
            Search
            <input
              className="ml-2 rounded border border-zinc-700 bg-black px-2 py-1"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="summary text"
            />
          </label>
        </div>
      </section>

      <LogTable logs={filtered} />

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">Latest Voice</h3>
        <AudioPlayer src={filtered.find((x) => x.audio_url)?.audio_url} />
        {loading && <p className="mt-2 text-xs text-zinc-500">Loading logs...</p>}
        {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
        {error?.includes('403') && (
          <p className="mt-2 text-xs text-amber-300">Logs require an admin role. If this persists, verify your Auth0 role mapping.</p>
        )}
      </section>
    </main>
  )
}
