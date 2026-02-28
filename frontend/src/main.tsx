import React from 'react'
import ReactDOM from 'react-dom/client'
import { Auth0Provider } from '@auth0/auth0-react'
import App from './App'
import './index.css'

const domain = import.meta.env.VITE_AUTH0_DOMAIN || ''
const clientId = import.meta.env.VITE_AUTH0_CLIENT_ID || ''
const audience = import.meta.env.VITE_AUTH0_AUDIENCE || ''
const hasAuth = domain && clientId

const Root = hasAuth ? (
  <Auth0Provider
    domain={domain}
    clientId={clientId}
    authorizationParams={{
      redirect_uri: window.location.origin,
      audience: audience || undefined,
    }}
  >
    <App />
  </Auth0Provider>
) : (
  <App />
)

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>{Root}</React.StrictMode>,
)
