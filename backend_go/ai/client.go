package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
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
	RiskScore int
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
			RiskScore: 20,
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
		text, err := c.generateWithRetries(ctx, model, prompt)
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

func (c *Client) generateWithRetries(ctx context.Context, model, prompt string) (string, error) {
	var lastErr error
	const attempts = 3
	for i := 0; i < attempts; i++ {
		text, err := c.generateWithModel(ctx, model, prompt)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !isRetryableGeminiError(err) || ctx.Err() != nil || i == attempts-1 {
			break
		}
		backoff := time.Duration(350*(i+1)) * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
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
		"generationConfig": map[string]interface{}{
			"temperature":      0.2,
			"maxOutputTokens":  900,
			"candidateCount":   1,
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

func isRetryableGeminiError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "context deadline exceeded") {
		return true
	}
	return strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 500") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 504") ||
		strings.Contains(msg, "timeout")
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
  "summary": "2-3 complete sentences (no truncation) grounded in telemetry and risk signals",
  "risk": "low|medium|high",
  "risk_score": 0,
  "actions": {
    "conservative": "Cautious action",
    "aggressive": "Assertive action"
  },
  "forecast": "Include three horizons (30m, 3h, 12h) and one leading indicator to watch",
  "confidence": 0,
  "audio_text": "1-2 sentence spoken advisory",
  "explain": "short technical reasoning with top 2 signal drivers and uncertainty"
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
		RiskScore int               `json:"risk_score"`
		Actions   map[string]string `json:"actions"`
		Forecast  string            `json:"forecast"`
		Confidence int              `json:"confidence"`
		AudioText string            `json:"audio_text"`
		Explain   string            `json:"explain"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &parsed); err != nil {
		looseSummary := extractField(trimmed, "summary")
		looseForecast := extractField(trimmed, "forecast")
		if looseSummary == "" {
			looseSummary = sanitizeNarrative(trimmed)
		}
		return ReasonResult{
			Summary:   smartClipSummary(looseSummary, 360),
			Risk:      "medium",
			RiskScore: 55,
			Actions:   map[string]string{"conservative": "Review AI output.", "aggressive": "Retry generation."},
			Forecast:  smartClipSummary(looseForecast, 260),
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
	if parsed.RiskScore <= 0 {
		switch strings.ToLower(parsed.Risk) {
		case "critical":
			parsed.RiskScore = 95
		case "high":
			parsed.RiskScore = 82
		case "medium":
			parsed.RiskScore = 58
		case "low":
			parsed.RiskScore = 28
		default:
			parsed.RiskScore = 45
		}
	}
	if parsed.RiskScore < 0 {
		parsed.RiskScore = 0
	}
	if parsed.RiskScore > 100 {
		parsed.RiskScore = 100
	}
	return ReasonResult{
		Summary:   smartClipSummary(parsed.Summary, 360),
		Risk:      parsed.Risk,
		RiskScore: parsed.RiskScore,
		Actions:   parsed.Actions,
		Forecast:  smartClipSummary(parsed.Forecast, 260),
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

func smartClipSummary(raw string, max int) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(s), "summary:") {
		s = strings.TrimSpace(s[len("summary:"):])
	}
	if s == "" {
		return ""
	}
	if len(s) > max {
		chunk := strings.TrimSpace(s[:max])
		if i := strings.LastIndexAny(chunk, ".!?"); i > max/2 {
			s = strings.TrimSpace(chunk[:i+1])
		} else if i := strings.LastIndex(chunk, " "); i > max/2 {
			s = strings.TrimSpace(chunk[:i])
		} else {
			s = chunk
		}
	}
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "!") && !strings.HasSuffix(s, "?") {
		s += "."
	}
	return s
}

func extractField(raw, field string) string {
	pattern := fmt.Sprintf(`(?is)["']%s["']\s*:\s*["']([^"']+)["']`, regexp.QuoteMeta(field))
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return sanitizeNarrative(match[1])
}

func sanitizeNarrative(raw string) string {
	text := strings.TrimSpace(raw)
	replacements := []string{"{", " ", "}", " ", "[", " ", "]", " ", "\"", "", "'", "", "\\n", " ", "\\t", " ", ",", " "}
	replacer := strings.NewReplacer(replacements...)
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
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
