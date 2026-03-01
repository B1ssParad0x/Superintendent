package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"superintendent/backend/auth"
	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/handlers"
	"superintendent/backend/models"
)

func main() {
	cfg := config.Load()
	if err := db.Init(cfg); err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer db.Close()

	h, err := handlers.New(cfg)
	if err != nil {
		log.Fatalf("handlers: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Edge-Key")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Public
	r.GET("/health", h.Health)
	r.GET("/api/state", h.State)
	r.GET("/api/telemetry", h.Telemetry)

	// Ingest (edge-signed, optional key check)
	r.POST("/api/ingest", h.Ingest)

	// Edge audio cache - returns recent advisories with audio for Pi to cache
	r.GET("/api/edge/audios", h.EdgeAudios)

	// Admin-only
	jwksURL := ""
	if cfg.Auth0Domain != "" {
		jwksURL = "https://" + cfg.Auth0Domain + "/.well-known/jwks.json"
	}
	user := r.Group("/api")
	user.Use(auth.RequireUser(jwksURL))
	{
		user.GET("/session/city", h.GetSessionCity)
		user.POST("/session/city", h.SetSessionCity)
		user.GET("/cities/search", h.SearchCities)
		user.GET("/feeds/public", h.PublicFeeds)
		user.GET("/ai/status", h.AIStatus)
		user.GET("/audit/verify", h.VerifyAuditTrail)
		user.GET("/risk/sources", h.RiskSources)
		user.POST("/advisory/refresh", h.RefreshAdvisory)
		user.GET("/logs", h.Logs)
		user.POST("/chat/thread", h.CreateChatThread)
		user.DELETE("/chat/thread/:id", h.DeleteChatThread)
		user.GET("/chat/threads", h.ListChatThreads)
		user.GET("/chat/thread/:id/messages", h.GetChatMessages)
		user.POST("/chat/thread/:id/message", h.PostChatMessage)
	}

	admin := r.Group("/api")
	admin.Use(auth.RequireAdmin(jwksURL))
	{
		admin.POST("/reason", h.Reason)
		admin.POST("/commit", h.Commit)
		admin.POST("/commit/latest", h.CommitLatest)
	}

	go func() {
		interval := time.Duration(cfg.IngestIntervalSec) * time.Second
		if interval < 15*time.Second {
			interval = 15 * time.Second
		}
		ticker := time.NewTicker(interval)
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			cities := []models.CitySelection{{
				CityID:      "default-city",
				CityName:    "Default City",
				CountryCode: "UN",
				Lat:         cfg.IngestLat,
				Lon:         cfg.IngestLon,
			}}
			cur, err := db.CitySessionsCol.Find(ctx, bson.M{}, options.Find().SetLimit(200))
			if err == nil {
				var sessions []models.CitySelection
				if err := cur.All(ctx, &sessions); err == nil {
					seen := map[string]bool{"default-city": true}
					for _, city := range sessions {
						if city.CityID == "" || seen[city.CityID] {
							continue
						}
						if city.Lat == 0 && city.Lon == 0 {
							city.Lat = cfg.IngestLat
							city.Lon = cfg.IngestLon
						}
						seen[city.CityID] = true
						cities = append(cities, city)
					}
				}
			}
			for _, city := range cities {
				cityCtx, cityCancel := context.WithTimeout(context.Background(), 45*time.Second)
				if err := h.RunCityCycle(cityCtx, city); err != nil {
					log.Printf("city cycle (%s): %v", city.CityID, err)
				}
				cityCancel()
			}
			cancel()
		}
	}()

	addr := ":" + cfg.Port
	log.Printf("Backend listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
