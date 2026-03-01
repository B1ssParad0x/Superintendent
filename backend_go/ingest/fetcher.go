package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"superintendent/backend/config"
	"superintendent/backend/db"
	"superintendent/backend/models"
)

const (
	nyc311URL      = "https://data.cityofnewyork.us/resource/fhrw-4uyv.json"
	openWeatherURL = "https://api.openweathermap.org/data/2.5/weather"
	openMeteoURL   = "https://api.open-meteo.com/v1/forecast"
)

// Run fetches NYC 311 and OpenWeather data, merges into metrics, and stores as telemetry
func Run(ctx context.Context, cfg *config.Config) error {
	city := models.CitySelection{
		CityID:      "default-city",
		CityName:    "Default City",
		CountryCode: "UN",
		Lat:         cfg.IngestLat,
		Lon:         cfg.IngestLon,
	}
	return RunForCity(ctx, cfg, city)
}

// RunForCity fetches telemetry for a specific city context.
func RunForCity(ctx context.Context, cfg *config.Config, city models.CitySelection) error {
	metrics := make(map[string]interface{})
	ts := time.Now().Unix()
	loc := models.Location{Lat: city.Lat, Lon: city.Lon}

	if cfg.OpenWeatherKey != "" {
		weather, err := fetchOpenWeather(ctx, cfg.OpenWeatherKey, loc.Lat, loc.Lon)
		if err == nil {
			for k, v := range weather {
				metrics[k] = v
			}
		}
	} else {
		weather, err := fetchOpenMeteo(ctx, loc.Lat, loc.Lon)
		if err == nil {
			for k, v := range weather {
				metrics[k] = v
			}
		}
	}

	if strings.EqualFold(city.CountryCode, "US") && strings.Contains(strings.ToLower(city.CityName), "new york") {
		complaints, err := fetchNYC311(ctx, 24*time.Hour)
		if err == nil {
			metrics["nyc311_last24h"] = len(complaints)
			metrics["nyc311_sample"] = complaints
		}
	}

	metrics["timestamp"] = ts
	metrics["city_name"] = city.CityName
	metrics["country_code"] = city.CountryCode
	metrics["ingest_heartbeat"] = 1
	doc := models.Telemetry{
		NodeID:      "api-ingest-001",
		Ts:          time.Unix(ts, 0),
		Loc:         loc,
		Metrics:     metrics,
		Signature:   "server-ingest",
		CityID:      city.CityID,
		CityName:    city.CityName,
		CountryCode: city.CountryCode,
	}
	if _, err := db.TelemetryCol.InsertOne(ctx, doc); err != nil {
		return err
	}
	return nil
}

func fetchOpenWeather(ctx context.Context, apiKey string, lat, lon float64) (map[string]interface{}, error) {
	u, _ := url.Parse(openWeatherURL)
	q := u.Query()
	q.Set("lat", fmt.Sprintf("%.4f", lat))
	q.Set("lon", fmt.Sprintf("%.4f", lon))
	q.Set("appid", apiKey)
	q.Set("units", "metric")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openweather: %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
			Pressure  int     `json:"pressure"`
		} `json:"main"`
		Weather []struct {
			Main string `json:"main"`
			Desc string `json:"description"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	out := map[string]interface{}{
		"temp_c":      data.Main.Temp,
		"humidity":    data.Main.Humidity,
		"pressure":    data.Main.Pressure,
		"wind_speed":  data.Wind.Speed,
		"feels_like":  data.Main.FeelsLike,
	}
	if len(data.Weather) > 0 {
		out["weather"] = data.Weather[0].Main
		out["weather_desc"] = data.Weather[0].Desc
	}
	return out, nil
}

func fetchNYC311(ctx context.Context, _ time.Duration) ([]map[string]interface{}, error) {
	u, _ := url.Parse(nyc311URL)
	q := u.Query()
	q.Set("$limit", "30")
	q.Set("$order", "created_date DESC")
	q.Set("$select", "complaint_type,borough,created_date")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nyc311: %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var complaints []map[string]interface{}
	if err := json.Unmarshal(body, &complaints); err != nil {
		return nil, err
	}
	return complaints, nil
}

func fetchOpenMeteo(ctx context.Context, lat, lon float64) (map[string]interface{}, error) {
	u, _ := url.Parse(openMeteoURL)
	q := u.Query()
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("current", "temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code")
	q.Set("timezone", "auto")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo: %s", resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		Current struct {
			TempC       float64 `json:"temperature_2m"`
			Humidity    float64 `json:"relative_humidity_2m"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"temp_c":          data.Current.TempC,
		"humidity":        data.Current.Humidity,
		"wind_speed":      data.Current.WindSpeed,
		"weather_code":    data.Current.WeatherCode,
		"weather_provider": "open-meteo",
	}, nil
}
