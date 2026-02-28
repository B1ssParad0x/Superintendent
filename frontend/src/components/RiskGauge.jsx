import { motion } from 'framer-motion'

export default function RiskGauge({ value = 0 }) {
  const safeValue = Math.max(0, Math.min(100, Number(value) || 0))
  const circumference = 2 * Math.PI * 54
  const dashOffset = circumference - (safeValue / 100) * circumference
  const tone = safeValue > 85 ? 'text-red-400' : safeValue > 60 ? 'text-orange-300' : 'text-emerald-300'

  return (
    <div className="panel rounded-xl p-4">
      <h3 className="mb-3 text-sm uppercase tracking-wider text-zinc-400">City Risk Index</h3>
      <div className="relative mx-auto h-36 w-36">
        <svg viewBox="0 0 140 140" className="h-36 w-36 -rotate-90">
          <circle cx="70" cy="70" r="54" className="fill-none stroke-zinc-800" strokeWidth="10" />
          <motion.circle
            cx="70"
            cy="70"
            r="54"
            className="fill-none stroke-crimson"
            strokeWidth="10"
            strokeLinecap="round"
            strokeDasharray={circumference}
            animate={{ strokeDashoffset: dashOffset }}
            transition={{ duration: 0.6 }}
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className={`font-display text-3xl ${tone}`}>{safeValue}</span>
          <span className="text-xs text-zinc-400">/100</span>
        </div>
      </div>
    </div>
  )
}
