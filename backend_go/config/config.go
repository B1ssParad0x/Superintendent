package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
)

type Config struct {
	Port          string
	MongoURI      string
	Auth0Domain   string
	Auth0Audience string
	AIWorkerURL   string
	EdgeAPIKey    string
	HostURL       string
	SolanaRPC     string
	SolanaKeypair string
	OpenWeatherKey string
	IngestLat      float64
	IngestLon      float64
	EdgePubkeys    map[string][]byte
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8000"),
		MongoURI:      getEnv("MONGO_URI", "mongodb://localhost:27017"),
		Auth0Domain:   getEnv("AUTH0_DOMAIN", ""),
		Auth0Audience: getEnv("AUTH0_AUDIENCE", ""),
		AIWorkerURL:   getEnv("AI_WORKER_URL", "http://localhost:8001"),
		EdgeAPIKey:    getEnv("EDGE_API_KEY", ""),
		HostURL:       getEnv("HOST_URL", "http://localhost:8000"),
		SolanaRPC:      getEnv("SOLANA_RPC", "https://api.devnet.solana.com"),
		SolanaKeypair:  getEnv("SOLANA_KEYPAIR_JSON", ""),
		OpenWeatherKey: getEnv("OPENWEATHER_KEY", ""),
		IngestLat:      parseFloat(getEnv("INGEST_LAT", "40.7128"), 40.7128),
		IngestLon:      parseFloat(getEnv("INGEST_LON", "-74.006"), -74.006),
		EdgePubkeys:    parseEdgePubkeys(getEnv("EDGE_PUBKEYS", "")),
	}
}

func parseEdgePubkeys(s string) map[string][]byte {
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	out := make(map[string][]byte)
	for nodeID, b64 := range m {
		key, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(key) != 32 {
			continue
		}
		out[nodeID] = key
	}
	return out
}

func parseFloat(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
