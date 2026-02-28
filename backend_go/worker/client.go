package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ReasonRequest sends context to AI worker for reasoning
type ReasonRequest struct {
	TelemetrySummary string                 `json:"telemetry_summary"`
	RecentDecisions  []string               `json:"recent_decisions,omitempty"`
	Context          map[string]interface{} `json:"context,omitempty"`
}

// ReasonResponse from AI worker
type ReasonResponse struct {
	Summary    string   `json:"summary"`
	Risk       string   `json:"risk"`
	Actions    struct {
		Conservative string `json:"conservative"`
		Aggressive   string `json:"aggressive"`
	} `json:"actions"`
	AudioText string `json:"audio_text"`
	Explain   string `json:"explain"`
}

func (c *Client) Reason(ctx context.Context, req ReasonRequest) (*ReasonResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/reason", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai worker reason: %s", string(b))
	}
	var out ReasonResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SpeakRequest for TTS
type SpeakRequest struct {
	Text string `json:"text"`
}

// SpeakResponse returns audio URL
type SpeakResponse struct {
	AudioURL string `json:"audio_url"`
}

func (c *Client) Speak(ctx context.Context, text string) (string, error) {
	body, _ := json.Marshal(SpeakRequest{Text: text})
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/speak", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai worker speak: %s", string(b))
	}
	var out SpeakResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.AudioURL, nil
}
