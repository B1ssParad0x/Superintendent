package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"superintendent/backend/config"
)

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
	mu         sync.RWMutex
	lastMode   string
	lastError  string
	lastAt     time.Time
}

type ReasonResult struct {
	Summary   string
	Risk      string
	Actions   map[string]string
	Forecast  string
	Confidence int
	AudioText string
	Explain   string
}

func New(cfg *config.Config) *Client {
	timeout := cfg.GeminiTimeoutSec
	if timeout <= 0 {
		timeout = 20
	}
	return &Client{
		apiKey: cfg.GeminiAPIKey,
		model:  cfg.GeminiModel,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		lastMode: func() string {
			if cfg.GeminiAPIKey == "" {
				return "local_not_configured"
			}
			return "configured_unverified"
		}(),
	}
}

func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

func (c *Client) Reason(ctx context.Context, cityName, telemetrySummary string, recentDecisions []string) (ReasonResult, error) {
	if !c.Enabled() {
		c.markStatus("local_not_configured", "missing GEMINI_API_KEY")
		return ReasonResult{
			Summary:   "AI reasoning is not configured.",
			Risk:      "low",
			Actions:   map[string]string{"conservative": "Set GEMINI_API_KEY.", "aggressive": "Configure Gemini model."},
			AudioText: "AI reasoning is offline.",
			Explain:   "Missing GEMINI_API_KEY.",
		}, nil
	}
	prompt := buildReasonPrompt(cityName, telemetrySummary, recentDecisions)
	text, err := c.generate(ctx, prompt)
	if err != nil {
		c.markStatus("local_fallback", err.Error())
		return ReasonResult{}, err
	}
	c.markStatus("cloud_active", "")
	return parseReasonJSON(text), nil
}

func (c *Client) Chat(ctx context.Context, cityName string, recentMessages []string, userInput string) (string, error) {
	if !c.Enabled() {
		c.markStatus("local_not_configured", "missing GEMINI_API_KEY")
		return "", fmt.Errorf("chat model unavailable")
	}
	prompt := buildChatPrompt(cityName, recentMessages, userInput)
	text, err := c.generate(ctx, prompt)
	if err != nil {
		c.markStatus("local_fallback", err.Error())
		return "", err
	}
	c.markStatus("cloud_active", "")
	return text, nil
}

func (c *Client) generate(ctx context.Context, prompt string) (string, error) {
	appendUnique := func(list []string, value string) []string {
		if value == "" {
			return list
		}
		for _, existing := range list {
			if existing == value {
				return list
			}
		}
		return append(list, value)
	}

	configured := normalizeModelName(c.getModel())
	candidates := make([]string, 0, 5)
	candidates = appendUnique(candidates, configured)
	candidates = appendUnique(candidates, "gemini-2.5-flash")
	candidates = appendUnique(candidates, "gemini-2.5-flash-lite")
	candidates = appendUnique(candidates, "gemini-2.0-flash-lite")
	candidates = appendUnique(candidates, "gemini-2.0-flash")
	candidates = appendUnique(candidates, "gemini-1.5-flash-latest")
	for _, discovered := range c.discoverFlashModels(ctx) {
		candidates = appendUnique(candidates, discovered)
	}

	errs := make([]string, 0, len(candidates))
	for _, model := range candidates {
		text, err := c.generateWithModel(ctx, model, prompt)
		if err == nil {
			// Persist successful model to avoid future fallback probes.
			c.setModel(model)
			return text, nil
		}
		errs = append(errs, fmt.Sprintf("%s -> %v", model, err))
		// Keep trying only on model-not-found; otherwise fail fast with the real API error.
		if !strings.Contains(err.Error(), "status 404") {
			return "", err
		}
	}
	return "", fmt.Errorf("gemini model resolution failed: %s", strings.Join(errs, " | "))
}

func (c *Client) generateWithModel(ctx context.Context, model, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	}
	b, _ := json.Marshal(reqBody)
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini status %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty gemini response")
	}
	return strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text), nil
}

func normalizeModelName(model string) string {
	m := strings.TrimSpace(model)
	return strings.TrimPrefix(m, "models/")
}

func (c *Client) discoverFlashModels(ctx context.Context) []string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://generativelanguage.googleapis.com/v1beta/models?key="+c.apiKey, nil)
	if err != nil {
		return nil
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	var payload struct {
		Models []struct {
			Name                      string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	out := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		name := normalizeModelName(model.Name)
		if !strings.Contains(strings.ToLower(name), "flash") {
			continue
		}
		supportsGenerate := false
		for _, method := range model.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func buildReasonPrompt(cityName, telemetrySummary string, recentDecisions []string) string {
	return fmt.Sprintf(`You are The Superintendent, an AI civic intelligence system.
City: %s
Telemetry summary:
%s

Recent decisions:
%s

Respond with VALID JSON only:
{
  "summary": "1-2 sentence executive summary",
  "risk": "low|medium|high",
  "actions": {
    "conservative": "Cautious action",
    "aggressive": "Assertive action"
  },
  "forecast": "1-2 sentence near-term forecast for the next 1-3 hours",
  "confidence": 0,
  "audio_text": "1-2 sentence spoken advisory",
  "explain": "short technical reasoning"
}`, cityName, telemetrySummary, strings.Join(recentDecisions, "\n- "))
}

func buildChatPrompt(cityName string, recentMessages []string, userInput string) string {
	return fmt.Sprintf(`You are The Superintendent, an operations AI assistant for city monitoring.
Active city: %s

Recent chat context:
%s

User message:
%s

Give a concise, actionable response in plain text (no markdown tables).`, cityName, strings.Join(recentMessages, "\n"), userInput)
}

func parseReasonJSON(text string) ReasonResult {
	trimmed := strings.TrimSpace(text)
	if strings.Contains(trimmed, "```json") {
		trimmed = strings.Split(strings.Split(trimmed, "```json")[1], "```")[0]
	} else if strings.Contains(trimmed, "```") {
		parts := strings.Split(trimmed, "```")
		if len(parts) > 1 {
			trimmed = parts[1]
		}
	}
	var parsed struct {
		Summary   string            `json:"summary"`
		Risk      string            `json:"risk"`
		Actions   map[string]string `json:"actions"`
		Forecast  string            `json:"forecast"`
		Confidence int              `json:"confidence"`
		AudioText string            `json:"audio_text"`
		Explain   string            `json:"explain"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &parsed); err != nil {
		return ReasonResult{
			Summary:   firstN(trimmed, 220),
			Risk:      "medium",
			Actions:   map[string]string{"conservative": "Review AI output.", "aggressive": "Retry generation."},
			AudioText: "Unable to produce a structured advisory.",
			Explain:   "Gemini returned non-JSON response.",
		}
	}
	if parsed.Actions == nil {
		parsed.Actions = map[string]string{"conservative": "", "aggressive": ""}
	}
	if parsed.Risk == "" {
		parsed.Risk = "medium"
	}
	return ReasonResult{
		Summary:   parsed.Summary,
		Risk:      parsed.Risk,
		Actions:   parsed.Actions,
		Forecast:  parsed.Forecast,
		Confidence: func(v int) int {
			if v < 0 {
				return 0
			}
			if v > 100 {
				return 100
			}
			return v
		}(parsed.Confidence),
		AudioText: parsed.AudioText,
		Explain:   parsed.Explain,
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (c *Client) markStatus(mode, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastMode = mode
	c.lastError = strings.TrimSpace(errMsg)
	c.lastAt = time.Now().UTC()
}

func (c *Client) Status() (mode, errMsg string, at time.Time, configured bool, model string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastMode, c.lastError, c.lastAt, c.apiKey != "", c.model
}

func (c *Client) getModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Client) setModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = model
}
