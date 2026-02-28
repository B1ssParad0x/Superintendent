import { useAuth0 } from '@auth0/auth0-react'
import Dashboard from './components/Dashboard'
import './index.css'

const hasAuth = !!(import.meta.env.VITE_AUTH0_DOMAIN && import.meta.env.VITE_AUTH0_CLIENT_ID)

function AppWithAuth() {
  const { isAuthenticated, isLoading, loginWithRedirect, logout, getAccessTokenSilently } = useAuth0()

  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>
        <span>Loading...</span>
      </div>
    )
  }

  if (!isAuthenticated) {
    return (
      <div style={{
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        gap: '1rem',
      }}>
        <h1 style={{ fontSize: '1.5rem', fontWeight: 600 }}>The Superintendent</h1>
        <p style={{ color: 'var(--text-muted)' }}>AI civic intelligence platform</p>
        <button
          onClick={() => loginWithRedirect()}
          style={{
            padding: '0.5rem 1.5rem',
            background: 'var(--accent)',
            color: 'var(--bg)',
            border: 'none',
            borderRadius: '6px',
            cursor: 'pointer',
            fontWeight: 600,
          }}
        >
          Log in
        </button>
      </div>
    )
  }

  return (
    <div>
      <header style={{
        padding: '0.75rem 1.5rem',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <h1 style={{ margin: 0, fontSize: '1.25rem' }}>The Superintendent</h1>
        <button
          onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}
          style={{
            padding: '0.35rem 0.75rem',
            background: 'transparent',
            color: 'var(--text-muted)',
            border: '1px solid var(--border)',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          Log out
        </button>
      </header>
      <Dashboard getToken={getAccessTokenSilently} />
    </div>
  )
}

function AppNoAuth() {
  return (
    <div>
      <header style={{
        padding: '0.75rem 1.5rem',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}>
        <h1 style={{ margin: 0, fontSize: '1.25rem' }}>The Superintendent <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>(dev mode)</span></h1>
      </header>
      <Dashboard getToken={async () => 'dev'} />
    </div>
  )
}

function App() {
  return hasAuth ? <AppWithAuth /> : <AppNoAuth />
}

export default App
