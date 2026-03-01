import { useEffect, useState } from 'react'
import AudioPlayer from '../components/AudioPlayer'
import { commitDecision, commitLatestDecision, getSessionCity, triggerReason, verifyAuditTrail } from '../context/appApi'
import { getErrorMessage } from '../context/apiClient'
import { useAppAuth } from '../context/AuthProvider'

export default function Admin() {
  const { isAdmin, roles } = useAppAuth()
  const [activeCity, setActiveCity] = useState(null)
  const [reasonResult, setReasonResult] = useState(null)
  const [commitResult, setCommitResult] = useState(null)
  const [auditResult, setAuditResult] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [lastAction, setLastAction] = useState('')

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const city = await getSessionCity()
        if (mounted) setActiveCity(city)
      } catch {
        // Default city fallback.
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  async function onReason(focus = '') {
    setLoading(true)
    setError('')
    setCommitResult(null)
    setLastAction(focus === 'predictive' ? 'Running predictive analysis...' : 'Triggering reasoning...')
    try {
      const result = await triggerReason(activeCity, focus ? { focus } : undefined)
      setReasonResult(result)
      setLastAction('AI reasoning complete.')
    } catch (err) {
      setError(getErrorMessage(err))
      setLastAction('')
    } finally {
      setLoading(false)
    }
  }

  async function onCommit() {
    if (!reasonResult?.summary) return
    setLoading(true)
    setError('')
    setLastAction('Committing advisory to Solana...')
    try {
      const result = await commitDecision(reasonResult.summary, reasonResult.audio_url, activeCity)
      setCommitResult(result)
      setLastAction('Commit completed.')
    } catch (err) {
      setError(getErrorMessage(err))
      setLastAction('')
    } finally {
      setLoading(false)
    }
  }

  async function onCommitLatest() {
    setLoading(true)
    setError('')
    setLastAction('Committing latest pending advisory...')
    try {
      const result = await commitLatestDecision()
      setCommitResult(result)
      setLastAction('Latest advisory committed.')
    } catch (err) {
      setError(getErrorMessage(err))
      setLastAction('')
    } finally {
      setLoading(false)
    }
  }

  async function onVerifyAudit() {
    setLoading(true)
    setError('')
    setLastAction('Verifying audit trail...')
    try {
      const result = await verifyAuditTrail()
      setAuditResult(result)
      setLastAction('Audit verification complete.')
    } catch (err) {
      setError(getErrorMessage(err))
      setLastAction('')
    } finally {
      setLoading(false)
    }
  }

  async function onDemoRun() {
    setLoading(true)
    setError('')
    setLastAction('Running demo flow: predictive analysis -> commit latest -> verify audit...')
    try {
      const reason = await triggerReason(activeCity, { focus: 'predictive' })
      setReasonResult(reason)
      const commit = await commitLatestDecision()
      setCommitResult(commit)
      const audit = await verifyAuditTrail()
      setAuditResult(audit)
      setLastAction('Demo flow complete.')
    } catch (err) {
      setError(getErrorMessage(err))
      setLastAction('')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="mx-auto max-w-5xl space-y-4 px-4 py-4">
      <section className="panel rounded-xl p-4">
        <h2 className="mb-4 font-display text-lg">Administrative Console</h2>
        {!isAdmin && (
          <p className="mb-3 rounded border border-amber-500/40 bg-amber-900/20 px-3 py-2 text-xs text-amber-200">
            You are signed in but your token is missing the `admin` role claim. Current roles: {roles?.join(', ') || 'none'}.
            Admin actions will return 403 until Auth0 role mapping includes `https://superintendent/roles: ["admin"]`.
          </p>
        )}
        <p className="mb-3 text-xs text-zinc-400">
          Active city: {activeCity?.city_name || 'Default City'} {activeCity?.country_code ? `(${activeCity.country_code})` : ''}
        </p>
        <div className="flex flex-wrap gap-3">
          <button disabled={loading} className="rounded bg-crimson px-4 py-2 text-sm font-medium text-white disabled:opacity-60" onClick={() => onReason('')}>
            Trigger Reasoning
          </button>
          <button
            disabled={loading}
            className="rounded border border-crimson/60 bg-crimson/10 px-4 py-2 text-sm text-zinc-100 disabled:opacity-60"
            onClick={() => onReason('predictive')}
          >
            Predictive Analysis
          </button>
          <button
            disabled={loading || !reasonResult?.summary}
            className="rounded border border-crimson/60 px-4 py-2 text-sm text-zinc-100 disabled:opacity-50"
            onClick={onCommit}
          >
            Commit Decision
          </button>
          <button disabled={loading} className="rounded border border-zinc-700 px-4 py-2 text-sm text-zinc-200 disabled:opacity-50" onClick={onCommitLatest}>
            Commit Latest Pending
          </button>
          <button disabled={loading} className="rounded border border-zinc-700 px-4 py-2 text-sm text-zinc-200 disabled:opacity-50" onClick={onVerifyAudit}>
            Verify Audit Trail
          </button>
          <button disabled={loading} className="rounded border border-emerald-600/50 bg-emerald-900/20 px-4 py-2 text-sm text-emerald-200 disabled:opacity-50" onClick={onDemoRun}>
            Run Demo Flow
          </button>
        </div>
        {error && <p className="mt-3 text-sm text-red-400">{error}</p>}
        {lastAction && <p className="mt-2 text-xs text-zinc-400">{lastAction}</p>}
      </section>

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">AI Output JSON</h3>
        <pre className="max-h-96 overflow-auto rounded bg-black/50 p-3 text-xs text-zinc-300">
          {JSON.stringify(reasonResult || { message: 'No reasoning triggered yet.' }, null, 2)}
        </pre>
      </section>

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">Audio Preview</h3>
        <AudioPlayer src={reasonResult?.audio_url} />
      </section>

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">Solana Confirmation</h3>
        {!commitResult ? (
          <p className="text-sm text-zinc-500">No commit performed yet.</p>
        ) : (
          <div className="space-y-2 text-sm text-zinc-300">
            <p>Hash: {commitResult.hash}</p>
            <a className="text-crimson hover:underline" href={`https://explorer.solana.com/tx/${commitResult.tx}?cluster=devnet`} target="_blank" rel="noreferrer">
              View transaction {commitResult.tx}
            </a>
          </div>
        )}
      </section>

      <section className="panel rounded-xl p-4">
        <h3 className="mb-2 text-sm uppercase tracking-wider text-zinc-400">Audit Verification</h3>
        <pre className="max-h-72 overflow-auto rounded bg-black/50 p-3 text-xs text-zinc-300">
          {JSON.stringify(auditResult || { message: 'Run "Verify Audit Trail" to validate Solana and hash integrity.' }, null, 2)}
        </pre>
      </section>
    </main>
  )
}
