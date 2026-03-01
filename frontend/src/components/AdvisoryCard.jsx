import { motion } from 'framer-motion'

function normalizeSummary(raw) {
  const text = String(raw || '').trim()
  if (!text) return ''
  if ((text.startsWith('{') || text.startsWith('"')) && text.includes('"summary"')) {
    try {
      let parsed = JSON.parse(text)
      if (typeof parsed === 'string') parsed = JSON.parse(parsed)
      if (parsed && typeof parsed.summary === 'string') return parsed.summary.trim()
    } catch {
      // Keep raw fallback.
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

export default function AdvisoryCard({
  summary,
  risk = 0,
  actions = [],
  forecast = '',
  confidence = null,
  onRetryGeneration,
  onReviewAIOutput,
  onPredictiveAnalysis,
  busy = false,
}) {
  const frame = risk > 85 ? 'border-red-500/60 crisis' : risk > 60 ? 'border-orange-400/50' : 'border-zinc-800'
  const actionList = Array.isArray(actions)
    ? actions
    : actions && typeof actions === 'object'
      ? Object.values(actions).filter(Boolean)
      : []
  const cleanSummary = normalizeSummary(summary)
  const cleanForecast = normalizeSummary(forecast)
  const hasRetryAction = actionList.some((a) => /retry/i.test(String(a)))
  const hasReviewAction = actionList.some((a) => /review/i.test(String(a)))

  return (
    <motion.section
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      className={`panel rounded-xl border p-4 ${frame}`}
    >
      <h2 className="mb-2 text-sm uppercase tracking-widest text-zinc-400">Latest Advisory</h2>
      <p className="text-sm leading-relaxed text-zinc-100">{cleanSummary || 'Awaiting AI advisories.'}</p>
      <div className="mt-3 flex items-center gap-2 text-xs">
        <span className="rounded border border-crimson/60 px-2 py-1 text-zinc-200">Risk {Math.round(risk)}</span>
        {typeof confidence === 'number' && confidence > 0 && (
          <span className="rounded border border-zinc-700 px-2 py-1 text-zinc-300">Confidence {confidence}%</span>
        )}
        <span className="text-zinc-500">{new Date().toLocaleTimeString()}</span>
      </div>
      {cleanForecast && <p className="mt-2 text-xs text-zinc-400">Forecast: {cleanForecast}</p>}
      {actionList.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-2">
          {actionList.map((action, index) => {
            const text = String(action)
            if (/retry/i.test(text) && typeof onRetryGeneration === 'function') {
              return (
                <button
                  key={`${text}-${index}`}
                  onClick={onRetryGeneration}
                  disabled={busy}
                  className="rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-100 transition hover:bg-zinc-700 disabled:opacity-60"
                >
                  {busy ? 'Retrying...' : text}
                </button>
              )
            }
            if (/review/i.test(text) && typeof onReviewAIOutput === 'function') {
              return (
                <button
                  key={`${text}-${index}`}
                  onClick={onReviewAIOutput}
                  className="rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-100 transition hover:bg-zinc-700"
                >
                  {text}
                </button>
              )
            }
            return (
              <span key={`${text}-${index}`} className="rounded bg-zinc-900 px-2 py-1 text-xs text-zinc-300">
                {text}
              </span>
            )
          })}
          {!hasRetryAction && typeof onRetryGeneration === 'function' && (
            <button
              onClick={onRetryGeneration}
              disabled={busy}
              className="rounded border border-zinc-700 px-2 py-1 text-xs text-zinc-100 transition hover:bg-zinc-900 disabled:opacity-60"
            >
              {busy ? 'Retrying...' : 'Retry generation'}
            </button>
          )}
          {typeof onPredictiveAnalysis === 'function' && (
            <button onClick={onPredictiveAnalysis} className="rounded border border-crimson/50 px-2 py-1 text-xs text-zinc-100 transition hover:bg-crimson/20">
              Predictive Analysis
            </button>
          )}
          {!hasReviewAction && typeof onReviewAIOutput === 'function' && (
            <button onClick={onReviewAIOutput} className="rounded border border-zinc-700 px-2 py-1 text-xs text-zinc-100 transition hover:bg-zinc-900">
              Review AI output
            </button>
          )}
        </div>
      )}
    </motion.section>
  )
}
