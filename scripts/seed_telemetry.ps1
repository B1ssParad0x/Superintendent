# Seed sample telemetry for demo (requires backend running)
# Usage: .\seed_telemetry.ps1 [API_URL]
# Example: .\seed_telemetry.ps1 http://localhost:8000

param(
    [string]$API = "http://localhost:8000",
    [string]$Key = $env:EDGE_API_KEY
)

$body = @{
    node_id   = "pi-demo-001"
    ts        = [int][double]::Parse((Get-Date -UFormat %s))
    loc       = @{ lat = 40.7128; lon = -74.006 }
    metrics   = @{ temp_c = 22; humidity = 65; aqi = 42 }
    signature = "dGVzdC1zaWduYXR1cmU="
} | ConvertTo-Json

$headers = @{
    "Content-Type" = "application/json"
}
if ($Key) {
    $headers["X-Edge-Key"] = $Key
}

try {
    $response = Invoke-RestMethod -Uri "$API/api/ingest" -Method Post -Body $body -Headers $headers
    Write-Host "Ingest OK: $response"
} catch {
    Write-Host "Ingest failed: $_"
    exit 1
}
