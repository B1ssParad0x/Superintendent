import { Auth0Provider, useAuth0 } from '@auth0/auth0-react'
import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { setTokenGetter } from './apiClient'

const AuthContext = createContext(null)

const authDomain = import.meta.env.VITE_AUTH0_DOMAIN || ''
const authClientId = import.meta.env.VITE_AUTH0_CLIENT_ID || ''
const authAudience = import.meta.env.VITE_AUTH0_AUDIENCE || ''
const authRedirectURI = import.meta.env.VITE_AUTH0_REDIRECT_URI || window.location.origin
const authLogoutURI = import.meta.env.VITE_AUTH0_LOGOUT_URI || window.location.origin
const roleClaimKey = import.meta.env.VITE_AUTH0_ROLE_CLAIM || 'https://superintendent/roles'
const hasAuth = Boolean(authDomain && authClientId)

function parseRoles(user) {
  if (!user) return ['viewer']
  const claimRoles = user?.[roleClaimKey]
  if (Array.isArray(claimRoles) && claimRoles.length) return claimRoles
  if (typeof claimRoles === 'string') return [claimRoles]
  return ['viewer']
}

function parseRolesFromAccessToken(token) {
  if (!token || typeof token !== 'string') return []
  const parts = token.split('.')
  if (parts.length < 2) return []
  try {
    const payload = JSON.parse(atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')))
    const claimRoles = payload?.[roleClaimKey]
    if (Array.isArray(claimRoles)) return claimRoles
    if (typeof claimRoles === 'string') return [claimRoles]
  } catch {
    // Ignore malformed token payloads.
  }
  return []
}

function AuthBridge({ children }) {
  const { isLoading, isAuthenticated, user, error, loginWithRedirect, logout, getAccessTokenSilently } = useAuth0()
  const [tokenRoles, setTokenRoles] = useState([])
  const profileRoles = parseRoles(user)
  const roles = tokenRoles.length > 0 ? tokenRoles : profileRoles
  const isAdmin = roles.some((r) => String(r).toLowerCase().includes('admin'))

  useEffect(() => {
    setTokenGetter(async () => (isAuthenticated ? await getAccessTokenSilently() : null))
    let mounted = true
    ;(async () => {
      if (!isAuthenticated) {
        if (mounted) setTokenRoles([])
        return
      }
      try {
        const token = await getAccessTokenSilently()
        if (!mounted) return
        setTokenRoles(parseRolesFromAccessToken(token))
      } catch {
        if (mounted) setTokenRoles([])
      }
    })()
    return () => {
      mounted = false
    }
  }, [isAuthenticated, getAccessTokenSilently, user?.sub])

  const value = useMemo(
    () => ({
      isLoading,
      isAuthenticated,
      user,
      roles,
      roleLabel: isAdmin ? 'admin' : roles[0] || 'viewer',
      isAdmin,
      authError: error || null,
      login: () => loginWithRedirect({ appState: { returnTo: '/dashboard' } }),
      logout: () => logout({ logoutParams: { returnTo: authLogoutURI } }),
      getToken: () => getAccessTokenSilently(),
    }),
    [isLoading, isAuthenticated, user, roles, isAdmin, error, loginWithRedirect, logout, getAccessTokenSilently],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

function DevAuthProvider({ children }) {
  useEffect(() => {
    setTokenGetter(async () => 'dev')
  }, [])

  const value = useMemo(
    () => ({
      isLoading: false,
      isAuthenticated: true,
      user: { name: 'Dev Operator', email: 'dev@local' },
      roles: ['admin'],
      roleLabel: 'admin',
      isAdmin: true,
      authError: null,
      login: () => {},
      logout: () => {},
      getToken: async () => 'dev',
      devMode: true,
    }),
    [],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function AppAuthProvider({ children }) {
  if (!hasAuth) return <DevAuthProvider>{children}</DevAuthProvider>

  const onRedirectCallback = (appState) => {
    const target = appState?.returnTo || '/dashboard'
    window.history.replaceState({}, document.title, target)
  }

  return (
    <Auth0Provider
      domain={authDomain}
      clientId={authClientId}
      cacheLocation="localstorage"
      useRefreshTokens
      onRedirectCallback={onRedirectCallback}
      authorizationParams={{
        redirect_uri: authRedirectURI,
        audience: authAudience || undefined,
      }}
    >
      <AuthBridge>{children}</AuthBridge>
    </Auth0Provider>
  )
}

export function useAppAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAppAuth must be used within AppAuthProvider')
  }
  return ctx
}
