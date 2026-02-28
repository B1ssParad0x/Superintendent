# Superintendent Frontend

React + Vite client for The Superintendent.

## Features

- Auth0 login-gated landing page and role-aware navigation
- Live dashboard polling `/api/state` every 10 seconds
- Mapbox telemetry heatmap and node overlays from `/api/telemetry`
- Protected logs and admin actions (`/api/logs`, `/api/reason`, `/api/commit`)
- Dark crimson theme with motion polish

## Environment Variables

- `VITE_API_URL` (default: `http://localhost:8000`)
- `VITE_MAPBOX_TOKEN`
- `VITE_AUTH0_DOMAIN`
- `VITE_AUTH0_CLIENT_ID`
- `VITE_AUTH0_AUDIENCE`
- `VITE_AUTH0_REDIRECT_URI` (optional, default: current origin)
- `VITE_AUTH0_LOGOUT_URI` (optional, default: current origin)
- `VITE_AUTH0_ROLE_CLAIM` (optional, default: `https://superintendent/roles`)

If Auth0 env vars are omitted, frontend runs in local dev-admin mode and uses `Bearer dev`.

## Scripts

- `npm run dev`
- `npm run build`
- `npm run preview`
