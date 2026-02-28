#!/bin/bash
# Seed sample telemetry for demo (requires backend running)
API=${1:-http://localhost:8000}
KEY=${EDGE_API_KEY:-}

curl -X POST "$API/api/ingest" \
  -H "Content-Type: application/json" \
  ${KEY:+ -H "X-Edge-Key: $KEY"} \
  -d '{
    "node_id": "pi-demo-001",
    "ts": '$(date +%s)',
    "loc": {"lat": 40.7128, "lon": -74.006},
    "metrics": {"temp_c": 22, "humidity": 65, "aqi": 42},
    "signature": "dGVzdC1zaWduYXR1cmU="
  }'

echo ""
