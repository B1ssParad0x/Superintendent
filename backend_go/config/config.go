package config

import (
	"os"
)

type Config struct {
	Port         string
	MongoURI     string
	Auth0Domain  string
	Auth0Audience string
	AIWorkerURL  string
	EdgeAPIKey   string
	HostURL      string
	SolanaRPC    string
	SolanaKeypair string
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
		SolanaRPC:     getEnv("SOLANA_RPC", "https://api.devnet.solana.com"),
		SolanaKeypair: getEnv("SOLANA_KEYPAIR_JSON", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
