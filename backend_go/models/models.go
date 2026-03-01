package models

import "time"

// Telemetry represents edge-signed sensor/API data
type Telemetry struct {
	NodeID      string                 `json:"node_id" bson:"node_id"`
	Ts          time.Time              `json:"ts" bson:"ts"`
	Loc         Location               `json:"loc" bson:"loc"`
	Metrics     map[string]interface{} `json:"metrics" bson:"metrics"`
	Signature   string                 `json:"signature" bson:"signature,omitempty"`
	CityID      string                 `json:"city_id,omitempty" bson:"city_id,omitempty"`
	CityName    string                 `json:"city_name,omitempty" bson:"city_name,omitempty"`
	CountryCode string                 `json:"country_code,omitempty" bson:"country_code,omitempty"`
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
	When        time.Time `json:"when" bson:"when"`
	Summary     string    `json:"summary" bson:"summary"`
	Hash        string    `json:"hash" bson:"hash"`
	AudioURL    string    `json:"audio_url" bson:"audio_url"`
	SolanaTx    string    `json:"solana_tx" bson:"solana_tx"`
	Risk        string    `json:"risk,omitempty" bson:"risk,omitempty"`
	Actions     map[string]string `json:"actions,omitempty" bson:"actions,omitempty"`
	Forecast    string    `json:"forecast,omitempty" bson:"forecast,omitempty"`
	Confidence  int       `json:"confidence,omitempty" bson:"confidence,omitempty"`
	Explain     string    `json:"explain,omitempty" bson:"explain,omitempty"`
	Source      string    `json:"source,omitempty" bson:"source,omitempty"`
	CityID      string    `json:"city_id,omitempty" bson:"city_id,omitempty"`
	CityName    string    `json:"city_name,omitempty" bson:"city_name,omitempty"`
	CountryCode string    `json:"country_code,omitempty" bson:"country_code,omitempty"`
}

// CityState is the public status summary
type CityState struct {
	Status      string    `json:"status"`
	Updated     time.Time `json:"updated"`
	Alerts      int       `json:"alerts"`
	Summary     string    `json:"summary,omitempty"`
	CityID      string    `json:"city_id,omitempty"`
	CityName    string    `json:"city_name,omitempty"`
	CountryCode string    `json:"country_code,omitempty"`
}

type CitySelection struct {
	PrincipalID string    `json:"-" bson:"principal_id"`
	CityID      string    `json:"city_id" bson:"city_id"`
	CityName    string    `json:"city_name" bson:"city_name"`
	CountryCode string    `json:"country_code" bson:"country_code"`
	Lat         float64   `json:"lat" bson:"lat"`
	Lon         float64   `json:"lon" bson:"lon"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

type ChatThread struct {
	ID          string    `json:"id" bson:"id"`
	PrincipalID string    `json:"-" bson:"principal_id"`
	Title       string    `json:"title" bson:"title"`
	CityID      string    `json:"city_id" bson:"city_id"`
	CityName    string    `json:"city_name,omitempty" bson:"city_name,omitempty"`
	CountryCode string    `json:"country_code,omitempty" bson:"country_code,omitempty"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" bson:"updated_at"`
}

type ChatMessage struct {
	ID        string    `json:"id" bson:"id"`
	ThreadID  string    `json:"thread_id" bson:"thread_id"`
	Role      string    `json:"role" bson:"role"`
	Content   string    `json:"content" bson:"content"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}
