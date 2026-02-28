import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 5173,
  },
  build: {
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('mapbox-gl') || id.includes('react-map-gl')) return 'map'
          if (id.includes('@auth0/auth0-react') || id.includes('axios')) return 'auth-api'
          if (id.includes('ogl')) return 'ogl'
          if (id.includes('framer-motion')) return 'motion'
          if (id.includes('react-router-dom')) return 'router'
          return 'vendor'
        },
      },
    },
  },
})
