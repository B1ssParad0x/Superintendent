#!/bin/bash
# Demo flow: ingest -> (manual: trigger reason, commit) -> check logs
# Windows: run scripts/seed_telemetry.ps1 instead of seed_telemetry.sh
set -e
API=${1:-http://localhost:8000}

echo "1. Seeding telemetry..."
./seed_telemetry.sh "$API"

echo ""
echo "2. Fetching public state..."
curl -s "$API/api/state" | jq .

echo ""
echo "3. Use the dashboard to: Trigger Reason -> Commit to Solana"
echo "   Or call admin endpoints with JWT (see README)"
