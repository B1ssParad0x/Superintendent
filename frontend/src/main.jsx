import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App.jsx'
import './index.css'
import { AppAuthProvider } from './context/AuthProvider'
import AppErrorBoundary from './components/AppErrorBoundary'

console.info('[superintendent] entry loaded: main.jsx -> App.jsx')

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
