import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import Navbar from './components/Navbar'
import Footer from './components/Footer'
import FaultyTerminal from './components/FaultyTerminal'
import ASCIIText from './components/ASCIIText'
import AppErrorBoundary from './components/AppErrorBoundary'
import { useAppAuth } from './context/AuthProvider'

const Dashboard = lazy(() => import('./routes/Dashboard'))
const Logs = lazy(() => import('./routes/Logs'))
const Nodes = lazy(() => import('./routes/Nodes'))
const Admin = lazy(() => import('./routes/Admin'))
const About = lazy(() => import('./routes/About'))

function ProtectedRoute({ children, adminOnly = false }) {
  const { isLoading, isAuthenticated, isAdmin } = useAppAuth()
  if (isLoading) return <div className="p-8 text-center text-zinc-400">Authenticating...</div>
  if (!isAuthenticated) return <Navigate to="/" replace />
  if (adminOnly && !isAdmin) return <Navigate to="/dashboard" replace />
  return children
}

function Landing() {
  const { login, isAuthenticated, authError } = useAppAuth()
  if (isAuthenticated) return <Navigate to="/dashboard" replace />
  return (
    <main className="relative flex min-h-[calc(100vh-118px)] items-center justify-center overflow-hidden px-4">
      <div className="pointer-events-none absolute inset-0 opacity-45">
        <FaultyTerminal
          scale={1.08}
          gridMul={[2.2, 1.1]}
          digitSize={1.4}
          timeScale={0.28}
          scanlineIntensity={0.42}
          flickerAmount={0.8}
          curvature={0.24}
          tint="#e5e7eb"
          mouseReact
          mouseStrength={0.18}
          pageLoadAnimation
          brightness={0.95}
        />
      </div>
      <section className="panel relative z-10 w-full max-w-3xl rounded-2xl border border-crimson/40 p-8 text-center">
        <div className="relative mx-auto mb-4 h-44 w-full max-w-2xl overflow-hidden rounded-md">
          <ASCIIText text="Superintendent" enableWaves={false} asciiFontSize={1} />
        </div>
        <p className="mx-auto mt-2 max-w-xl text-sm text-zinc-300">
          Civic intelligence that watches quietly, reasons clearly, and logs decisions immutably.
        </p>
        {authError && (
          <p className="mx-auto mt-3 max-w-xl rounded border border-red-500/40 bg-red-950/20 px-3 py-2 text-xs text-red-200">
            Auth error: {authError.message || 'Check callback/logout URLs in Auth0 app settings.'}
          </p>
        )}
        <button onClick={login} className="mt-6 rounded-md bg-crimson px-5 py-2 text-sm font-semibold text-white shadow-glow">
          Sign in with Auth0
        </button>
      </section>
    </main>
  )
}

export default function App() {
  return (
    <div className="min-h-screen bg-black text-white">
      <Navbar />
      <AppErrorBoundary>
        <Suspense fallback={<div className="p-8 text-center text-sm text-zinc-500">Loading view...</div>}>
          <Routes>
            <Route path="/" element={<Landing />} />
            <Route
              path="/dashboard"
              element={
                <ProtectedRoute>
                  <Dashboard />
                </ProtectedRoute>
              }
            />
            <Route
              path="/logs"
              element={
                <ProtectedRoute>
                  <Logs />
                </ProtectedRoute>
              }
            />
            <Route
              path="/nodes"
              element={
                <ProtectedRoute>
                  <Nodes />
                </ProtectedRoute>
              }
            />
            <Route
              path="/admin"
              element={
                <ProtectedRoute adminOnly>
                  <Admin />
                </ProtectedRoute>
              }
            />
            <Route path="/about" element={<About />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </AppErrorBoundary>
      <Footer />
    </div>
  )
}
