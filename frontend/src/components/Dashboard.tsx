import { useState, useEffect, useCallback } from 'react'
import Map from './Map'
import { getState, getLogs, triggerReason, commitDecision, getErrorMessage, Decision } from '../api'

const mapboxToken = import.meta.env.VITE_MAPBOX_TOKEN || ''

interface DashboardProps {
  getToken: () => Promise<string>
}

export default function Dashboard({ getToken }: DashboardProps) {
  const [state, setState] = useState<{ status: string; alerts: number; summary?: string } | null>(null)
  const [decisions, setDecisions] = useState<Decision[]>([])
  const [reasonResult, setReasonResult] = useState<{
    summary?: string
    audio_url?: string
    audio_text?: string
  } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refreshState = useCallback(async () => {
    try {
      const s = await getState()
      setState({ status: s.status, alerts: s.alerts, summary: s.summary })
      setError(null)
    } catch (e) {
      setError(getErrorMessage(e))
    }
  }, [])

  const refreshLogs = useCallback(async () => {
    try {
      const token = await getToken()
      const list = await getLogs(token)
      setDecisions(list || [])
      setError(null)
    } catch (e) {
      setError(getErrorMessage(e))
    }
  }, [getToken])

  useEffect(() => {
    refreshState()
    const t = setInterval(refreshState, 30000)
    return () => clearInterval(t)
  }, [refreshState])

  useEffect(() => {
    refreshLogs()
  }, [refreshLogs])

  const onReason = async () => {
    setLoading(true)
    setError(null)
    setReasonResult(null)
    try {
      const token = await getToken()
      const res = await triggerReason(token)
      setReasonResult({
        summary: res.summary,
        audio_url: res.audio_url,
        audio_text: res.audio_text,
      })
    } catch (e: unknown) {
      setError(getErrorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  const onCommit = async () => {
    if (!reasonResult?.summary) return
    setLoading(true)
    setError(null)
    try {
      const token = await getToken()
      await commitDecision(token, reasonResult.summary, reasonResult.audio_url)
      setReasonResult(null)
      refreshLogs()
    } catch (e: unknown) {
      setError(getErrorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '1fr 340px', minHeight: 'calc(100vh - 52px)' }}>
      <div style={{ borderRight: '1px solid var(--border)', display: 'flex', flexDirection: 'column' }}>
        {mapboxToken ? (
          <div style={{ flex: 1, minHeight: 300 }}>
            <Map token={mapboxToken} />
          </div>
        ) : (
          <div style={{ flex: 1, padding: '2rem', color: 'var(--text-muted)', background: 'var(--surface)' }}>
            <p>Add VITE_MAPBOX_TOKEN for map view</p>
          </div>
        )}
        <div style={{ padding: '1rem 1.5rem', borderTop: '1px solid var(--border)' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', flexWrap: 'wrap' }}>
              <span style={{ color: 'var(--text-muted)' }}>Status: {state?.status ?? '—'}</span>
              <span style={{ color: 'var(--text-muted)' }}>Alerts: {state?.alerts ?? 0}</span>
            </div>
            {state?.summary && (
              <p style={{ margin: 0, fontSize: '0.9rem', color: 'var(--text-muted)' }}>{state.summary}</p>
            )}
          </div>
          <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            <button
              onClick={onReason}
              disabled={loading}
              style={{
                padding: '0.4rem 1rem',
                background: 'var(--accent)',
                color: 'var(--bg)',
                border: 'none',
                borderRadius: '4px',
                cursor: loading ? 'not-allowed' : 'pointer',
                fontWeight: 600,
              }}
            >
              {loading ? '…' : 'Trigger Reason'}
            </button>
            {reasonResult && (
              <button
                onClick={onCommit}
                disabled={loading}
                style={{
                  padding: '0.4rem 1rem',
                  background: 'var(--success)',
                  color: 'var(--bg)',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: loading ? 'not-allowed' : 'pointer',
                  fontWeight: 600,
                }}
              >
                Commit to Solana
              </button>
            )}
          </div>
          {error && <p style={{ color: 'var(--danger)', margin: '0.5rem 0 0', fontSize: '0.9rem' }}>{error}</p>}
          {reasonResult && (
            <div style={{ marginTop: '1rem', padding: '1rem', background: 'var(--surface)', borderRadius: '6px' }}>
              <p style={{ margin: '0 0 0.5rem', fontWeight: 600 }}>AI Summary</p>
              <p style={{ margin: 0, fontSize: '0.9rem' }}>{reasonResult.summary}</p>
              {reasonResult.audio_url && (
                <audio controls src={reasonResult.audio_url} style={{ marginTop: '0.5rem', width: '100%' }} />
              )}
            </div>
          )}
        </div>
      </div>
      <div style={{ overflow: 'auto', padding: '1rem' }}>
        <h3 style={{ margin: '0 0 1rem', fontSize: '1rem' }}>Solana Audit Log</h3>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
          {decisions.length === 0 && <p style={{ color: 'var(--text-muted)', fontSize: '0.9rem' }}>No decisions yet</p>}
          {decisions.map((d) => (
            <div
              key={d.solana_tx || d.hash || d.when}
              style={{
                padding: '0.75rem',
                background: 'var(--surface)',
                borderRadius: '6px',
                border: '1px solid var(--border)',
              }}
            >
              {d.when && (
                <p style={{ margin: '0 0 0.25rem', fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                  {new Date(d.when).toLocaleString()}
                </p>
              )}
              <p style={{ margin: '0 0 0.25rem', fontSize: '0.85rem' }}>{d.summary}</p>
              <p style={{ margin: 0, fontSize: '0.75rem', color: 'var(--text-muted)', fontFamily: 'monospace' }}>
                TX:{' '}
                {d.solana_tx && !d.solana_tx.startsWith('dev-stub') ? (
                  <a
                    href={`https://explorer.solana.com/tx/${d.solana_tx}?cluster=devnet`}
                    target="_blank"
                    rel="noopener noreferrer"
                    style={{ color: 'var(--accent)', textDecoration: 'none' }}
                  >
                    {d.solana_tx.slice(0, 12)}…
                  </a>
                ) : (
                  d.solana_tx
                )}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
