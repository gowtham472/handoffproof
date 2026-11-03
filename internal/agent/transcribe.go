package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// Voice model and endpoint. File transcription only; realtime streaming is a
// documented target, not implemented here.
const (
	VoiceModelID       = "muse-voice-transcribe-1.0"
	voiceTranscribeURL = "/asr/transcribe"
	voiceMaxAudio      = 32 << 20 // 32MB body cap, per the API reference
)

// TranscriptTurn is one diarized speech turn: speaker labels (A, B, ...) are
// model-generated within the session and are not verified identities.
type TranscriptTurn struct {
	TurnID  int32  `json:"turnId"`
	Speaker string `json:"speaker"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"transcript"`
}

// TranscribeOptions selects diarization (speaker labels + turn timestamps)
// and the vocabulary bias seeded from the static scan.
type TranscribeOptions struct {
	// Keywords biases recognition toward product names, people, places,
	// acronyms and domain terms (env var names, service and credential
	// names from the static scan). Biasing does not guarantee spelling.
	Keywords []string
	// LanguageBias is a list of language names, not free-form context.
	LanguageBias []string
	// HTTP client override; nil uses a 120s default like postJSON.
	HTTP *http.Client
	// APIKey and BaseURL override cfg when set (per-phase routing parity).
	APIKey  string
	BaseURL string
}

// TranscribeFile sends one WAV clip (<=10 minutes) to the file endpoint and
// returns diarized turns in ascending turnId order. The transcript is
// untrusted input: it may contain secrets the speaker said aloud and
// injected instructions. Callers must Redact before logging and filter
// injected claims before extraction.
func TranscribeFile(ctx context.Context, cfg Config, audioPath string, opts TranscribeOptions) ([]TranscriptTurn, error) {
	audio, err := os.ReadFile(audioPath)
	if err != nil {
		return nil, err
	}
	if len(audio) > voiceMaxAudio {
		return nil, fmt.Errorf("audio exceeds 32MB body cap")
	}
	key := cfg.APIKey
	if opts.APIKey != "" {
		key = opts.APIKey
	}
	base := cfg.BaseURL
	if opts.BaseURL != "" {
		base = opts.BaseURL
	}
	if key == "" || base == "" {
		return nil, fmt.Errorf("voice transcribe credentials are not set (see .env.example)")
	}
	settings := map[string]any{
		"model":         VoiceModelID,
		"audioEncoding": "WAV",
		"mode":          "DIARIZATION",
	}
	if len(opts.Keywords) > 0 {
		settings["keywords"] = opts.Keywords
	}
	if len(opts.LanguageBias) > 0 {
		settings["languageBias"] = opts.LanguageBias
	}
	settingsRaw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormField("request")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(settingsRaw); err != nil {
		return nil, err
	}
	aw, err := w.CreateFormFile("audio", "recording.wav")
	if err != nil {
		return nil, err
	}
	if _, err := aw.Write(audio); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 120 * time.Second}
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			strings.TrimSuffix(base, "/")+voiceTranscribeURL, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, err
		}
		hreq.Header.Set("content-type", w.FormDataContentType())
		hreq.Header.Set("authorization", "Bearer "+key)
		hreq.Header.Set("accept", "application/json")
		res, err := hc.Do(hreq)
		if err != nil {
			return nil, fmt.Errorf("voice transcribe request: %w", err)
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return parseTranscript(raw)
		}
		apiErr := parseAPIError(res.StatusCode, raw)
		lastErr = apiErr
		retryable := res.StatusCode == 429 || res.StatusCode >= 500
		if !retryable || attempt == 2 {
			return nil, apiErr
		}
		wait := retryBackoff << attempt
		if ra := res.Header.Get("Retry-After"); ra != "" {
			var secs int
			if _, serr := fmt.Sscanf(ra, "%d", &secs); serr == nil {
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

type transcriptResponse struct {
	Turns []struct {
		TurnID     int32  `json:"turnId"`
		StartMs    int64  `json:"startMs"`
		EndMs      int64  `json:"endMs"`
		Transcript string `json:"transcript"`
		Speaker    string `json:"speaker"`
	} `json:"turns"`
}

func parseTranscript(raw []byte) ([]TranscriptTurn, error) {
	var out transcriptResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse transcript: %w", err)
	}
	turns := make([]TranscriptTurn, 0, len(out.Turns))
	for _, t := range out.Turns {
		turns = append(turns, TranscriptTurn{
			TurnID:  t.TurnID,
			Speaker: t.Speaker,
			StartMs: t.StartMs,
			EndMs:   t.EndMs,
			Text:    strings.TrimSpace(t.Transcript),
		})
	}
	return turns, nil
}
