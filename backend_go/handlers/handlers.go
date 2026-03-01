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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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

	recentDecisions := h.getRecentDecisionSummaries(ctx, activeCity.CityID, 5)
	resp, err := h.ai.Reason(ctx, activeCity.CityName, summary, recentDecisions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Optionally generate audio
	audioURL := ""
	if resp.AudioText != "" && h.worker != nil {
		audioURL, _ = h.worker.Speak(ctx, resp.AudioText)
	}

	c.JSON(http.StatusOK, gin.H{
		"summary":    resp.Summary,
		"risk":       resp.Risk,
		"actions":    resp.Actions,
		"audio_text": resp.AudioText,
		"audio_url":  audioURL,
		"explain":    resp.Explain,
	})
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

	txSig, err := h.solana.SubmitMemo(ctx, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	doc := models.Decision{
		When:        time.Now(),
		Summary:     body.Summary,
		Hash:        hash,
		AudioURL:    body.AudioURL,
		SolanaTx:    txSig,
		CityID:      activeCity.CityID,
		CityName:    activeCity.CityName,
		CountryCode: activeCity.CountryCode,
	}
	if _, err := db.DecisionsCol.InsertOne(ctx, doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store decision"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tx": txSig, "hash": hash})
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
	links := []gin.H{
		{
			"label": "Open data portal search",
			"url":   fmt.Sprintf("https://www.google.com/search?q=%s+official+open+data+portal", url.QueryEscape(city.CityName)),
		},
		{
			"label": "Emergency management search",
			"url":   fmt.Sprintf("https://www.google.com/search?q=%s+official+emergency+management", url.QueryEscape(city.CityName)),
		},
	}
	if strings.EqualFold(city.CountryCode, "US") && strings.Contains(strings.ToLower(city.CityName), "new york") {
		links = append(links,
			gin.H{"label": "NYC Open Data", "url": "https://opendata.cityofnewyork.us/"},
			gin.H{"label": "NYC DOT Traffic Cameras", "url": "https://nyctmc.org/cameras"},
		)
	}
	return links
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
		_ = ingest.RunForCity(bg, h.cfg, city)
	}(req)
	c.JSON(http.StatusOK, req)
}

func (h *Handlers) CreateChatThread(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	activeCity, _ := h.getActiveCity(c.Request.Context(), principalID)
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

func (h *Handlers) ListChatThreads(c *gin.Context) {
	principalID := h.getPrincipalID(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	cur, err := db.ChatThreadsCol.Find(ctx, bson.M{"principal_id": principalID}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}).SetLimit(50))
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
	aiText, err := h.ai.Chat(ctx, thread.CityName, msgs, userMsg.Content)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
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
