import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './index.css'
import { AppAuthProvider } from './context/AuthProvider'
import AppErrorBoundary from './components/AppErrorBoundary'

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <BrowserRouter>
        <AppAuthProvider>
          <App />
        </AppAuthProvider>
      </BrowserRouter>
    </AppErrorBoundary>
  </React.StrictMode>,
)
