import { Auth0Provider, useAuth0 } from '@auth0/auth0-react'
import { createContext, useContext, useEffect, useMemo } from 'react'
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

function AuthBridge({ children }) {
  const { isLoading, isAuthenticated, user, loginWithRedirect, logout, getAccessTokenSilently } = useAuth0()
  const roles = parseRoles(user)
  const isAdmin = roles.some((r) => String(r).toLowerCase().includes('admin'))

  useEffect(() => {
    setTokenGetter(async () => (isAuthenticated ? await getAccessTokenSilently() : null))
  }, [isAuthenticated, getAccessTokenSilently])

  const value = useMemo(
    () => ({
      isLoading,
      isAuthenticated,
      user,
      roles,
      roleLabel: isAdmin ? 'admin' : roles[0] || 'viewer',
      isAdmin,
      login: () => loginWithRedirect(),
      logout: () => logout({ logoutParams: { returnTo: authLogoutURI } }),
      getToken: () => getAccessTokenSilently(),
    }),
    [isLoading, isAuthenticated, user, roles, isAdmin, loginWithRedirect, logout, getAccessTokenSilently],
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

  return (
    <Auth0Provider
      domain={authDomain}
      clientId={authClientId}
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
