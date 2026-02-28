package models

import "time"

// Telemetry represents edge-signed sensor/API data
type Telemetry struct {
	NodeID   string                 `json:"node_id" bson:"node_id"`
	Ts       time.Time              `json:"ts" bson:"ts"`
	Loc      Location               `json:"loc" bson:"loc"`
	Metrics  map[string]interface{} `json:"metrics" bson:"metrics"`
	Signature string                `json:"signature" bson:"signature,omitempty"`
}

type Location struct {
	Lat float64 `json:"lat" bson:"lat"`
	Lon float64 `json:"lon" bson:"lon"`
}

// IngestRequest is the payload from edge agent
type IngestRequest struct {
	NodeID    string                 `json:"node_id" binding:"required"`
	Ts       int64                   `json:"ts" binding:"required"`
	Loc      Location                `json:"loc" binding:"required"`
	Metrics  map[string]interface{}  `json:"metrics" binding:"required"`
	Signature string                 `json:"signature" binding:"required"`
}

// Decision represents a committed AI decision
type Decision struct {
	When       time.Time `json:"when" bson:"when"`
	Summary    string    `json:"summary" bson:"summary"`
	Hash       string    `json:"hash" bson:"hash"`
	AudioURL   string    `json:"audio_url" bson:"audio_url"`
	SolanaTx   string    `json:"solana_tx" bson:"solana_tx"`
}

// CityState is the public status summary
type CityState struct {
	Status   string     `json:"status"`
	Updated  time.Time  `json:"updated"`
	Alerts   int        `json:"alerts"`
	Summary  string     `json:"summary,omitempty"`
}
