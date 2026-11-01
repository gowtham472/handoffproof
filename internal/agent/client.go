package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds Meta Model API settings. Secrets come from the environment
// (optionally via a .env file loaded by the caller) and are never logged.
type Config struct {
	APIKey  string
	ModelID string
	BaseURL string
	HTTP    *http.Client
}

// ConfigFromEnv reads META_API_KEY (falling back to the official
// MODEL_API_KEY), META_MODEL_ID and META_API_BASE_URL.
func ConfigFromEnv() Config {
	key := os.Getenv("META_API_KEY")
	if key == "" {
		key = os.Getenv("MODEL_API_KEY")
	}
	base := os.Getenv("META_API_BASE_URL")
	if base == "" {
		base = "https://api.meta.ai/v1"
	}
	return Config{APIKey: key, ModelID: os.Getenv("META_MODEL_ID"), BaseURL: strings.TrimSuffix(base, "/")}
}

// Validate fails closed when credentials are missing.
func (c Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("META_API_KEY is not set (see .env.example)")
	}
	if c.ModelID == "" {
		return fmt.Errorf("META_MODEL_ID is not set (see .env.example)")
	}
	return nil
}

// APIError is a Meta Model API error (see error handling docs).
type APIError struct {
	Status  int
	Type    string
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("meta api %d %s/%s: %s", e.Status, e.Type, e.Code, e.Message)
}

// Usage mirrors the Responses API usage object.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Item is one Responses output/input item.
type Item struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
	Output string `json:"output,omitempty"`
}

// Response is a POST /v1/responses result.
type Response struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Output []Item `json:"output"`
	Usage  Usage  `json:"usage"`
}

// Text returns concatenated assistant output_text, if any.
func (r *Response) Text() string {
	var b strings.Builder
	for _, it := range r.Output {
		if it.Type != "message" {
			continue
		}
		for _, c := range it.Content {
			if c.Type == "output_text" {
				b.WriteString(c.Text)
			}
		}
	}
	return b.String()
}

// Calls returns function_call items from the response.
func (r *Response) Calls() []Item {
	var out []Item
	for _, it := range r.Output {
		if it.Type == "function_call" {
			out = append(out, it)
		}
	}
	return out
}

type responseRequest struct {
	Model           string           `json:"model"`
	Instructions    string           `json:"instructions"`
	Input           []any            `json:"input"`
	Tools           []map[string]any `json:"tools,omitempty"`
	Text            any              `json:"text,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	PreviousID      string           `json:"previous_response_id,omitempty"`
}

// Format builds a Responses text.format structured-output selector.
func Format(name string, schema json.RawMessage) map[string]any {
	return map[string]any{
		"format": map[string]any{
			"type":   "json_schema",
			"name":   name,
			"schema": json.RawMessage(schema),
			"strict": false,
		},
	}
}

var retryBackoff = time.Second

// CreateResponse posts to /v1/responses with retry on 429 and 5xx,
// honoring Retry-After. It never retries 4xx validation errors.
func (c Config) CreateResponse(ctx context.Context, req responseRequest) (*Response, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 120 * time.Second}
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		hreq.Header.Set("content-type", "application/json")
		hreq.Header.Set("authorization", "Bearer "+c.APIKey)
		res, err := hc.Do(hreq)
		if err != nil {
			return nil, fmt.Errorf("meta api request: %w", err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			var out Response
			if err := json.Unmarshal(body, &out); err != nil {
				return nil, fmt.Errorf("parse response: %w", err)
			}
			return &out, nil
		}
		apiErr := parseAPIError(res.StatusCode, body)
		lastErr = apiErr
		retryable := res.StatusCode == 429 || res.StatusCode >= 500
		if !retryable || attempt == 2 {
			return nil, apiErr
		}
		wait := retryBackoff << attempt
		if ra := res.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				wait = time.Duration(secs) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func parseAPIError(status int, body []byte) *APIError {
	var wrap struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	e := &APIError{Status: status}
	if err := json.Unmarshal(body, &wrap); err == nil {
		e.Type = wrap.Error.Type
		e.Code = wrap.Error.Code
		e.Message = Redact(wrap.Error.Message) // provider text is untrusted; never echo it raw
		if len(e.Message) > 500 {
			e.Message = e.Message[:500]
		}
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("status %d", status)
	}
	return e
}
