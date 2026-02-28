import { motion } from 'framer-motion'

export default function AdvisoryCard({ summary, risk = 0, actions = [] }) {
  const frame = risk > 85 ? 'border-red-500/60 crisis' : risk > 60 ? 'border-orange-400/50' : 'border-zinc-800'

  return (
    <motion.section
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      className={`panel rounded-xl border p-4 ${frame}`}
    >
      <h2 className="mb-2 text-sm uppercase tracking-widest text-zinc-400">Latest Advisory</h2>
      <p className="text-sm leading-relaxed text-zinc-100">{summary || 'Awaiting AI advisories.'}</p>
      <div className="mt-3 flex items-center gap-2 text-xs">
        <span className="rounded border border-crimson/60 px-2 py-1 text-zinc-200">Risk {Math.round(risk)}</span>
        <span className="text-zinc-500">{new Date().toLocaleTimeString()}</span>
      </div>
      {actions.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-2">
          {actions.map((action, index) => (
            <span key={`${action}-${index}`} className="rounded bg-zinc-900 px-2 py-1 text-xs text-zinc-300">
              {action}
            </span>
          ))}
        </div>
      )}
    </motion.section>
  )
}
