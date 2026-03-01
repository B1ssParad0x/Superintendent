package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"superintendent/backend/auth"
	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/handlers"
	"superintendent/backend/ingest"
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
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Edge-Key")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Public
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
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
		user.POST("/chat/thread", h.CreateChatThread)
		user.GET("/chat/threads", h.ListChatThreads)
		user.GET("/chat/thread/:id/messages", h.GetChatMessages)
		user.POST("/chat/thread/:id/message", h.PostChatMessage)
	}

	admin := r.Group("/api")
	admin.Use(auth.RequireAdmin(jwksURL))
	{
		admin.POST("/reason", h.Reason)
		admin.POST("/commit", h.Commit)
		admin.GET("/logs", h.Logs)
	}

	go func() {
		interval := time.Duration(cfg.IngestIntervalSec) * time.Second
		if interval < 15*time.Second {
			interval = 15 * time.Second
		}
		ticker := time.NewTicker(interval)
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := ingest.Run(ctx, cfg); err != nil {
				log.Printf("ingest: %v", err)
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
