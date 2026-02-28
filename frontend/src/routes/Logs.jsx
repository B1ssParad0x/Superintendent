import { useMemo, useState } from 'react'
import AudioPlayer from '../components/AudioPlayer'
import LogTable from '../components/LogTable'
import { getLogs } from '../context/appApi'
import { useFetch } from '../hooks/useFetch'

function riskFromSummary(summary = '') {
  const s = summary.toLowerCase()
  if (s.includes('critical')) return 'critical'
  if (s.includes('high')) return 'high'
  if (s.includes('medium')) return 'medium'
  return 'low'
}

export default function Logs() {
  const { data = [], error, loading, refresh } = useFetch(getLogs, 20_000, [])
  const [risk, setRisk] = useState('all')
  const [date, setDate] = useState('')
  const [node, setNode] = useState('all')

  const nodes = useMemo(() => {
    const names = new Set()
    data.forEach((d) => {
      const match = d.summary?.match(/node[\s:-]+([a-zA-Z0-9-_]+)/i)
      if (match?.[1]) names.add(match[1])
    })
    return Array.from(names)
  }, [data])

  const filtered = useMemo(() => {
    return data.filter((entry) => {
      const byRisk = risk === 'all' ? true : riskFromSummary(entry.summary) === risk
      const byDate = date ? new Date(entry.when).toDateString() === new Date(date).toDateString() : true
      const nodeMatch = entry.summary?.match(/node[\s:-]+([a-zA-Z0-9-_]+)/i)?.[1]
      const byNode = node === 'all' ? true : nodeMatch === node
      return byRisk && byDate && byNode
    })
  }, [data, risk, date, node])

  return (
    <main className="mx-auto max-w-7xl space-y-4 px-4 py-4">
      <section className="panel rounded-xl p-4">
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
          <button onClick={refresh} className="rounded border border-zinc-700 px-3 py-1 text-xs text-zinc-200">
            Refresh
          </button>
        </div>
      </section>

      <LogTable logs={filtered} />

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">Latest Voice</h3>
        <AudioPlayer src={filtered.find((x) => x.audio_url)?.audio_url} />
        {loading && <p className="mt-2 text-xs text-zinc-500">Loading logs...</p>}
        {error && <p className="mt-2 text-xs text-red-400">{error}</p>}
      </section>
    </main>
  )
}
