package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"superintendent/backend/config"
)

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

type ReasonResult struct {
	Summary   string
	Risk      string
	Actions   map[string]string
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
	}
}

func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

func (c *Client) Reason(ctx context.Context, cityName, telemetrySummary string, recentDecisions []string) (ReasonResult, error) {
	if !c.Enabled() {
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
		return ReasonResult{}, err
	}
	return parseReasonJSON(text), nil
}

func (c *Client) Chat(ctx context.Context, cityName string, recentMessages []string, userInput string) (string, error) {
	if !c.Enabled() {
		return "AI chat is not configured. Add GEMINI_API_KEY.", nil
	}
	prompt := buildChatPrompt(cityName, recentMessages, userInput)
	return c.generate(ctx, prompt)
}

func (c *Client) generate(ctx context.Context, prompt string) (string, error) {
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
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)
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
