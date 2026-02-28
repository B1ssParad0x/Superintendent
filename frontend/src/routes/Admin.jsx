import { useState } from 'react'
import AudioPlayer from '../components/AudioPlayer'
import { commitDecision, triggerReason } from '../context/appApi'
import { getErrorMessage } from '../context/apiClient'

export default function Admin() {
  const [reasonResult, setReasonResult] = useState(null)
  const [commitResult, setCommitResult] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function onReason() {
    setLoading(true)
    setError('')
    setCommitResult(null)
    try {
      const result = await triggerReason()
      setReasonResult(result)
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  async function onCommit() {
    if (!reasonResult?.summary) return
    setLoading(true)
    setError('')
    try {
      const result = await commitDecision(reasonResult.summary, reasonResult.audio_url)
      setCommitResult(result)
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="mx-auto max-w-5xl space-y-4 px-4 py-4">
      <section className="panel rounded-xl p-4">
        <h2 className="mb-4 font-display text-lg">Administrative Console</h2>
        <div className="flex flex-wrap gap-3">
          <button disabled={loading} className="rounded bg-crimson px-4 py-2 text-sm font-medium text-white disabled:opacity-60" onClick={onReason}>
            Trigger Reasoning
          </button>
          <button
            disabled={loading || !reasonResult?.summary}
            className="rounded border border-crimson/60 px-4 py-2 text-sm text-zinc-100 disabled:opacity-50"
            onClick={onCommit}
          >
            Commit Decision
          </button>
        </div>
        {error && <p className="mt-3 text-sm text-red-400">{error}</p>}
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
    </main>
  )
}
