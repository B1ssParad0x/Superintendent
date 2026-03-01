package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"superintendent/backend/ai"
	"superintendent/backend/auth"
	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/ingest"
	"superintendent/backend/models"
	"superintendent/backend/solana"
	"superintendent/backend/worker"
)

type Handlers struct {
	cfg    *config.Config
	ai     *ai.Client
	worker *worker.Client
	solana *solana.Client
}

func New(cfg *config.Config) (*Handlers, error) {
	w := worker.New(cfg.AIWorkerURL)
	g := ai.New(cfg)
	sc, err := solana.New(cfg.SolanaRPC, cfg.SolanaKeypair)
	if err != nil {
		return nil, err
	}
	return &Handlers{cfg: cfg, ai: g, worker: w, solana: sc}, nil
}

func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"ts": time.Now().UTC(),
	})
}

// Ingest handles POST /api/ingest - verify edge signature, store, enqueue AI
func (h *Handlers) Ingest(c *gin.Context) {
	var req models.IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Edge verification
	switch {
	case req.NodeID == "api-ingest-001" && req.Signature == "server-ingest":
		// Server-side API ingest (NYC 311, OpenWeather)
	case len(h.cfg.EdgePubkeys) > 0 && h.cfg.EdgePubkeys[req.NodeID] != nil:
		// Ed25519 verification when pubkey is registered
		metricsJSON, _ := json.Marshal(req.Metrics)
		payload := auth.BuildSignablePayload(req.NodeID, req.Ts, req.Loc.Lat, req.Loc.Lon, string(metricsJSON))
		if !auth.VerifyEdgeSignature(payload, req.Signature, h.cfg.EdgePubkeys[req.NodeID]) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid edge signature"})
			return
		}
	case h.cfg.EdgeAPIKey != "":
		if c.GetHeader("X-Edge-Key") != h.cfg.EdgeAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid edge key"})
			return
		}
	}

	doc := models.Telemetry{
		NodeID:      req.NodeID,
		Ts:          time.Unix(req.Ts, 0),
		Loc:         req.Loc,
		Metrics:     req.Metrics,
		Signature:   req.Signature,
		CityID:      h.defaultCity().CityID,
		CityName:    h.defaultCity().CityName,
		CountryCode: h.defaultCity().CountryCode,
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := db.TelemetryCol.InsertOne(ctx, doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store"})
		return
	}

	metricsJSON, _ := json.Marshal(req.Metrics)
	activeCity := h.defaultCity()
	go func() {
		_, _ = h.ai.Reason(context.Background(), activeCity.CityName, string(metricsJSON), nil)
	}()

	c.JSON(http.StatusOK, gin.H{"ok": true, "node_id": req.NodeID})
}

// Reason handles POST /api/reason - admin only, triggers AI reasoning
func (h *Handlers) Reason(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()

	// Fetch recent telemetry for context
	cur, err := db.TelemetryCol.Find(ctx, h.cityFilter(activeCity.CityID),
		options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}).SetLimit(10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch telemetry"})
		return
	}
	var t []models.Telemetry
	if err := cur.All(ctx, &t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode"})
		return
	}
	summary := "No recent telemetry."
	if len(t) > 0 {
		b, _ := json.Marshal(t)
		summary = string(b)
	}
	signals, signalScore := h.collectRiskSignals(ctx, activeCity)
	if len(signals) > 0 {
		if b, err := json.Marshal(signals); err == nil {
			summary = summary + "\n\nPublic safety signals:\n" + string(b)
		}
	}

	recentDecisions := h.getRecentDecisionSummaries(ctx, activeCity.CityID, 5)
	if strings.EqualFold(strings.TrimSpace(c.Query("focus")), "predictive") {
		recentDecisions = append(recentDecisions,
			"OPERATOR_REQUEST: prioritize predictive analysis with explicit 30m, 3h, and 12h outlooks plus key leading indicators.")
	}
	resp, err := h.ai.Reason(ctx, activeCity.CityName, summary, recentDecisions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp = h.enforceGroundedAdvisory(activeCity, resp, signals, signalScore)
	if resp.RiskScore <= 0 {
		resp.RiskScore = h.scoreFromRiskLabel(resp.Risk)
	}

	// Optionally generate audio
	audioURL := ""
	if resp.AudioText != "" && h.worker != nil {
		audioURL, _ = h.worker.Speak(ctx, advisorySpeechText(resp.Summary, resp.Forecast))
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":    resp.Summary,
		"risk":       resp.Risk,
		"risk_score": resp.RiskScore,
		"actions":    resp.Actions,
		"forecast":   resp.Forecast,
		"confidence": resp.Confidence,
		"audio_text": resp.AudioText,
		"audio_url":  audioURL,
		"explain":    resp.Explain,
	})

	// Persist the advisory so operators can see fresh AI guidance in logs/state.
	_ = h.saveAdvisory(ctx, activeCity, resp, audioURL, "manual_reason")
}

// Commit handles POST /api/commit - admin only, hashes and submits to Solana
func (h *Handlers) Commit(c *gin.Context) {
	var body struct {
		Summary  string `json:"summary" binding:"required"`
		AudioURL string `json:"audio_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	hash := auth.HashForSolana(body.Summary + "|" + activeCity.CityID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if h.cfg.SolanaRequireOnchain && (h.solana == nil || !h.solana.Enabled()) {
		c.JSON(http.StatusPreconditionFailed, gin.H{
			"error": "on-chain commit required but SOLANA_KEYPAIR_JSON is not configured",
		})
		return
	}

	txSig, err := h.solana.SubmitMemo(ctx, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Prefer marking an existing uncommitted advisory as committed.
	update := bson.M{
		"$set": bson.M{
			"solana_tx": txSig,
			"hash":      hash,
			"source":    "manual_commit",
		},
	}
	filter := bson.M{
		"city_id":   activeCity.CityID,
		"summary":   body.Summary,
		"solana_tx": bson.M{"$in": []any{"", nil}},
	}
	result, err := db.DecisionsCol.UpdateOne(ctx, filter, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update existing advisory"})
		return
	}
	if result.MatchedCount == 0 {
		doc := models.Decision{
			When:        time.Now(),
			Summary:     body.Summary,
			Hash:        hash,
			AudioURL:    body.AudioURL,
			SolanaTx:    txSig,
			CityID:      activeCity.CityID,
			CityName:    activeCity.CityName,
			CountryCode: activeCity.CountryCode,
			Source:      "manual_commit",
		}
		if _, err := db.DecisionsCol.InsertOne(ctx, doc); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store decision"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"tx": txSig, "hash": hash})
}

// CommitLatest commits the newest uncommitted advisory for the active city.
func (h *Handlers) CommitLatest(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if h.cfg.SolanaRequireOnchain && (h.solana == nil || !h.solana.Enabled()) {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "on-chain commit required but SOLANA_KEYPAIR_JSON is not configured"})
		return
	}

	var latest models.Decision
	err := db.DecisionsCol.FindOne(ctx, bson.M{
		"city_id":   activeCity.CityID,
		"solana_tx": bson.M{"$in": []any{"", nil}},
	}, options.FindOne().SetSort(bson.D{{Key: "when", Value: -1}})).Decode(&latest)
	if err != nil || strings.TrimSpace(latest.Summary) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no uncommitted advisory found"})
		return
	}

	hash := auth.HashForSolana(latest.Summary + "|" + activeCity.CityID)
	txSig, err := h.solana.SubmitMemo(ctx, hash)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	_, err = db.DecisionsCol.UpdateOne(ctx, bson.M{
		"city_id": activeCity.CityID,
		"when":    latest.When,
		"summary": latest.Summary,
	}, bson.M{
		"$set": bson.M{
			"solana_tx": txSig,
			"hash":      hash,
			"source":    "manual_commit",
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update advisory"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tx":      txSig,
		"hash":    hash,
		"summary": latest.Summary,
	})
}

// Telemetry handles GET /api/telemetry - public recent telemetry for map
func (h *Handlers) Telemetry(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	cityID := c.Query("city_id")
	filter := bson.M{}
	if cityID != "" {
		filter = h.cityFilter(cityID)
	}
	cur, err := db.TelemetryCol.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}).SetLimit(500))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var list []models.Telemetry
	if err := cur.All(ctx, &list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"telemetry": list})
}

// EdgeAudios returns last 10 decisions with audio for edge caching
func (h *Handlers) EdgeAudios(c *gin.Context) {
	if h.cfg.EdgeAPIKey != "" && c.GetHeader("X-Edge-Key") != h.cfg.EdgeAPIKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid edge key"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	cur, err := db.DecisionsCol.Find(ctx, bson.M{"audio_url": bson.M{"$ne": ""}},
		options.Find().SetSort(bson.D{{Key: "when", Value: -1}}).SetLimit(10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var list []models.Decision
	if err := cur.All(ctx, &list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"audios": list})
}

// State handles GET /api/state - public city status
func (h *Handlers) State(c *gin.Context) {
	activeCity := h.cityFromRequest(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	filter := h.cityFilter(activeCity.CityID)
	count, _ := db.TelemetryCol.CountDocuments(ctx, filter)
	decCount, _ := db.DecisionsCol.CountDocuments(ctx, filter)

	summary := ""
	var latest models.Decision
	if err := db.DecisionsCol.FindOne(ctx, filter,
		options.FindOne().SetSort(bson.D{{Key: "when", Value: -1}})).Decode(&latest); err == nil && latest.Summary != "" {
		summary = latest.Summary
	}
	if summary == "" && count > 0 {
		summary = "Telemetry flowing. No advisories yet."
	}
	if summary == "" {
		summary = "System online. Awaiting data."
	}

	c.JSON(http.StatusOK, models.CityState{
		Status:  "operational",
		Updated: time.Now(),
		Alerts:  int(decCount),
		Summary: summary,
		CityID:  activeCity.CityID,
		CityName: activeCity.CityName,
		CountryCode: activeCity.CountryCode,
	})
	_ = count
}

// Logs handles GET /api/logs - admin only, decision history
func (h *Handlers) Logs(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	cur, err := db.DecisionsCol.Find(ctx, h.cityFilter(activeCity.CityID),
		options.Find().SetSort(bson.D{{Key: "when", Value: -1}}).SetLimit(100))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var decisions []models.Decision
	if err := cur.All(ctx, &decisions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"decisions": decisions})
}

// VerifyAuditTrail checks decision hash integrity and Solana commit status.
func (h *Handlers) VerifyAuditTrail(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()

	cur, err := db.DecisionsCol.Find(ctx, h.cityFilter(activeCity.CityID),
		options.Find().SetSort(bson.D{{Key: "when", Value: -1}}).SetLimit(300))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var decisions []models.Decision
	if err := cur.All(ctx, &decisions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	issues := make([]gin.H, 0)
	committed := 0
	stubbed := 0
	hashMismatch := 0
	for _, d := range decisions {
		expected := ""
		if strings.TrimSpace(d.SolanaTx) != "" {
			expected = auth.HashForSolana(d.Summary + "|" + d.CityID)
		} else if strings.TrimSpace(d.Source) != "" {
			expected = auth.HashForSolana(d.Summary + "|" + d.CityID + "|" + d.Source)
		} else {
			// Older local-only rows may not include a source. Keep them verifiable.
			expected = auth.HashForSolana(d.Summary + "|" + d.CityID)
		}
		if strings.TrimSpace(expected) != strings.TrimSpace(d.Hash) {
			hashMismatch++
			issues = append(issues, gin.H{
				"when":         d.When,
				"summary":      firstN(d.Summary, 120),
				"type":         "hash_mismatch",
				"expected":     expected,
				"actual":       d.Hash,
				"solana_tx":    d.SolanaTx,
				"city_id":      d.CityID,
				"country_code": d.CountryCode,
			})
		}

		tx := strings.TrimSpace(d.SolanaTx)
		if tx == "" {
			continue
		}
		if strings.HasPrefix(tx, "dev-stub-") {
			stubbed++
			continue
		}
		committed++
	}

	c.JSON(http.StatusOK, gin.H{
		"city_id":         activeCity.CityID,
		"city_name":       activeCity.CityName,
		"country_code":    activeCity.CountryCode,
		"checked":         len(decisions),
		"committed_count": committed,
		"stubbed_count":   stubbed,
		"hash_mismatch":   hashMismatch,
		"ok":              hashMismatch == 0,
		"issues":          issues,
	})
}

func (h *Handlers) AIStatus(c *gin.Context) {
	mode, lastErr, lastAt, configured, model := h.ai.Status()
	status := "local"
	if mode == "cloud_active" {
		status = "cloud"
	}
	c.JSON(http.StatusOK, gin.H{
		"status":      status,
		"mode":        mode,
		"configured":  configured,
		"model":       model,
		"last_error":  firstN(lastErr, 280),
		"last_checked": lastAt,
	})
}

func (h *Handlers) RiskSources(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
	defer cancel()

	filter := h.cityFilter(activeCity.CityID)
	filter["node_id"] = "public-risk-signals"

	var latest models.Telemetry
	if err := db.TelemetryCol.FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "ts", Value: -1}})).Decode(&latest); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "risk signals not available yet",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"city_id":      activeCity.CityID,
		"city_name":    activeCity.CityName,
		"country_code": activeCity.CountryCode,
		"updated":      latest.Ts,
		"signals":      latest.Metrics,
	})
}

func (h *Handlers) RefreshAdvisory(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	var req struct {
		ForceAudio bool `json:"force_audio"`
	}
	_ = c.ShouldBindJSON(&req)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	if err := h.RunCityCycle(ctx, activeCity); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	var latest models.Decision
	if err := db.DecisionsCol.FindOne(ctx, h.cityFilter(activeCity.CityID), options.FindOne().SetSort(bson.D{{Key: "when", Value: -1}})).Decode(&latest); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no advisory available"})
		return
	}

	if req.ForceAudio && h.worker != nil {
		text := advisorySpeechText(latest.Summary, latest.Forecast)
		if audioURL, err := h.worker.Speak(ctx, text); err == nil && strings.TrimSpace(audioURL) != "" {
			latest.AudioURL = audioURL
			_, _ = db.DecisionsCol.UpdateOne(ctx, bson.M{
				"city_id": latest.CityID,
				"when":    latest.When,
				"summary": latest.Summary,
			}, bson.M{"$set": bson.M{"audio_url": audioURL}})
		}
	}

	c.JSON(http.StatusOK, gin.H{"advisory": latest})
}

// PublicFeeds aggregates city-scoped public feeds from official/open providers.
func (h *Handlers) PublicFeeds(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	if cityID := strings.TrimSpace(c.Query("city_id")); cityID != "" && cityID != activeCity.CityID {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var city models.CitySelection
		if err := db.CitySessionsCol.FindOne(ctx, bson.M{"principal_id": principalID, "city_id": cityID}).Decode(&city); err == nil {
			activeCity = city
		}
	}
	if activeCity.Lat == 0 && activeCity.Lon == 0 {
		activeCity.Lat = h.cfg.IngestLat
		activeCity.Lon = h.cfg.IngestLon
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339)
	feeds := make([]gin.H, 0, 6)

	if weather, err := h.fetchOpenMeteoCurrent(ctx, activeCity.Lat, activeCity.Lon); err == nil {
		feeds = append(feeds, gin.H{
			"id":         "weather-current",
			"kind":       "weather",
			"source":     "Open-Meteo",
			"title":      "Current weather conditions",
			"value":      weather,
			"updated_at": now,
			"links": []gin.H{
				{"label": "Open-Meteo", "url": "https://open-meteo.com/"},
			},
		})
	}

	if quakes, err := h.fetchUSGSNearbyQuakes(ctx, activeCity.Lat, activeCity.Lon); err == nil {
		feeds = append(feeds, gin.H{
			"id":         "seismic-nearby",
			"kind":       "seismic",
			"source":     "USGS",
			"title":      "Recent nearby seismic events",
			"items":      quakes,
			"updated_at": now,
			"links": []gin.H{
				{"label": "USGS Earthquake Feed", "url": "https://earthquake.usgs.gov/earthquakes/feed/"},
			},
		})
	}

	if crimeCount, crimeSource, crimeWindow, okCrime := h.fetchOfficialCrimeSignal(ctx, activeCity); okCrime {
		feeds = append(feeds, gin.H{
			"id":         "crime-official",
			"kind":       "crime",
			"source":     crimeSource,
			"title":      "Official crime incident volume",
			"value":      fmt.Sprintf("%d events (%s)", crimeCount, crimeWindow),
			"updated_at": now,
		})
	}

	if strings.EqualFold(activeCity.CountryCode, "US") && strings.Contains(strings.ToLower(activeCity.CityName), "new york") {
		if count, err := h.fetchNYC311Count(ctx); err == nil {
			feeds = append(feeds, gin.H{
				"id":         "nyc-311",
				"kind":       "civic",
				"source":     "NYC Open Data",
				"title":      "NYC 311 recent complaint sample",
				"value":      fmt.Sprintf("%d complaints in latest sample window", count),
				"updated_at": now,
				"links": []gin.H{
					{"label": "NYC 311 API", "url": "https://data.cityofnewyork.us/resource/fhrw-4uyv.json"},
				},
			})
		}
	}

	feeds = append(feeds, gin.H{
		"id":         "official-portals",
		"kind":       "civic",
		"source":     "Official portals",
		"title":      "City operations and open-data links",
		"updated_at": now,
		"links":      h.officialCityLinks(activeCity),
	})

	c.JSON(http.StatusOK, gin.H{
		"city":       activeCity,
		"updated_at": now,
		"feeds":      feeds,
	})
}

func (h *Handlers) fetchOpenMeteoCurrent(ctx context.Context, lat, lon float64) (string, error) {
	u := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code&timezone=auto",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("open-meteo: %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Current struct {
			TempC      float64 `json:"temperature_2m"`
			Humidity   float64 `json:"relative_humidity_2m"`
			WindSpeed  float64 `json:"wind_speed_10m"`
			WeatherCode int    `json:"weather_code"`
		} `json:"current"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return fmt.Sprintf("%.1f C, %.0f%% humidity, %.1f m/s wind, weather code %d",
		payload.Current.TempC, payload.Current.Humidity, payload.Current.WindSpeed, payload.Current.WeatherCode), nil
}

func (h *Handlers) fetchUSGSNearbyQuakes(ctx context.Context, lat, lon float64) ([]string, error) {
	u := fmt.Sprintf(
		"https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&orderby=time&limit=5&latitude=%.4f&longitude=%.4f&maxradiuskm=300",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usgs: %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Features []struct {
			Properties struct {
				Mag  float64 `json:"mag"`
				Place string `json:"place"`
				Time int64   `json:"time"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload.Features) == 0 {
		return []string{"No recent events in configured radius."}, nil
	}
	out := make([]string, 0, len(payload.Features))
	for _, feature := range payload.Features {
		dist := 0.0
		if len(feature.Geometry.Coordinates) >= 2 {
			dist = haversineKm(lat, lon, feature.Geometry.Coordinates[1], feature.Geometry.Coordinates[0])
		}
		when := time.UnixMilli(feature.Properties.Time).UTC().Format("2006-01-02 15:04 UTC")
		out = append(out, fmt.Sprintf("M%.1f · %.0f km · %s · %s", feature.Properties.Mag, dist, when, feature.Properties.Place))
	}
	return out, nil
}

func (h *Handlers) fetchNYC311Count(ctx context.Context) (int, error) {
	u := "https://data.cityofnewyork.us/resource/fhrw-4uyv.json?$limit=30&$order=created_date%20DESC&$select=complaint_type"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("nyc311: %s", resp.Status)
	}
	var rows []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (h *Handlers) officialCityLinks(city models.CitySelection) []gin.H {
	registry := map[string][]gin.H{
		"new york|US": {
			{"label": "NYC Open Data", "url": "https://opendata.cityofnewyork.us/"},
			{"label": "NYC DOT Traffic Cameras", "url": "https://nyctmc.org/cameras"},
			{"label": "NYC Emergency Management", "url": "https://www.nyc.gov/site/em/index.page"},
		},
		"los angeles|US": {
			{"label": "LA Open Data", "url": "https://data.lacity.org/"},
			{"label": "LADOT Traffic Info", "url": "https://ladot.lacity.gov/"},
			{"label": "LAFD Alerts", "url": "https://www.lafd.org/alerts"},
		},
		"chicago|US": {
			{"label": "Chicago Data Portal", "url": "https://data.cityofchicago.org/"},
			{"label": "Chicago OEMC", "url": "https://www.chicago.gov/city/en/depts/oem.html"},
		},
		"london|GB": {
			{"label": "London Datastore", "url": "https://data.london.gov.uk/"},
			{"label": "TfL Status", "url": "https://tfl.gov.uk/tube-dlr-overground/status/"},
			{"label": "London Resilience", "url": "https://www.london.gov.uk/programmes-strategies/london-resilience"},
		},
		"paris|FR": {
			{"label": "Paris Open Data", "url": "https://opendata.paris.fr/"},
			{"label": "Prefecture de Police Alerts", "url": "https://www.prefecturedepolice.interieur.gouv.fr/"},
		},
		"tokyo|JP": {
			{"label": "Tokyo Open Data", "url": "https://portal.data.metro.tokyo.lg.jp/"},
			{"label": "Tokyo Disaster Prevention", "url": "https://www.bousai.metro.tokyo.lg.jp/"},
		},
		"seoul|KR": {
			{"label": "Seoul Open Data Plaza", "url": "https://data.seoul.go.kr/"},
			{"label": "Seoul Disaster Safety", "url": "https://safecity.seoul.go.kr/"},
		},
		"singapore|SG": {
			{"label": "Data.gov.sg", "url": "https://data.gov.sg/"},
			{"label": "LTA Traffic Updates", "url": "https://www.lta.gov.sg/content/ltagov/en/map/traffic-news.html"},
		},
		"sydney|AU": {
			{"label": "NSW Open Data", "url": "https://www.nsw.gov.au/departments-and-agencies/dcs/open-data"},
			{"label": "Transport NSW Live Traffic", "url": "https://www.livetraffic.com/"},
		},
		"toronto|CA": {
			{"label": "Toronto Open Data", "url": "https://open.toronto.ca/"},
			{"label": "Toronto Emergency", "url": "https://www.toronto.ca/community-people/public-safety-alerts/emergency-preparedness/"},
		},
	}
	key := fmt.Sprintf("%s|%s", strings.ToLower(strings.TrimSpace(city.CityName)), strings.ToUpper(strings.TrimSpace(city.CountryCode)))
	if links, ok := registry[key]; ok {
		return links
	}
	return []gin.H{
		{"label": "OpenStreetMap", "url": "https://www.openstreetmap.org/"},
		{"label": "Open-Meteo", "url": "https://open-meteo.com/"},
		{"label": "USGS Earthquake Feed", "url": "https://earthquake.usgs.gov/earthquakes/feed/"},
	}
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return r * c
}

// SearchCities provides global city lookup using Open-Meteo geocoding.
func (h *Handlers) SearchCities(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q must be at least 2 characters"})
		return
	}
	u := "https://geocoding-api.open-meteo.com/v1/search?count=8&language=en&format=json&name=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "city provider unavailable"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "city provider error"})
		return
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Results []struct {
			ID          int64   `json:"id"`
			Name        string  `json:"name"`
			CountryCode string  `json:"country_code"`
			Country     string  `json:"country"`
			Admin1      string  `json:"admin1"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid city response"})
		return
	}
	out := make([]gin.H, 0, len(payload.Results))
	for _, r := range payload.Results {
		cityID := fmt.Sprintf("%d", r.ID)
		if cityID == "0" {
			cityID = strings.ToLower(fmt.Sprintf("%s-%s", strings.ReplaceAll(r.Name, " ", "-"), r.CountryCode))
		}
		out = append(out, gin.H{
			"city_id":      cityID,
			"city_name":    r.Name,
			"country_code": r.CountryCode,
			"country":      r.Country,
			"region":       r.Admin1,
			"lat":          r.Latitude,
			"lon":          r.Longitude,
		})
	}
	c.JSON(http.StatusOK, gin.H{"cities": out})
}

func (h *Handlers) GetSessionCity(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	city, err := h.getActiveCity(c.Request.Context(), principalID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load city"})
		return
	}
	c.JSON(http.StatusOK, city)
}

func (h *Handlers) SetSessionCity(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	var req models.CitySelection
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.CityID == "" || req.CityName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "city_id and city_name are required"})
		return
	}
	req.PrincipalID = principalID
	req.UpdatedAt = time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	_, err := db.CitySessionsCol.UpdateOne(ctx,
		bson.M{"principal_id": principalID},
		bson.M{"$set": req},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save city"})
		return
	}
	go func(city models.CitySelection) {
		bg, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = h.RunCityCycle(bg, city)
	}(req)
	c.JSON(http.StatusOK, req)
}

func (h *Handlers) CreateChatThread(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
	if qCityID := strings.TrimSpace(c.Query("city_id")); qCityID != "" && qCityID != activeCity.CityID {
		activeCity = models.CitySelection{
			CityID:      qCityID,
			CityName:    strings.TrimSpace(c.Query("city_name")),
			CountryCode: strings.TrimSpace(c.Query("country_code")),
		}
		if activeCity.CityName == "" {
			activeCity.CityName = "Active City"
		}
	}
	var req struct {
		Title string `json:"title"`
	}
	_ = c.ShouldBindJSON(&req)
	now := time.Now()
	thread := models.ChatThread{
		ID:          h.newID("thr"),
		PrincipalID: principalID,
		Title:       strings.TrimSpace(req.Title),
		CityID:      activeCity.CityID,
		CityName:    activeCity.CityName,
		CountryCode: activeCity.CountryCode,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if thread.Title == "" {
		thread.Title = "New thread"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := db.ChatThreadsCol.InsertOne(ctx, thread); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create thread"})
		return
	}
	c.JSON(http.StatusOK, thread)
}

func (h *Handlers) DeleteChatThread(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	threadID := strings.TrimSpace(c.Param("id"))
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "thread id is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	delThread, err := db.ChatThreadsCol.DeleteOne(ctx, bson.M{"id": threadID, "principal_id": principalID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete thread"})
		return
	}
	if delThread.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
		return
	}
	delMsgs, _ := db.ChatMessagesCol.DeleteMany(ctx, bson.M{"thread_id": threadID})
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted_messages": delMsgs.DeletedCount})
}

func (h *Handlers) ListChatThreads(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	cityID := strings.TrimSpace(c.Query("city_id"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	filter := bson.M{"principal_id": principalID}
	if cityID != "" {
		filter["city_id"] = cityID
	}
	cur, err := db.ChatThreadsCol.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(50))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list threads"})
		return
	}
	var threads []models.ChatThread
	if err := cur.All(ctx, &threads); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode threads"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"threads": threads})
}

func (h *Handlers) GetChatMessages(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	threadID := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := db.ChatThreadsCol.FindOne(ctx, bson.M{"id": threadID, "principal_id": principalID}).Err(); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
		return
	}
	cur, err := db.ChatMessagesCol.Find(ctx, bson.M{"thread_id": threadID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(400))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read messages"})
		return
	}
	var messages []models.ChatMessage
	if err := cur.All(ctx, &messages); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode messages"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *Handlers) PostChatMessage(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	threadID := c.Param("id")
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	var thread models.ChatThread
	if err := db.ChatThreadsCol.FindOne(ctx, bson.M{"id": threadID, "principal_id": principalID}).Decode(&thread); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "thread not found"})
		return
	}

	userMsg := models.ChatMessage{
		ID:        h.newID("msg"),
		ThreadID:  threadID,
		Role:      "user",
		Content:   strings.TrimSpace(req.Content),
		CreatedAt: time.Now(),
	}
	if _, err := db.ChatMessagesCol.InsertOne(ctx, userMsg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message"})
		return
	}

	cur, err := db.ChatMessagesCol.Find(ctx, bson.M{"thread_id": threadID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(8))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load context"})
		return
	}
	var recent []models.ChatMessage
	if err := cur.All(ctx, &recent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode context"})
		return
	}
	msgs := make([]string, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		msgs = append(msgs, fmt.Sprintf("%s: %s", recent[i].Role, recent[i].Content))
	}
	cityName := strings.TrimSpace(thread.CityName)
	if cityName == "" {
		cityName = "Active City"
	}
	aiText, err := h.ai.Chat(ctx, cityName, msgs, userMsg.Content)
	if err != nil {
		fmt.Printf("ai chat fallback for city=%s thread=%s: %v\n", cityName, threadID, err)
		aiText = h.localChatFallback(ctx, thread.CityID, cityName, userMsg.Content)
	}
	assistant := models.ChatMessage{
		ID:        h.newID("msg"),
		ThreadID:  threadID,
		Role:      "assistant",
		Content:   aiText,
		CreatedAt: time.Now(),
	}
	if _, err := db.ChatMessagesCol.InsertOne(ctx, assistant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save assistant message"})
		return
	}
	_, _ = db.ChatThreadsCol.UpdateOne(ctx, bson.M{"id": threadID, "principal_id": principalID}, bson.M{"$set": bson.M{"updated_at": assistant.CreatedAt}})
	c.JSON(http.StatusOK, gin.H{"user": userMsg, "assistant": assistant})
}

func (h *Handlers) localChatFallback(ctx context.Context, cityID, cityName, userInput string) string {
	summaries := h.getRecentDecisionSummaries(ctx, cityID, 2)
	if len(summaries) == 0 {
		return fmt.Sprintf("Monitoring %s in local mode. I can help with operational checklists and triage, but deeper AI analysis is currently unavailable. Start with: verify telemetry freshness, check transport chokepoints, and validate emergency comms readiness.", cityName)
	}
	return fmt.Sprintf(
		"Local mode response for %s: recent advisory says \"%s\". Based on your request (\"%s\"), prioritize one conservative step and one aggressive step, then reassess in 10-15 minutes.",
		cityName,
		firstN(summaries[0], 180),
		firstN(strings.TrimSpace(userInput), 120),
	)
}

// RunCityCycle ingests fresh city telemetry and writes a predictive advisory.
func (h *Handlers) RunCityCycle(ctx context.Context, city models.CitySelection) error {
	if err := ingest.RunForCity(ctx, h.cfg, city); err != nil {
		return err
	}
	signals, signalScore := h.collectRiskSignals(ctx, city)
	if len(signals) > 0 {
		doc := models.Telemetry{
			NodeID:      "public-risk-signals",
			Ts:          time.Now(),
			Loc:         models.Location{Lat: city.Lat, Lon: city.Lon},
			Metrics:     signals,
			Signature:   "server-ingest",
			CityID:      city.CityID,
			CityName:    city.CityName,
			CountryCode: city.CountryCode,
		}
		_, _ = db.TelemetryCol.InsertOne(ctx, doc)
	}
	telemetrySummary, recentDecisions, err := h.cityReasonContext(ctx, city.CityID)
	if err != nil {
		return err
	}
	if len(signals) > 0 {
		if b, err := json.Marshal(signals); err == nil {
			telemetrySummary = telemetrySummary + "\n\nPublic safety signals:\n" + string(b)
		}
	}
	resp, err := h.ai.Reason(ctx, city.CityName, telemetrySummary, recentDecisions)
	if err != nil {
		return err
	}
	resp = h.enforceGroundedAdvisory(city, resp, signals, signalScore)
	if resp.RiskScore <= 0 && signalScore > 0 {
		resp.RiskScore = signalScore
	}
	if resp.RiskScore <= 0 {
		resp.RiskScore = h.scoreFromRiskLabel(resp.Risk)
	}
	audioURL := ""
	if resp.AudioText != "" && h.worker != nil {
		audioURL, _ = h.worker.Speak(ctx, advisorySpeechText(resp.Summary, resp.Forecast))
	}
	return h.saveAdvisory(ctx, city, resp, audioURL, "auto_cycle")
}

func (h *Handlers) cityReasonContext(ctx context.Context, cityID string) (string, []string, error) {
	cur, err := db.TelemetryCol.Find(ctx, h.cityFilter(cityID),
		options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}).SetLimit(12))
	if err != nil {
		return "", nil, err
	}
	var t []models.Telemetry
	if err := cur.All(ctx, &t); err != nil {
		return "", nil, err
	}
	summary := "No recent telemetry."
	if len(t) > 0 {
		b, _ := json.Marshal(t)
		summary = string(b)
	}
	dcur, err := db.DecisionsCol.Find(ctx, h.cityFilter(cityID), options.Find().SetSort(bson.D{{Key: "when", Value: -1}}).SetLimit(8))
	if err != nil {
		return "", nil, err
	}
	var decisions []models.Decision
	if err := dcur.All(ctx, &decisions); err != nil {
		return "", nil, err
	}
	recentDecisions := make([]string, 0, minInt(5, len(decisions)))
	for i := 0; i < len(decisions) && i < 5; i++ {
		if s := strings.TrimSpace(decisions[i].Summary); s != "" {
			recentDecisions = append(recentDecisions, s)
		}
	}
	if pred := buildPredictiveContext(t, decisions); len(pred) > 0 {
		if b, err := json.Marshal(pred); err == nil {
			summary = summary + "\n\nPredictive context:\n" + string(b)
		}
	}
	return summary, recentDecisions, nil
}

func buildPredictiveContext(telemetry []models.Telemetry, decisions []models.Decision) map[string]any {
	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if len(telemetry) > 0 {
		latest := telemetry[0].Ts
		nodes := map[string]struct{}{}
		for _, point := range telemetry {
			if point.NodeID != "" {
				nodes[point.NodeID] = struct{}{}
			}
		}
		out["telemetry_points"] = len(telemetry)
		out["active_nodes"] = len(nodes)
		out["freshness_minutes"] = int(time.Since(latest).Minutes())
	}
	scores := make([]int, 0, len(decisions))
	recent6h := 0
	cut := time.Now().Add(-6 * time.Hour)
	for _, d := range decisions {
		if d.RiskScore > 0 {
			scores = append(scores, d.RiskScore)
		}
		if d.When.After(cut) {
			recent6h++
		}
	}
	if len(scores) > 0 {
		last := scores
		if len(last) > 6 {
			last = last[:6]
		}
		out["recent_risk_scores"] = last
		out["risk_trend"] = riskTrend(last)
	}
	out["advisories_last_6h"] = recent6h
	return out
}

func riskTrend(scores []int) string {
	if len(scores) < 4 {
		return "insufficient_data"
	}
	n := len(scores)
	newer := avgInts(scores[:n/2])
	older := avgInts(scores[n/2:])
	delta := newer - older
	switch {
	case delta >= 8:
		return "rising"
	case delta <= -8:
		return "falling"
	default:
		return "stable"
	}
}

func avgInts(v []int) int {
	if len(v) == 0 {
		return 0
	}
	sum := 0
	for _, x := range v {
		sum += x
	}
	return int(math.Round(float64(sum) / float64(len(v))))
}

func (h *Handlers) saveAdvisory(ctx context.Context, city models.CitySelection, resp ai.ReasonResult, audioURL, source string) error {
	if strings.TrimSpace(resp.Summary) == "" {
		return nil
	}
	// Suppress duplicates when advisory text is unchanged in a short window.
	var last models.Decision
	if err := db.DecisionsCol.FindOne(ctx, h.cityFilter(city.CityID), options.FindOne().SetSort(bson.D{{Key: "when", Value: -1}})).Decode(&last); err == nil {
		if strings.EqualFold(strings.TrimSpace(last.Summary), strings.TrimSpace(resp.Summary)) && time.Since(last.When) < 8*time.Minute {
			return nil
		}
	}
	doc := models.Decision{
		When:        time.Now(),
		Summary:     resp.Summary,
		Hash:        auth.HashForSolana(resp.Summary + "|" + city.CityID + "|" + source),
		AudioURL:    audioURL,
		SolanaTx:    "",
		Risk:        strings.ToLower(strings.TrimSpace(resp.Risk)),
		RiskScore:   resp.RiskScore,
		Actions:     resp.Actions,
		Forecast:    resp.Forecast,
		Confidence:  resp.Confidence,
		Explain:     resp.Explain,
		Source:      source,
		CityID:      city.CityID,
		CityName:    city.CityName,
		CountryCode: city.CountryCode,
	}
	_, err := db.DecisionsCol.InsertOne(ctx, doc)
	return err
}

func advisorySpeechText(summary, forecast string) string {
	s := strings.TrimSpace(summary)
	f := strings.TrimSpace(forecast)
	if s == "" && f == "" {
		return "City advisory update: conditions are being monitored. Please review dashboard telemetry for current status."
	}
	if f == "" {
		return s
	}
	return fmt.Sprintf("%s Forecast: %s", s, f)
}

func (h *Handlers) getRecentDecisionSummaries(ctx context.Context, cityID string, limit int64) []string {
	cur, err := db.DecisionsCol.Find(ctx, h.cityFilter(cityID), options.Find().SetSort(bson.D{{Key: "when", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil
	}
	var decisions []models.Decision
	if err := cur.All(ctx, &decisions); err != nil {
		return nil
	}
	out := make([]string, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, d.Summary)
	}
	return out
}

func (h *Handlers) newID(prefix string) string {
	r := make([]byte, 4)
	_, _ = rand.Read(r)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixNano(), base64.RawURLEncoding.EncodeToString(r))
}

func (h *Handlers) getPrincipalID(c *gin.Context) string {
	v, ok := c.Get("claims")
	if !ok {
		return "public"
	}
	claims, ok := v.(*auth.Claims)
	if !ok || claims.Sub == "" {
		return "public"
	}
	return claims.Sub
}

func (h *Handlers) defaultCity() models.CitySelection {
	return models.CitySelection{
		CityID:      "default-city",
		CityName:    "Default City",
		CountryCode: "UN",
		Lat:         h.cfg.IngestLat,
		Lon:         h.cfg.IngestLon,
	}
}

func (h *Handlers) getActiveCity(ctx context.Context, principalID string) (models.CitySelection, error) {
	if principalID == "" || principalID == "public" {
		return h.defaultCity(), nil
	}
	var city models.CitySelection
	err := db.CitySessionsCol.FindOne(ctx, bson.M{"principal_id": principalID}).Decode(&city)
	if err != nil {
		return h.defaultCity(), nil
	}
	return city, nil
}

func (h *Handlers) cityFromRequest(c *gin.Context) models.CitySelection {
	cityID := strings.TrimSpace(c.Query("city_id"))
	if cityID == "" {
		return h.defaultCity()
	}
	return models.CitySelection{
		CityID:      cityID,
		CityName:    strings.TrimSpace(c.Query("city_name")),
		CountryCode: strings.TrimSpace(c.Query("country_code")),
	}
}

func (h *Handlers) cityFilter(cityID string) bson.M {
	if cityID == "" || cityID == "default-city" {
		return bson.M{"$or": []bson.M{{"city_id": cityID}, {"city_id": bson.M{"$exists": false}}}}
	}
	return bson.M{"city_id": cityID}
}

func (h *Handlers) collectRiskSignals(ctx context.Context, city models.CitySelection) (map[string]any, int) {
	sources := []string{}
	components := map[string]int{}
	coverage := map[string]bool{
		"travel":   false,
		"disaster": false,
		"crime":    false,
		"conflict": false,
	}
	details := map[string]any{
		"city":         city.CityName,
		"country_code": strings.ToUpper(strings.TrimSpace(city.CountryCode)),
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}

	travelScore, travelLevel, travelText, okTravel := h.fetchTravelAdvisory(ctx, city.CountryCode)
	if okTravel {
		components["travel"] = travelScore
		details["travel_level"] = travelLevel
		details["travel_advisory"] = travelText
		sources = append(sources, "travel-advisory.info")
		coverage["travel"] = true
	}

	disasterEvents, quakeCount, okDisaster := h.fetchDisasterSignals(ctx, city)
	if okDisaster {
		components["disaster"] = minInt(100, disasterEvents*18+quakeCount*12)
		details["disaster_events_30d"] = disasterEvents
		details["earthquakes_7d"] = quakeCount
		sources = append(sources, "nasa-eonet", "usgs")
		coverage["disaster"] = true
	}

	crimeCount7d, crimeSource, crimeWindow, okCrime := h.fetchOfficialCrimeSignal(ctx, city)
	if okCrime {
		components["crime"] = minInt(100, int(math.Round(math.Log1p(float64(maxInt(0, crimeCount7d)))*14)))
		details["crime_events_window"] = crimeCount7d
		details["crime_window"] = crimeWindow
		details["crime_source"] = crimeSource
		sources = append(sources, crimeSource)
		coverage["crime"] = true
	}

	conflictMentions, okConflict := h.fetchGDELTMentions(ctx, fmt.Sprintf(`"%s" AND (conflict OR terror OR military OR unrest OR riot OR war)`, city.CityName))
	if okConflict {
		components["conflict"] = minInt(100, conflictMentions*2)
		details["conflict_mentions_72h_osint"] = conflictMentions
		sources = append(sources, "gdelt")
		coverage["conflict"] = true
	}

	score := h.weightedRiskScore(components)
	if score == 0 {
		return nil, 0
	}
	details["components"] = components
	details["coverage"] = coverage
	details["sources"] = dedupeStrings(sources)
	details["score"] = score
	return map[string]any{
		"risk_signal_score": score,
		"risk_signal_label": h.labelFromScore(score),
		"risk_signals":      details,
	}, score
}

func (h *Handlers) fetchTravelAdvisory(ctx context.Context, countryCode string) (score int, level string, advisory string, ok bool) {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	if len(cc) < 2 {
		return 0, "", "", false
	}
	u := "https://www.travel-advisory.info/api?countrycode=" + url.QueryEscape(cc)
	body, err := h.fetchJSON(ctx, u)
	if err != nil {
		return 0, "", "", false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, "", "", false
	}
	data, _ := payload["data"].(map[string]any)
	entry, _ := data[cc].(map[string]any)
	advisoryObj, _ := entry["advisory"].(map[string]any)
	rawScore := toFloat(advisoryObj["score"])
	if rawScore <= 0 {
		return 0, "", "", false
	}
	return minInt(100, int(math.Round(rawScore*20))), fmt.Sprintf("%.1f/5", rawScore), strings.TrimSpace(fmt.Sprint(advisoryObj["message"])), true
}

func (h *Handlers) fetchDisasterSignals(ctx context.Context, city models.CitySelection) (events int, quakeCount int, ok bool) {
	const eonetURL = "https://eonet.gsfc.nasa.gov/api/v3/events?status=open&days=30&limit=150"
	body, err := h.fetchJSON(ctx, eonetURL)
	if err == nil {
		var payload struct {
			Events []struct {
				Geometry []struct {
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
			} `json:"events"`
		}
		if json.Unmarshal(body, &payload) == nil {
			for _, ev := range payload.Events {
				for _, g := range ev.Geometry {
					if len(g.Coordinates) >= 2 {
						lon := g.Coordinates[0]
						lat := g.Coordinates[1]
						if haversineKm(city.Lat, city.Lon, lat, lon) <= 600 {
							events++
							break
						}
					}
				}
			}
		}
	}

	quakeBody, quakeErr := h.fetchJSON(ctx, fmt.Sprintf(
		"https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&latitude=%.4f&longitude=%.4f&maxradiuskm=300&minmagnitude=3.5&orderby=time&starttime=%s",
		city.Lat,
		city.Lon,
		time.Now().Add(-7*24*time.Hour).UTC().Format("2006-01-02"),
	))
	if quakeErr == nil {
		var payload struct {
			Features []json.RawMessage `json:"features"`
		}
		if json.Unmarshal(quakeBody, &payload) == nil {
			quakeCount = len(payload.Features)
		}
	}
	return events, quakeCount, events > 0 || quakeCount > 0
}

func (h *Handlers) fetchOfficialCrimeSignal(ctx context.Context, city models.CitySelection) (count7d int, source string, window string, ok bool) {
	cityKey := strings.ToLower(strings.TrimSpace(city.CityName))
	cc := strings.ToUpper(strings.TrimSpace(city.CountryCode))
	switch {
	case cc == "US" && strings.Contains(cityKey, "new york"):
		n, err := h.fetchNYCCrimeRecent(ctx)
		if err != nil {
			return 0, "", "", false
		}
		return n, "nyc-open-data-nypd", "latest-500-records", true
	case cc == "US" && strings.Contains(cityKey, "chicago"):
		n, err := h.fetchChicagoCrimeRecent(ctx)
		if err != nil {
			return 0, "", "", false
		}
		return n, "chicago-open-data", "last-7-days", true
	case cc == "GB":
		n, err := h.fetchUKPoliceCrimeRecent(ctx, city.Lat, city.Lon)
		if err != nil {
			return 0, "", "", false
		}
		return n, "uk-police-data", "last-month", true
	default:
		return 0, "", "", false
	}
}

func (h *Handlers) fetchNYCCrimeRecent(ctx context.Context) (int, error) {
	u := "https://data.cityofnewyork.us/resource/qgea-i56i.json?$limit=500&$order=cmplnt_fr_dt%20DESC"
	body, err := h.fetchJSON(ctx, u)
	if err != nil {
		return 0, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (h *Handlers) fetchChicagoCrimeRecent(ctx context.Context) (int, error) {
	start := time.Now().Add(-7 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05")
	where := url.QueryEscape(fmt.Sprintf("date >= '%s'", start))
	u := "https://data.cityofchicago.org/resource/ijzp-q8t2.json?$select=count(*)&$where=" + where
	body, err := h.fetchJSON(ctx, u)
	if err != nil {
		return 0, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int(math.Round(toFloat(rows[0]["count"]))), nil
}

func (h *Handlers) fetchUKPoliceCrimeRecent(ctx context.Context, lat, lon float64) (int, error) {
	if lat == 0 && lon == 0 {
		return 0, fmt.Errorf("missing coordinates")
	}
	month := time.Now().AddDate(0, -1, 0).Format("2006-01")
	u := fmt.Sprintf("https://data.police.uk/api/crimes-street/all-crime?lat=%.4f&lng=%.4f&date=%s", lat, lon, month)
	body, err := h.fetchJSON(ctx, u)
	if err != nil {
		return 0, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (h *Handlers) fetchGDELTMentions(ctx context.Context, query string) (int, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, false
	}
	u := "https://api.gdeltproject.org/api/v2/doc/doc?mode=ArtList&format=json&maxrecords=60&query=" + url.QueryEscape(query+" AND sourcelang:english")
	body, err := h.fetchJSON(ctx, u)
	if err != nil {
		return 0, false
	}
	var payload struct {
		Articles []json.RawMessage `json:"articles"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, false
	}
	return len(payload.Articles), true
}

func (h *Handlers) fetchJSON(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func (h *Handlers) weightedRiskScore(components map[string]int) int {
	if len(components) == 0 {
		return 0
	}
	weights := map[string]float64{
		"travel":   0.30,
		"disaster": 0.30,
		"crime":    0.20,
		"conflict": 0.20,
	}
	totalWeight := 0.0
	acc := 0.0
	for k, v := range components {
		w := weights[k]
		if w <= 0 {
			continue
		}
		totalWeight += w
		acc += float64(v) * w
	}
	if totalWeight <= 0 {
		return 0
	}
	return minInt(100, maxInt(0, int(math.Round(acc/totalWeight))))
}

func (h *Handlers) scoreFromRiskLabel(label string) int {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "critical":
		return 90
	case "high":
		return 75
	case "medium":
		return 55
	case "low":
		return 30
	default:
		return 0
	}
}

func (h *Handlers) labelFromScore(score int) string {
	switch {
	case score >= 85:
		return "critical"
	case score >= 65:
		return "high"
	case score >= 40:
		return "medium"
	default:
		return "low"
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		k := strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *Handlers) enforceGroundedAdvisory(city models.CitySelection, resp ai.ReasonResult, signals map[string]any, signalScore int) ai.ReasonResult {
	if signalScore > 0 {
		if resp.RiskScore > 0 {
			resp.RiskScore = int(math.Round(float64(signalScore)*0.75 + float64(resp.RiskScore)*0.25))
		} else {
			resp.RiskScore = signalScore
		}
		resp.RiskScore = minInt(100, maxInt(0, resp.RiskScore))
		resp.Risk = h.labelFromScore(resp.RiskScore)
	}
	quakeCount := signalDetailInt(signals, "earthquakes_7d")
	disasterEvents := signalDetailInt(signals, "disaster_events_30d")
	summaryText := strings.TrimSpace(resp.Summary)
	hallucinatedQuake := quakeCount == 0 && strings.Contains(strings.ToLower(summaryText), "earthquake")
	if hallucinatedQuake || looksIncompleteText(summaryText) {
		resp = buildGroundedFallbackAdvisory(city, resp, signalScore, quakeCount, disasterEvents, signalDetailComponents(signals))
	}
	return resp
}

func signalDetailInt(signals map[string]any, key string) int {
	if len(signals) == 0 {
		return 0
	}
	detail, _ := signals["risk_signals"].(map[string]any)
	if detail == nil {
		return 0
	}
	return int(math.Round(toFloat(detail[key])))
}

func signalDetailComponents(signals map[string]any) map[string]int {
	out := map[string]int{"travel": 0, "disaster": 0, "crime": 0, "conflict": 0}
	if len(signals) == 0 {
		return out
	}
	detail, _ := signals["risk_signals"].(map[string]any)
	if detail == nil {
		return out
	}
	raw, _ := detail["components"].(map[string]any)
	for k := range out {
		out[k] = int(math.Round(toFloat(raw[k])))
	}
	return out
}

func looksIncompleteText(s string) bool {
	text := strings.TrimSpace(strings.ToLower(s))
	if len(text) < 40 {
		return true
	}
	badEnds := []string{" with", " with.", " and", " and.", " of", " of.", " in", " in.", " at", " at.", " to", " to.", " for", " for.", " currently", " currently."}
	for _, tail := range badEnds {
		if strings.HasSuffix(text, tail) {
			return true
		}
	}
	return false
}

func buildGroundedFallbackAdvisory(city models.CitySelection, prev ai.ReasonResult, signalScore, quakeCount, disasterEvents int, components map[string]int) ai.ReasonResult {
	score := signalScore
	if score <= 0 {
		score = 45
	}
	risk := "medium"
	switch {
	case score >= 85:
		risk = "critical"
	case score >= 65:
		risk = "high"
	case score < 40:
		risk = "low"
	}
	drivers := []string{}
	for _, k := range []string{"disaster", "travel", "crime", "conflict"} {
		if components[k] > 0 {
			drivers = append(drivers, fmt.Sprintf("%s:%d", k, components[k]))
		}
	}
	if len(drivers) == 0 {
		drivers = append(drivers, "limited-signal-data")
	}
	summary := fmt.Sprintf("%s risk index is %d/100 (%s) based on current public signals. Primary drivers are %s.", city.CityName, score, risk, strings.Join(drivers, ", "))
	forecast := "Risk is likely to stay near current levels over the next 3-12 hours unless one of the primary drivers changes materially."
	if quakeCount > 0 || disasterEvents > 0 {
		forecast = fmt.Sprintf("Disaster monitoring is active (%d earthquakes in 7d, %d nearby open events in 30d). Risk may rise in the next 30m-3h if event volume accelerates.", quakeCount, disasterEvents)
	}
	return ai.ReasonResult{
		Summary:   summary,
		Risk:      risk,
		RiskScore: score,
		Actions: map[string]string{
			"conservative": "Increase telemetry watch cadence and verify source freshness.",
			"aggressive":   "Stage response resources in high-risk corridors and publish an operator advisory.",
		},
		Forecast:   forecast,
		Confidence: maxInt(prev.Confidence, 65),
		AudioText:  summary + " " + forecast,
		Explain:    "Grounded fallback used to prevent unsupported or incomplete model output.",
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
