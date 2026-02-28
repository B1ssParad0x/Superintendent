import { motion } from 'framer-motion'

export default function SystemMoodOrb({ risk = 0 }) {
  const tone = risk > 85 ? 'bg-red-500' : risk > 60 ? 'bg-orange-400' : 'bg-emerald-400'
  return (
    <div className="panel rounded-xl p-4">
      <h3 className="mb-3 text-sm uppercase tracking-wider text-zinc-400">System Mood</h3>
      <div className="flex items-center justify-center py-4">
        <motion.div
          className={`h-20 w-20 rounded-full ${tone}`}
          animate={{ scale: [1, 1.08, 1], opacity: [0.9, 1, 0.9] }}
          transition={{ duration: 4, repeat: Infinity, ease: 'easeInOut' }}
          style={{ boxShadow: '0 0 36px rgba(210, 4, 45, 0.45)' }}
        />
      </div>
      <p className="text-center text-xs text-zinc-500">
        {risk > 85 ? 'Crisis mode' : risk > 60 ? 'Elevated monitoring' : 'Stable baseline'}
      </p>
    </div>
  )
}
