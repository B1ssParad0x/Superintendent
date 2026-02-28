package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"superintendent/backend/auth"
	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/models"
	"superintendent/backend/solana"
	"superintendent/backend/worker"
)

type Handlers struct {
	cfg    *config.Config
	worker *worker.Client
	solana *solana.Client
}

func New(cfg *config.Config) (*Handlers, error) {
	w := worker.New(cfg.AIWorkerURL)
	sc, err := solana.New(cfg.SolanaRPC, cfg.SolanaKeypair)
	if err != nil {
		return nil, err
	}
	return &Handlers{cfg: cfg, worker: w, solana: sc}, nil
}

// Ingest handles POST /api/ingest - verify edge signature, store, enqueue AI
func (h *Handlers) Ingest(c *gin.Context) {
	var req models.IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Edge verification: require X-Edge-Key header when EDGE_API_KEY is set.
	// In production, verify Ed25519 signature against registered edge pubkey per node_id.
	if h.cfg.EdgeAPIKey != "" {
		if c.GetHeader("X-Edge-Key") != h.cfg.EdgeAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid edge key"})
			return
		}
	}

	doc := models.Telemetry{
		NodeID:    req.NodeID,
		Ts:        time.Unix(req.Ts, 0),
		Loc:       req.Loc,
		Metrics:   req.Metrics,
		Signature: req.Signature,
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := db.TelemetryCol.InsertOne(ctx, doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store"})
		return
	}

	metricsJSON, _ := json.Marshal(req.Metrics)
	go func() {
		_, _ = h.worker.Reason(context.Background(), worker.ReasonRequest{
			TelemetrySummary: string(metricsJSON),
		})
	}()

	c.JSON(http.StatusOK, gin.H{"ok": true, "node_id": req.NodeID})
}

// Reason handles POST /api/reason - admin only, triggers AI reasoning
func (h *Handlers) Reason(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Fetch recent telemetry for context
	cur, err := db.TelemetryCol.Find(ctx, bson.M{},
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
		b, _ := json.Marshal(t[0].Metrics)
		summary = string(b)
	}

	resp, err := h.worker.Reason(ctx, worker.ReasonRequest{TelemetrySummary: summary})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Optionally generate audio
	audioURL := ""
	if resp.AudioText != "" {
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

	hash := auth.HashForSolana(body.Summary)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	txSig, err := h.solana.SubmitMemo(ctx, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	doc := models.Decision{
		When:     time.Now(),
		Summary:  body.Summary,
		Hash:     hash,
		AudioURL: body.AudioURL,
		SolanaTx: txSig,
	}
	if _, err := db.DecisionsCol.InsertOne(ctx, doc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store decision"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tx": txSig, "hash": hash})
}

// State handles GET /api/state - public city status
func (h *Handlers) State(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	count, _ := db.TelemetryCol.CountDocuments(ctx, bson.M{})
	decCount, _ := db.DecisionsCol.CountDocuments(ctx, bson.M{})

	c.JSON(http.StatusOK, models.CityState{
		Status:  "operational",
		Updated: time.Now(),
		Alerts:  int(decCount),
		Summary: "",
	})
	_ = count
}

// Logs handles GET /api/logs - admin only, decision history
func (h *Handlers) Logs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	cur, err := db.DecisionsCol.Find(ctx, bson.M{},
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
