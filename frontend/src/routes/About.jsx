import { motion } from 'framer-motion'

const items = [
  {
    title: 'Mission',
    body: 'Superintendent provides calm, explainable civic intelligence for emergency operations and urban resilience teams.',
  },
  {
    title: 'Ethics',
    body: 'Human operators retain authority. The AI recommends, logs evidence, and keeps a transparent chain of accountability.',
  },
  {
    title: 'Architecture',
    body: 'Edge telemetry enters a Go API, AI reasoning runs in FastAPI, and commitments are hashed to Solana for immutable public audit.',
  },
]

const principles = [
  'Human operators remain the final authority.',
  'Every critical decision is auditable and attributable.',
  'Risk communication must be clear, calm, and actionable.',
]

export default function About() {
  return (
    <main className="mx-auto max-w-5xl space-y-4 px-4 py-4">
      <section className="panel relative overflow-hidden rounded-xl border border-crimson/30 p-6">
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-br from-crimson/10 via-transparent to-transparent" />
        <h1 className="relative font-display text-3xl text-white">The Superintendent</h1>
        <motion.p
          initial={{ opacity: 0.5 }}
          animate={{ opacity: [0.5, 1, 0.5] }}
          transition={{ duration: 5, repeat: Infinity }}
          className="relative mt-2 text-sm text-crimson"
        >
          "People forget. I do not."
        </motion.p>
        <p className="relative mt-3 max-w-2xl text-sm text-zinc-300">
          A civic intelligence layer that turns noisy urban telemetry into explainable decisions, voice advisories, and immutable accountability trails.
        </p>
      </section>

      <section className="grid gap-3 md:grid-cols-3">
        {principles.map((text) => (
          <motion.div
            key={text}
            initial={{ opacity: 0, y: 6 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            className="panel rounded-xl p-4"
          >
            <p className="text-sm leading-relaxed text-zinc-200">{text}</p>
          </motion.div>
        ))}
      </section>

      <section className="grid gap-3 md:grid-cols-3">
        {items.map((item) => (
          <motion.article
            key={item.title}
            initial={{ opacity: 0, y: 8 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            className="panel rounded-xl p-4"
          >
            <h2 className="font-display text-lg text-zinc-100">{item.title}</h2>
            <p className="mt-2 text-sm leading-relaxed text-zinc-300">{item.body}</p>
          </motion.article>
        ))}
      </section>
    </main>
  )
}
