package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"superintendent/backend/auth"
	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/handlers"
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

	// Ingest (edge-signed, optional key check)
	r.POST("/api/ingest", h.Ingest)

	// Admin-only
	jwksURL := ""
	if cfg.Auth0Domain != "" {
		jwksURL = "https://" + cfg.Auth0Domain + "/.well-known/jwks.json"
	}
	admin := r.Group("/api")
	admin.Use(auth.RequireAdmin(jwksURL))
	{
		admin.POST("/reason", h.Reason)
		admin.POST("/commit", h.Commit)
		admin.GET("/logs", h.Logs)
	}

	addr := ":" + cfg.Port
	log.Printf("Backend listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
