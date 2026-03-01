import { useEffect, useMemo } from 'react'
import AdvisoryCard from '../components/AdvisoryCard'
import AudioPlayer from '../components/AudioPlayer'
import MapView from '../components/MapView'
import RiskGauge from '../components/RiskGauge'
import SystemMoodOrb from '../components/SystemMoodOrb'
import { getState, getTelemetry, getLogs } from '../context/appApi'
import { useAppState } from '../context/useAppState'
import { useFetch } from '../hooks/useFetch'

export default function Dashboard() {
  const summary = useAppState((s) => s.summary)
  const risk = useAppState((s) => s.risk)
  const advisories = useAppState((s) => s.advisories)
  const setFromState = useAppState((s) => s.setFromState)

  const stateQuery = useFetch(getState, 10_000, [])
  const telemetryQuery = useFetch(getTelemetry, 12_000, [])
  const logsQuery = useFetch(getLogs, 20_000, [])

  useEffect(() => {
    if (!stateQuery.data) return
    const latestSummary = stateQuery.data.summary || 'System online. Awaiting data.'
    const latestRisk =
      advisories[0]?.risk ??
      (logsQuery.data?.[0]?.summary?.toLowerCase()?.includes('critical') ? 95 : logsQuery.data?.[0] ? 65 : 30)
    setFromState({ summary: latestSummary, risk: latestRisk })
  }, [stateQuery.data, logsQuery.data, setFromState])

  const latestAudio = useMemo(() => logsQuery.data?.find((x) => x.audio_url)?.audio_url, [logsQuery.data])
  const tickerText = useMemo(() => {
    const text = (logsQuery.data || [])
      .slice(0, 12)
      .map((item) => item.summary?.trim())
      .filter(Boolean)
      .join(' • ')
    return text || 'Awaiting advisories'
  }, [logsQuery.data])

  return (
    <main className="mx-auto grid h-full w-full max-w-7xl gap-4 px-4 py-4 lg:grid-cols-[1.8fr_1fr]">
      <section className="space-y-4">
        <div className="panel rounded-xl p-3">
          <MapView telemetry={telemetryQuery.data || []} nodes={telemetryQuery.data || []} />
        </div>
        <AdvisoryCard summary={summary} risk={risk} actions={['Monitor transit corridors', 'Stage EMS', 'Broadcast advisory']} />
        <div className="panel overflow-hidden rounded-xl py-2">
          <div className="ticker-track whitespace-nowrap px-3 text-sm text-zinc-300">
            <span>{tickerText} &#8226; </span>
            <span>{tickerText} &#8226; </span>
          </div>
        </div>
      </section>

      <aside className="space-y-4">
        <RiskGauge value={risk} />
        <SystemMoodOrb risk={risk} />
        <section className="panel rounded-xl p-4">
          <h3 className="mb-2 text-sm uppercase tracking-widest text-zinc-400">Voice Advisory</h3>
          <AudioPlayer src={latestAudio} />
        </section>
        <section className="panel rounded-xl p-4">
          <h3 className="mb-2 text-sm uppercase tracking-widest text-zinc-400">System Health</h3>
          <p className="text-xs text-zinc-500">Status: {stateQuery.data?.status || 'operational'}</p>
          <p className="text-xs text-zinc-500">Alerts committed: {stateQuery.data?.alerts ?? 0}</p>
          {stateQuery.error && <p className="mt-2 text-xs text-red-400">{stateQuery.error}</p>}
        </section>
      </aside>
    </main>
  )
}
