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

export default function About() {
  return (
    <main className="mx-auto max-w-4xl space-y-4 px-4 py-4">
      <section className="panel rounded-xl p-5">
        <h1 className="font-display text-2xl text-white">The Superintendent</h1>
        <motion.p
          initial={{ opacity: 0.5 }}
          animate={{ opacity: [0.5, 1, 0.5] }}
          transition={{ duration: 5, repeat: Infinity }}
          className="mt-2 text-sm text-crimson"
        >
          "People forget. I do not."
        </motion.p>
      </section>
      {items.map((item) => (
        <details key={item.title} className="panel rounded-xl p-4" open>
          <summary className="cursor-pointer font-medium text-zinc-100">{item.title}</summary>
          <p className="mt-2 text-sm leading-relaxed text-zinc-300">{item.body}</p>
        </details>
      ))}
    </main>
  )
}
