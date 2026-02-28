import { create } from 'zustand'

export const useAppState = create((set) => ({
  summary: 'System online. Awaiting data.',
  risk: 0,
  lastAdvisory: null,
  advisories: [],
  setFromState: (payload) =>
    set((state) => {
      const nextSummary = payload?.summary || state.summary
      const nextRisk = Number(payload?.risk ?? state.risk ?? 0)
      const advisory = {
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        summary: nextSummary,
        risk: nextRisk,
        when: new Date().toISOString(),
      }
      const advisories = [advisory, ...state.advisories].slice(0, 25)
      return {
        summary: nextSummary,
        risk: Math.max(0, Math.min(100, nextRisk)),
        lastAdvisory: advisory,
        advisories,
      }
    }),
}))
