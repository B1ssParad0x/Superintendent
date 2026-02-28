package db

import (
	"context"
	"superintendent/backend/config"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client
var TelemetryCol *mongo.Collection
var DecisionsCol *mongo.Collection

func Init(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return err
	}

	Client = client
	db := client.Database("superintendent")
	TelemetryCol = db.Collection("telemetry")
	DecisionsCol = db.Collection("decisions")

	return client.Ping(ctx, nil)
}

func Close() {
	if Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = Client.Disconnect(ctx)
	}
}
