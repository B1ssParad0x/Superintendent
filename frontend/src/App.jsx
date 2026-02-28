import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import Navbar from './components/Navbar'
import Footer from './components/Footer'
import FaultyTerminal from './components/FaultyTerminal'
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

function AsciiTitle() {
  return (
    <pre className="font-mono text-[10px] leading-tight text-zinc-100 md:text-xs">
      {String.raw`
  _____                         _       _                 _            _   
 / ____|                       (_)     | |               | |          | |  
| (___  _   _ _ __   ___ _ __  _ _ __ | |_ ___ _ __   __| | ___ _ __ | |_ 
 \___ \| | | | '_ \ / _ \ '_ \| | '_ \| __/ _ \ '_ \ / _  |/ _ \ '_ \| __|
 ____) | |_| | |_) |  __/ | | | | | | | ||  __/ | | | (_| |  __/ | | | |_ 
|_____/ \__,_| .__/ \___|_| |_|_|_| |_|\__\___|_| |_|\__,_|\___|_| |_|\__|
             | |                                                            
             |_|                                                            `}
    </pre>
  )
}

function Landing() {
  const { login, isAuthenticated } = useAppAuth()
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
        <AsciiTitle />
        <h1 className="mt-3 font-display text-4xl tracking-wide text-white">Superintendent</h1>
        <p className="mx-auto mt-3 max-w-xl text-sm text-zinc-300">
          Civic intelligence that watches quietly, reasons clearly, and logs decisions immutably.
        </p>
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
      <Footer />
    </div>
  )
}
