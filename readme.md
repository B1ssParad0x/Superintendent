# Superintendent

> _"A city is a body. I am its nervous system."_

---

## Overview

**Superintendent** is an AI-driven civic intelligence platform designed to monitor, interpret, and voice the state of a city in real time. It unifies live data streams, advanced reasoning, and secure infrastructure into one cohesive system that can summarize urban conditions, predict issues, and deliver natural, human-like advisories.

The project fuses AI, blockchain auditing, and edge computing to build a secure, ethical, and insightful Smart City management framework.

---

## Key Features

- **Live Data Ingestion:** Integrates public APIs such as weather and civic service feeds.
- **AI Reasoning:** Uses a large language model to interpret data, identify trends, and propose actions.
- **Natural Voice Output:** Generates expressive audio advisories for human operators.
- **Immutable Logging:** Records critical decisions on the Solana devnet blockchain for transparency.
- **Edge Device Integration:** Raspberry Pi acts as a secure edge node that signs telemetry and caches local data.
- **Secure Authentication:** Protected by Auth0 JWT and cryptographic signing for data integrity.
- **Web Dashboard:** Displays real-time maps, advisories, and blockchain audit trails.

---

## Architecture

```
[Edge Device]  →  [Backend Server]  →  [AI Worker]  →  [Dashboard]
       |                |                   |                |
   (Go Agent)     (Go + MongoDB)     (Python FastAPI)     (React + Mapbox)
```

**Components:**
- **Edge Device:** Raspberry Pi 5 running a Go agent that signs and transmits telemetry.
- **Backend:** Go API server with MongoDB Atlas integration, Auth0 authentication, and Solana auditing.
- **AI Worker:** Python FastAPI service responsible for reasoning (Gemini API) and voice synthesis (ElevenLabs).
- **Frontend:** React dashboard visualizing live data, advisories, and blockchain records.

---

## Tech Stack

- **Languages:** Go, Python, JavaScript (React)
- **Frameworks:** Gin, FastAPI, Vite
- **Database:** MongoDB Atlas
- **Auth:** Auth0
- **Blockchain:** Solana devnet
- **Deployment:** Docker, Vultr Cloud

---

## Setup

### Prerequisites
- Docker + Docker Compose
- MongoDB Atlas account
- Auth0 credentials
- API keys for AI and voice services

### Environment Variables

Copy `.env.example` to `.env` and configure:

| Variable | Description |
|----------|-------------|
| `AUTH0_DOMAIN` | Auth0 tenant (optional for dev) |
| `AUTH0_AUDIENCE` | API audience |
| `AUTH0_CLIENT_ID` | SPA client ID |
| `MONGO_URI` | MongoDB connection (default: `mongodb://mongo:27017` in Docker) |
| `GEMINI_API_KEY` | Google Gemini API key |
| `ELEVEN_API_KEY` | ElevenLabs API key |
| `ELEVEN_VOICE_ID` | ElevenLabs voice ID |
| `SOLANA_KEYPAIR_JSON` | Base64 or JSON array keypair for devnet |
| `SOLANA_RPC` | Solana RPC URL (default: devnet) |
| `EDGE_API_KEY` | Shared secret for edge ingest (optional) |
| `MAPBOX_TOKEN` | Mapbox token for dashboard map |

**Dev mode:** Without Auth0 configured, the frontend runs in dev mode and the backend accepts `Bearer dev` for admin routes.

### Run Locally

```bash
cp .env.example .env
# Edit .env with your keys
docker-compose up --build
```

- Backend: http://localhost:8000
- Frontend: http://localhost:5173
- AI Worker: http://localhost:8001

### Production (Vultr + Atlas + Vercel)

Use the production compose when deploying backend + AI worker to a server with MongoDB Atlas:

```bash
# Set MONGO_URI, HOST_URL, and other vars in .env
docker compose -f docker-compose.prod.yml up -d --build
```

Deploy the frontend separately (e.g. Vercel) with `VITE_API_URL` pointing to your backend.

### Edge Agent (Pi)

```bash
cd edge_pi
go build -o edge .
SUPER_API=http://your-backend:8000 EDGE_ID=pi-001 ./edge
```

### Scripts

- `scripts/seed_telemetry.sh` – Seed demo telemetry (bash)
- `scripts/seed_telemetry.ps1` – Same for Windows (PowerShell)
- `scripts/demo.sh` – Full demo flow

---

## Security Principles

- Every telemetry packet from the edge is signed using Ed25519 keys.
- Auth0 enforces role-based access for all routes.
- HTTPS enforced across all layers.
- Sensitive data filtered at source before storage.
- Immutable blockchain log ensures audit transparency.

---

## Demo Workflow

1. Edge agent collects local or simulated telemetry.
2. Backend verifies the signature and stores the data.
3. AI Worker analyzes trends and generates summaries and advisories.
4. Admin commits key decisions to Solana for audit.
5. Dashboard displays current state, voice advisories, and blockchain proof.

---

## Ethical Framework

- Human-in-the-loop: AI only assists, never enforces.
- Transparent reasoning and data sources.
- Privacy and compliance by design.
- Immutable records for accountability.

---

## License

Uhhh, IDK, it's mine lol?




              \
               \
                \\
                 \\
                  >\/7
              _.-(6'  \
             (=___._/` \
                  )  \ |
                 /   / |
                /    > /
               j    < _\
           _.-' :      ``.
           \ r=._\        `.
          <`\\_  \         .`-.
           \ r-7  `-. ._  ' .  `\
            \`,      `-.`7  7)   )
             \/         \|  \'  / `-._
                        ||    .'
                         \\  (
                          >\  >
                      ,.-' >.'
                     <.'_.''
                       <'
