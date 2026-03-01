import { useEffect, useMemo, useRef, useState } from 'react'
import AdvisoryCard from '../components/AdvisoryCard'
import AudioPlayer from '../components/AudioPlayer'
import MapView from '../components/MapView'
import RiskGauge from '../components/RiskGauge'
import SystemMoodOrb from '../components/SystemMoodOrb'
import {
  createChatThread,
  deleteChatThread,
  getAIStatus,
  getChatMessages,
  getLogs,
  getPublicFeeds,
  getRiskSources,
  getSessionCity,
  getStateByCity,
  getTelemetry,
  listChatThreads,
  refreshAdvisory,
  searchCities,
  sendChatMessage,
  setSessionCity,
} from '../context/appApi'
import { getErrorMessage } from '../context/apiClient'
import { useAppState } from '../context/useAppState'
import { useFetch } from '../hooks/useFetch'

function riskToScore(riskLabel, fallback = 35) {
  const risk = String(riskLabel || '').toLowerCase()
  if (risk === 'critical') return 95
  if (risk === 'high') return 82
  if (risk === 'medium') return 58
  if (risk === 'low') return 28
  return fallback
}

function normalizeAdvisoryText(raw) {
  const text = String(raw || '').trim()
  if (!text) return ''
  const maybeJSON = text.startsWith('{') || text.startsWith('"')
  if (maybeJSON && text.includes('"summary"')) {
    try {
      let parsed = JSON.parse(text)
      if (typeof parsed === 'string') parsed = JSON.parse(parsed)
      if (parsed && typeof parsed.summary === 'string') return parsed.summary.trim()
    } catch {
      // Fall back to plain text normalization.
    }
  }
  const summaryMatch = text.match(/["']summary["']\s*:\s*["']([^"']+)["']/i)
  const extracted = summaryMatch?.[1] || text
  return extracted
    .replace(/[{}\[\]"]/g, ' ')
    .replace(/\s*,\s*/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function compactTickerItem(raw, maxLen = 140) {
  const normalized = normalizeAdvisoryText(raw)
  if (!normalized) return ''
  if (normalized.length <= maxLen) return normalized
  return `${normalized.slice(0, maxLen - 1)}…`
}

export default function Dashboard() {
  const [activeCity, setActiveCity] = useState(null)
  const [cityQuery, setCityQuery] = useState('')
  const [cityResults, setCityResults] = useState([])
  const [threads, setThreads] = useState([])
  const [activeThread, setActiveThread] = useState(null)
  const [messages, setMessages] = useState([])
  const [messageInput, setMessageInput] = useState('')
  const [chatBusy, setChatBusy] = useState(false)
  const [chatError, setChatError] = useState('')
  const [cityError, setCityError] = useState('')
  const [voiceBusy, setVoiceBusy] = useState(false)
  const lastAutoVoiceSrcRef = useRef('')

  const summary = useAppState((s) => s.summary)
  const risk = useAppState((s) => s.risk)
  const setFromState = useAppState((s) => s.setFromState)

  const stateQuery = useFetch(() => getStateByCity(activeCity), 10_000, [activeCity?.city_id])
  const telemetryQuery = useFetch(() => getTelemetry(activeCity), 12_000, [activeCity?.city_id])
  const logsQuery = useFetch(() => getLogs(activeCity), 20_000, [activeCity?.city_id])
  const feedsQuery = useFetch(() => getPublicFeeds(activeCity), 20_000, [activeCity?.city_id])
  const aiStatusQuery = useFetch(getAIStatus, 30_000, [])
  const riskSignalsQuery = useFetch(() => getRiskSources(activeCity), 45_000, [activeCity?.city_id])

  useEffect(() => {
    let mounted = true
    let initialCity = null
    ;(async () => {
      try {
        const city = await getSessionCity()
        initialCity = city
        if (mounted) setActiveCity(city)
      } catch {
        // ignore
      }
      try {
        const list = await listChatThreads(initialCity)
        if (!mounted) return
        setThreads(list)
        if (list[0]) setActiveThread(list[0])
      } catch {
        // ignore
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  useEffect(() => {
    if (!activeCity?.city_id) return
    let mounted = true
    ;(async () => {
      try {
        const list = await listChatThreads(activeCity)
        if (!mounted) return
        setThreads(list)
        setActiveThread(list[0] || null)
        setMessages([])
      } catch (err) {
        if (mounted) setChatError(getErrorMessage(err))
      }
    })()
    return () => {
      mounted = false
    }
  }, [activeCity?.city_id])

  const latestAudio = useMemo(() => logsQuery.data?.find((x) => x.audio_url)?.audio_url, [logsQuery.data])
  const latestDecision = useMemo(() => (logsQuery.data || [])[0] || null, [logsQuery.data])
  const riskSignals = riskSignalsQuery.data?.signals?.risk_signals || null
  const riskComponents = riskSignals?.components || {}
  const liveRisk = useMemo(() => {
    const score = Number(latestDecision?.risk_score)
    if (Number.isFinite(score) && score >= 0) return Math.max(0, Math.min(100, score))
    return riskToScore(latestDecision?.risk, risk)
  }, [latestDecision?.risk_score, latestDecision?.risk, risk])
  const advisorySummary = useMemo(() => normalizeAdvisoryText(latestDecision?.summary || summary), [latestDecision?.summary, summary])
  const tickerText = useMemo(() => {
    const text = (logsQuery.data || [])
      .slice(0, 8)
      .map((item) => compactTickerItem(item.summary))
      .filter(Boolean)
      .join(' • ')
    return text || 'Awaiting advisories'
  }, [logsQuery.data])

  useEffect(() => {
    if (!stateQuery.data) return
    const latestSummary = stateQuery.data.summary || 'System online. Awaiting data.'
    setFromState({ summary: latestSummary, risk: liveRisk })
  }, [stateQuery.data, liveRisk, setFromState])

  useEffect(() => {
    if (!latestAudio || voiceBusy) return
    if (lastAutoVoiceSrcRef.current === latestAudio) return
    lastAutoVoiceSrcRef.current = latestAudio
    const probe = new Audio(latestAudio)
    probe.preload = 'metadata'
    probe.onloadedmetadata = async () => {
      if (probe.duration > 0 && probe.duration < 3 && !voiceBusy) {
        try {
          setVoiceBusy(true)
          await refreshAdvisory(activeCity, true)
          await logsQuery.refresh()
        } finally {
          setVoiceBusy(false)
        }
      }
    }
  }, [latestAudio, activeCity?.city_id, logsQuery, voiceBusy])

  useEffect(() => {
    if (!activeThread?.id) {
      setMessages([])
      return
    }
    let mounted = true
    ;(async () => {
      try {
        const list = await getChatMessages(activeThread.id)
        if (mounted) setMessages(list)
      } catch {
        // ignore
      }
    })()
    return () => {
      mounted = false
    }
  }, [activeThread?.id])

  async function onSearchCity() {
    if (cityQuery.trim().length < 2) return
    setCityError('')
    try {
      const list = await searchCities(cityQuery.trim())
      setCityResults(list)
    } catch (err) {
      setCityError(getErrorMessage(err))
    }
  }

  async function onSelectCity(city) {
    setCityError('')
    try {
      const saved = await setSessionCity(city)
      setActiveCity(saved)
      setCityResults([])
      await Promise.all([stateQuery.refresh(), telemetryQuery.refresh(), logsQuery.refresh(), feedsQuery.refresh()])
      setTimeout(() => {
        void Promise.all([stateQuery.refresh(), telemetryQuery.refresh(), logsQuery.refresh(), feedsQuery.refresh()])
      }, 1800)
    } catch (err) {
      setCityError(getErrorMessage(err))
    }
  }

  async function onCreateThread() {
    setChatError('')
    const thread = await createChatThread(`${activeCity?.city_name || 'City'} Ops`, activeCity)
    const list = [thread, ...threads]
    setThreads(list)
    setActiveThread(thread)
  }

  async function onSend() {
    const content = messageInput.trim()
    if (!content || chatBusy) return
    setChatBusy(true)
    setChatError('')
    setMessageInput('')
    try {
      let thread = activeThread
      if (!thread?.id) {
        thread = await createChatThread(`${activeCity?.city_name || 'City'} Ops`, activeCity)
        setThreads((prev) => [thread, ...prev])
        setActiveThread(thread)
      }
      const resp = await sendChatMessage(thread.id, content)
      setMessages((prev) => [...prev, resp.user, resp.assistant])
      const refreshed = await listChatThreads(activeCity)
      setThreads(refreshed)
    } catch (err) {
      setChatError(getErrorMessage(err))
    } finally {
      setChatBusy(false)
    }
  }

  async function onDeleteThread(threadId) {
    if (!threadId) return
    setChatError('')
    try {
      await deleteChatThread(threadId)
      const nextThreads = threads.filter((t) => t.id !== threadId)
      setThreads(nextThreads)
      if (activeThread?.id === threadId) {
        setActiveThread(nextThreads[0] || null)
        setMessages([])
      }
    } catch (err) {
      setChatError(getErrorMessage(err))
    }
  }

  async function onRefreshVoice(forceAudio = true) {
    try {
      setVoiceBusy(true)
      await refreshAdvisory(activeCity, forceAudio)
      await Promise.all([stateQuery.refresh(), logsQuery.refresh()])
    } catch (err) {
      setChatError(getErrorMessage(err))
    } finally {
      setVoiceBusy(false)
    }
  }

  return (
    <main className="mx-auto grid h-full w-full max-w-7xl gap-4 px-4 py-4 lg:grid-cols-[1.8fr_1fr]">
      <section className="space-y-4">
        <div className="panel rounded-xl p-3">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="text-xs uppercase tracking-widest text-zinc-400">Active city</span>
            <span className="rounded border border-crimson/40 px-2 py-1 text-xs text-zinc-200">
              {activeCity?.city_name ? `${activeCity.city_name} (${activeCity.country_code || 'N/A'})` : 'Default City'}
            </span>
          </div>
          <div className="flex flex-wrap gap-2">
            <input
              value={cityQuery}
              onChange={(e) => setCityQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onSearchCity()
              }}
              placeholder="Search city (e.g. Tokyo, Lagos, Sao Paulo)"
              className="min-w-64 flex-1 rounded border border-zinc-700 bg-black/50 px-3 py-2 text-sm text-zinc-100"
            />
            <button onClick={onSearchCity} className="rounded border border-zinc-700 px-3 py-2 text-sm text-zinc-200">
              Search
            </button>
          </div>
          {cityResults.length > 0 && (
            <div className="mt-2 max-h-44 overflow-auto rounded border border-zinc-800 bg-black/60">
              {cityResults.map((city) => (
                <button
                  key={city.city_id}
                  onClick={() => onSelectCity(city)}
                  className="block w-full border-b border-zinc-900 px-3 py-2 text-left text-sm text-zinc-200 hover:bg-zinc-900"
                >
                  {city.city_name}, {city.country} {city.region ? `· ${city.region}` : ''}
                </button>
              ))}
            </div>
          )}
          {cityError && <p className="mt-2 text-xs text-red-400">{cityError}</p>}
        </div>
        <div className="panel rounded-xl p-3">
          <MapView telemetry={telemetryQuery.data || []} nodes={telemetryQuery.data || []} city={activeCity} />
        </div>
        <AdvisoryCard
          summary={advisorySummary}
          risk={liveRisk}
          actions={latestDecision?.actions || ['Monitor transit corridors', 'Stage EMS', 'Broadcast advisory']}
          forecast={latestDecision?.forecast || ''}
          confidence={typeof latestDecision?.confidence === 'number' ? latestDecision.confidence : null}
        />
        <div className="panel rounded-xl py-2">
          <div className="ticker-viewport px-3">
            <div className="ticker-track text-sm text-zinc-300">
            <span>{tickerText} &#8226; </span>
            <span>{tickerText} &#8226; </span>
            </div>
          </div>
        </div>
        <section className="panel rounded-xl p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm uppercase tracking-widest text-zinc-400">Operator AI Chat</h3>
            <div className="flex items-center gap-2">
              <span
                className={`rounded border px-2 py-1 text-[10px] uppercase tracking-widest ${
                  aiStatusQuery.data?.status === 'cloud'
                    ? 'border-emerald-600/50 bg-emerald-900/30 text-emerald-200'
                    : 'border-amber-600/50 bg-amber-900/30 text-amber-200'
                }`}
                title={aiStatusQuery.data?.last_error || 'No recent AI error recorded.'}
              >
                AI {aiStatusQuery.data?.status === 'cloud' ? 'Cloud' : 'Local'}
              </span>
              <button onClick={onCreateThread} className="rounded border border-zinc-700 px-2 py-1 text-xs text-zinc-200">
                New Thread
              </button>
            </div>
          </div>
          <div className="mb-2 flex gap-2 overflow-auto">
            {threads.map((thread) => (
              <div
                key={thread.id}
                className={`flex items-center gap-1 rounded px-2 py-1 text-xs ${
                  activeThread?.id === thread.id ? 'bg-crimson/90 text-white' : 'border border-zinc-700 text-zinc-300'
                }`}
              >
                <button onClick={() => setActiveThread(thread)} className="text-left">
                  {thread.title}
                </button>
                <button
                  onClick={() => onDeleteThread(thread.id)}
                  className="rounded px-1 text-[10px] text-zinc-200/80 hover:bg-black/20 hover:text-white"
                  title="Delete thread"
                >
                  x
                </button>
              </div>
            ))}
          </div>
          <div className="mb-2 h-44 overflow-auto rounded border border-zinc-800 bg-black/40 p-2 text-sm">
            {messages.length === 0 ? (
              <p className="text-zinc-500">Start a thread and ask about risks, mitigation, and city operations.</p>
            ) : (
              messages.map((m) => (
                <div key={m.id} className="mb-2">
                  <p className="text-xs uppercase tracking-wide text-zinc-500">{m.role}</p>
                  <p className="whitespace-pre-wrap text-zinc-200">{m.content}</p>
                </div>
              ))
            )}
          </div>
          {chatError && <p className="mb-2 text-xs text-red-400">{chatError}</p>}
          {aiStatusQuery.data?.status !== 'cloud' && (
            <p className="mb-2 text-xs text-amber-300">
              Running in local fallback mode.
              {aiStatusQuery.data?.last_error ? ` Last backend AI error: ${aiStatusQuery.data.last_error}` : ''}
            </p>
          )}
          <div className="flex gap-2">
            <input
              value={messageInput}
              onChange={(e) => setMessageInput(e.target.value)}
              placeholder="Ask AI about this city's situation..."
              className="flex-1 rounded border border-zinc-700 bg-black/60 px-3 py-2 text-sm text-zinc-100"
              onKeyDown={(e) => {
                if (e.key === 'Enter') onSend()
              }}
            />
            <button
              onClick={onSend}
              disabled={chatBusy}
              className="rounded bg-crimson px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
            >
              {chatBusy ? '...' : 'Send'}
            </button>
          </div>
        </section>
        <section className="panel rounded-xl p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm uppercase tracking-widest text-zinc-400">Live City Feeds</h3>
            <button onClick={feedsQuery.refresh} className="rounded border border-zinc-700 px-2 py-1 text-xs text-zinc-200">
              Refresh
            </button>
          </div>
          {feedsQuery.error && <p className="mb-2 text-xs text-red-400">{feedsQuery.error}</p>}
          <div className="max-h-64 space-y-2 overflow-auto pr-1">
            {(feedsQuery.data?.feeds || []).length === 0 ? (
              <p className="text-xs text-zinc-500">No public feeds available for this city yet.</p>
            ) : (
              (feedsQuery.data?.feeds || []).map((feed) => (
                <div key={feed.id} className="rounded border border-zinc-800 bg-black/30 p-2">
                  <p className="text-xs uppercase tracking-wide text-zinc-500">
                    {feed.kind} · {feed.source}
                  </p>
                  <p className="text-sm text-zinc-200">{feed.title}</p>
                  {feed.value && <p className="mt-1 text-xs text-zinc-300">{feed.value}</p>}
                  {feed.links?.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-2">
                      {feed.links.slice(0, 3).map((link) => (
                        <a
                          key={link.url}
                          href={link.url}
                          target="_blank"
                          rel="noreferrer"
                          className="rounded border border-zinc-700 px-2 py-1 text-xs text-crimson hover:bg-zinc-900"
                        >
                          {link.label}
                        </a>
                      ))}
                    </div>
                  )}
                  {feed.items?.length > 0 && (
                    <ul className="mt-2 list-disc space-y-1 pl-4 text-xs text-zinc-300">
                      {feed.items.slice(0, 3).map((item, idx) => (
                        <li key={`${feed.id}-${idx}`}>{item}</li>
                      ))}
                    </ul>
                  )}
                </div>
              ))
            )}
          </div>
        </section>
      </section>

      <aside className="space-y-4">
        <RiskGauge value={liveRisk} />
        <SystemMoodOrb risk={liveRisk} />
        <section className="panel rounded-xl p-4">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm uppercase tracking-widest text-zinc-400">Voice Advisory</h3>
            <button
              onClick={() => onRefreshVoice(true)}
              disabled={voiceBusy}
              className="rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-200 disabled:opacity-50"
            >
              {voiceBusy ? 'Refreshing...' : 'Refresh Voice'}
            </button>
          </div>
          <AudioPlayer src={latestAudio} />
        </section>
        <section className="panel rounded-xl p-4">
          <h3 className="mb-2 text-sm uppercase tracking-widest text-zinc-400">System Health</h3>
          <p className="text-xs text-zinc-500">Status: {stateQuery.data?.status || 'operational'}</p>
          <p className="text-xs text-zinc-500">Alerts committed: {stateQuery.data?.alerts ?? 0}</p>
          {stateQuery.error && <p className="mt-2 text-xs text-red-400">{stateQuery.error}</p>}
        </section>
        <section className="panel rounded-xl p-4">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm uppercase tracking-widest text-zinc-400">Risk Source Matrix</h3>
            <button onClick={riskSignalsQuery.refresh} className="rounded border border-zinc-700 px-2 py-1 text-[11px] text-zinc-200">
              Refresh
            </button>
          </div>
          {riskSignalsQuery.error && <p className="text-xs text-zinc-500">Signals are warming up for this city.</p>}
          {!riskSignalsQuery.error && !riskSignals && <p className="text-xs text-zinc-500">No matrix available yet.</p>}
          {riskSignals && (
            <div className="space-y-2 text-xs">
              <p className="text-zinc-300">Composite score: {riskSignals.score ?? riskSignalsQuery.data?.signals?.risk_signal_score ?? '--'}/100</p>
              <p className="text-zinc-500">Travel advisory: {riskSignals.travel_level || 'n/a'}</p>
              <p className="text-zinc-500">Disaster events (30d): {riskSignals.disaster_events_30d ?? 0}</p>
              <p className="text-zinc-500">Crime mentions (72h): {riskSignals.crime_mentions_72h ?? 0}</p>
              <p className="text-zinc-500">Conflict mentions (72h): {riskSignals.conflict_mentions_72h ?? 0}</p>
              <p className="text-zinc-500">
                Component scores: T {riskComponents.travel ?? 0} · D {riskComponents.disaster ?? 0} · C {riskComponents.crime ?? 0} · X {riskComponents.conflict ?? 0}
              </p>
            </div>
          )}
        </section>
      </aside>
    </main>
  )
}
