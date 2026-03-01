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
  // Keep the Admin page visible even when claim mapping is wrong; backend still enforces admin on actions.
  if (adminOnly && !isAdmin) return children
  return children
}

function Landing() {
  const { login, isAuthenticated, authError } = useAppAuth()
  if (isAuthenticated) return <Navigate to="/dashboard" replace />
  return (
    <main className="relative flex min-h-[calc(100vh-118px)] flex-1 items-center justify-center overflow-hidden px-4">
      <div className="pointer-events-none absolute inset-0 opacity-45">
        <div style={{ width: '100%', height: '100%', position: 'relative' }}>
          <FaultyTerminal
            scale={3}
            gridMul={[2, 1]}
            digitSize={0.6}
            timeScale={0.5}
            pause={false}
            scanlineIntensity={0.5}
            glitchAmount={1}
            flickerAmount={1}
            noiseAmp={1}
            chromaticAberration={0}
            dither={0}
            curvature={0.22}
            tint="#ff0000"
            mouseReact
            mouseStrength={0.3}
            pageLoadAnimation
            brightness={0.6}
          />
        </div>
      </div>
      <section className="panel relative z-10 w-full max-w-5xl rounded-2xl border border-crimson/40 p-8 text-center">
        <div className="relative mx-auto mb-4 h-56 w-full max-w-5xl rounded-md" data-ascii-source="ASCIIText.jsx">
          <ASCIIText text={'Superintendent   '} enableWaves={false} asciiFontSize={7} textFontSize={158} planeBaseHeight={5.8} />
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
    <div className="flex min-h-screen flex-col bg-black text-white">
      <Navbar />
      <div className="flex-1">
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
      </div>
      <Footer />
    </div>
  )
}
