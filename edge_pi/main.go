// Superintendent Edge Agent - Raspberry Pi 5 telemetry collector.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxCachedAudios = 10

// Config from env
var (
	apiURL      string
	edgeID      string
	keyPath     string
	cacheDir    string
	sendInterval time.Duration
	lat, lon    float64
	edgeAPIKey  string
)

func loadConfig() {
	apiURL = getEnv("SUPER_API", "http://localhost:8000")
	edgeID = getEnv("EDGE_ID", "pi-001")
	keyPath = getEnv("EDGE_KEY_PATH", "edge_key.json")
	cacheDir = getEnv("PI_CACHE", "./cache")
	edgeAPIKey = getEnv("EDGE_API_KEY", "")
	if i, err := strconv.Atoi(getEnv("SEND_INTERVAL", "60")); err == nil {
		sendInterval = time.Duration(i) * time.Second
	} else {
		sendInterval = 60 * time.Second
	}
	lat, _ = strconv.ParseFloat(getEnv("PI_LAT", "40.7128"), 64)
	lon, _ = strconv.ParseFloat(getEnv("PI_LON", "-74.0060"), 64)
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadOrGenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey) {
	data, err := os.ReadFile(keyPath)
	if err == nil {
		var kv struct {
			Pub  string `json:"pub"`
			Priv string `json:"priv"`
		}
		if json.Unmarshal(data, &kv) == nil && kv.Pub != "" && kv.Priv != "" {
			pub, _ := base64.StdEncoding.DecodeString(kv.Pub)
			priv, _ := base64.StdEncoding.DecodeString(kv.Priv)
			if len(pub) == ed25519.PublicKeySize && len(priv) == ed25519.PrivateKeySize {
				return pub, priv
			}
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal("generate key:", err)
	}
	_ = os.MkdirAll(filepath.Dir(keyPath), 0700)
	b, _ := json.Marshal(map[string]string{
		"pub":  base64.StdEncoding.EncodeToString(pub),
		"priv": base64.StdEncoding.EncodeToString(priv),
	})
	_ = os.WriteFile(keyPath, b, 0600)
	return pub, priv
}

// filterPII scrubs any PII from metrics
func filterPII(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range m {
		// Skip common PII keys
		if k == "email" || k == "phone" || k == "name" || k == "address" {
			continue
		}
		out[k] = v
	}
	return out
}

func signPayload(payload string, priv ed25519.PrivateKey) string {
	sig := ed25519.Sign(priv, []byte(payload))
	return base64.StdEncoding.EncodeToString(sig)
}

func collectMetrics() map[string]interface{} {
	// Simulated metrics - in production, read from sensors or APIs
	return filterPII(map[string]interface{}{
		"temp_c":      22.5,
		"humidity":    65,
		"aqi":         45,
		"noise_db":    55,
		"timestamp":   time.Now().Unix(),
	})
}

func sendTelemetry(_ ed25519.PublicKey, priv ed25519.PrivateKey) error {
	ts := time.Now().Unix()
	metrics := collectMetrics()
	metricsJSON, _ := json.Marshal(metrics)
	payload := fmt.Sprintf("%s|%d|%.6f|%.6f|%s", edgeID, ts, lat, lon, string(metricsJSON))
	sig := signPayload(payload, priv)

	body := map[string]interface{}{
		"node_id":   edgeID,
		"ts":        ts,
		"loc":       map[string]float64{"lat": lat, "lon": lon},
		"metrics":   metrics,
		"signature": sig,
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost, apiURL+"/api/ingest", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if edgeAPIKey != "" {
		req.Header.Set("X-Edge-Key", edgeAPIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ingest: %s", resp.Status)
	}
	return nil
}

func fetchAndCacheAudios() {
	req, err := http.NewRequest(http.MethodGet, apiURL+"/api/edge/audios", nil)
	if err != nil {
		return
	}
	if edgeAPIKey != "" {
		req.Header.Set("X-Edge-Key", edgeAPIKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Audios []struct {
			Hash     string `json:"hash"`
			AudioURL string `json:"audio_url"`
			Summary  string `json:"summary"`
		} `json:"audios"`
	}
	if json.Unmarshal(body, &out) != nil {
		return
	}
	audioDir := filepath.Join(cacheDir, "audio")
	_ = os.MkdirAll(audioDir, 0755)
	for i, a := range out.Audios {
		if a.AudioURL == "" || i >= maxCachedAudios {
			continue
		}
		if !strings.HasPrefix(a.AudioURL, "data:audio/") {
			continue
		}
		parts := strings.SplitN(a.AudioURL, ",", 2)
		if len(parts) != 2 {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		name := a.Hash
		if name == "" {
			h := sha256.Sum256(data)
			name = fmt.Sprintf("%x", h[:8])
		}
		fpath := filepath.Join(audioDir, name+".mp3")
		if err := os.WriteFile(fpath, data, 0644); err != nil {
			log.Printf("cache audio: %v", err)
		}
	}
	// Prune to last 10
	entries, err := os.ReadDir(audioDir)
	if err != nil || len(entries) <= maxCachedAudios {
		return
	}
	type fi struct {
		path string
		mod  time.Time
	}
	var files []fi
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fi{filepath.Join(audioDir, e.Name()), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	for i := maxCachedAudios; i < len(files); i++ {
		_ = os.Remove(files[i].path)
	}
}

func main() {
	loadConfig()
	pub, priv := loadOrGenerateKeypair()
	_ = pub

	_ = os.MkdirAll(cacheDir, 0755)

	audioTicker := time.NewTicker(2 * time.Minute)
	go func() {
		fetchAndCacheAudios()
		for range audioTicker.C {
			fetchAndCacheAudios()
		}
	}()

	log.Printf("Edge %s starting, send interval %v", edgeID, sendInterval)
	for {
		if err := sendTelemetry(pub, priv); err != nil {
			log.Printf("send failed: %v", err)
		} else {
			log.Printf("sent telemetry ok")
		}
		time.Sleep(sendInterval)
	}
}
