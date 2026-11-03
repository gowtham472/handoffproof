package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Media model IDs (dev.meta.ai, verified 2026-11-04). Both use the same
// base URL and bearer key as Muse Spark.
const (
	ImageModelID   = "muse-image-1.0"
	SegmentModelID = "sam-3.1"
)

// ImageInput is one image sent to a model as a base64 data URL.
type ImageInput struct {
	Data []byte
	MIME string // image/png, image/jpeg, ...
}

func (in ImageInput) dataURL() string {
	return "data:" + in.MIME + ";base64," + base64.StdEncoding.EncodeToString(in.Data)
}

// GenerateImage creates one PNG with Muse Image via POST /images/generations.
// Callers must label the result synthetic; it is never evidence.
func GenerateImage(ctx context.Context, cfg Config, prompt, size string) ([]byte, Usage, error) {
	if err := cfg.Validate(); err != nil {
		return nil, Usage{}, err
	}
	req := map[string]any{
		"model":           ImageModelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
		"output_format":   "png",
	}
	if size != "" {
		req["size"] = size
	}
	raw, err := postJSON(ctx, cfg.HTTP, strings.TrimSuffix(cfg.BaseURL, "/")+"/images/generations", cfg.APIKey, req)
	if err != nil {
		return nil, Usage{}, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, Usage{}, fmt.Errorf("parse image response: %w", err)
	}
	if len(resp.Data) == 0 || resp.Data[0].B64JSON == "" {
		return nil, resp.Usage, fmt.Errorf("image response has no image")
	}
	img, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, resp.Usage, fmt.Errorf("decode image: %w", err)
	}
	return img, resp.Usage, nil
}

// Box is one SAM 3.1 detection in source pixels.
type Box struct {
	ObjectID   int     `json:"object_id"`
	X1         int     `json:"x1"`
	Y1         int     `json:"y1"`
	X2         int     `json:"x2"`
	Y2         int     `json:"y2"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Segment runs SAM 3.1 text-prompted segmentation on one image and returns
// the detected boxes. Masks are not decoded: callers crop or blur by box.
func Segment(ctx context.Context, cfg Config, img ImageInput, prompt string) ([]Box, Usage, error) {
	if err := cfg.Validate(); err != nil {
		return nil, Usage{}, err
	}
	req := map[string]any{
		"model": SegmentModelID,
		"input": []any{map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": prompt},
				map[string]any{"type": "input_image", "image_url": img.dataURL()},
			},
		}},
	}
	raw, err := postJSON(ctx, cfg.HTTP, strings.TrimSuffix(cfg.BaseURL, "/")+"/responses", cfg.APIKey, req)
	if err != nil {
		return nil, Usage{}, err
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, Usage{}, fmt.Errorf("parse segmentation response: %w", err)
	}
	boxes, err := ParseSegmentation(resp.Text())
	return boxes, resp.Usage, err
}

// boxRe matches "<id><|box;k=v;...|>" in SAM's special-token text. Mask
// payloads are base85 and never contain '|', so the match cannot run into
// a mask.
var boxRe = regexp.MustCompile(`(\d+)<\|box;([^|]*)\|>`)

// ParseSegmentation reads SAM 3.1 output_text (one line per image) into
// boxes. Empty text means no matches. A box missing a coordinate fails.
func ParseSegmentation(text string) ([]Box, error) {
	var out []Box
	for _, m := range boxRe.FindAllStringSubmatch(text, -1) {
		id, _ := strconv.Atoi(m[1])
		kv := map[string]string{}
		for _, part := range strings.Split(m[2], ";") {
			if k, v, ok := strings.Cut(part, "="); ok {
				kv[k] = v
			}
		}
		b := Box{ObjectID: id}
		for _, f := range []struct {
			key string
			dst *int
		}{{"x1", &b.X1}, {"y1", &b.Y1}, {"x2", &b.X2}, {"y2", &b.Y2}} {
			v, err := strconv.Atoi(kv[f.key])
			if err != nil {
				return nil, fmt.Errorf("segmentation box %d: bad %s %q", id, f.key, kv[f.key])
			}
			*f.dst = v
		}
		if c, ok := kv["c"]; ok {
			b.Confidence, _ = strconv.ParseFloat(c, 64)
		}
		if b.X2 <= b.X1 || b.Y2 <= b.Y1 {
			return nil, fmt.Errorf("segmentation box %d is empty", id)
		}
		out = append(out, b)
	}
	return out, nil
}

// ExtractFromImages runs one structured phase over images on the Responses
// API: the same text.format schema, local validation, schema retries and
// fail-closed behaviour as RunPhase, without tools.
func ExtractFromImages(ctx context.Context, cfg Config, spec *Spec, phase, task string, images []ImageInput) (*PhaseResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	schema, ok := spec.Schemas[phase]
	if !ok {
		return nil, fmt.Errorf("unknown phase %q", phase)
	}
	content := []any{map[string]any{"type": "input_text", "text": task}}
	for _, img := range images {
		content = append(content, map[string]any{"type": "input_image", "image_url": img.dataURL()})
	}
	input := []any{map[string]any{"type": "message", "role": "user", "content": content}}
	var usage UsageTotal
	var latency time.Duration
	prevID := ""
	for attempt := 0; ; attempt++ {
		req := responseRequest{
			Model:           cfg.ModelID,
			Instructions:    spec.SystemPrompt,
			Input:           input,
			Text:            Format(phase, schema),
			MaxOutputTokens: spec.MaxOutputTokens,
			PreviousID:      prevID,
		}
		start := time.Now()
		raw, err := postJSON(ctx, cfg.HTTP, strings.TrimSuffix(cfg.BaseURL, "/")+"/responses", cfg.APIKey, req)
		latency += time.Since(start)
		if err != nil {
			return nil, err
		}
		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens
		usage.TotalTokens += resp.Usage.TotalTokens
		usage.Calls++
		text := strings.TrimSpace(resp.Text())
		var v any
		violations := ""
		if text == "" {
			violations = "empty output"
		} else if err := json.Unmarshal([]byte(text), &v); err != nil {
			violations = "output is not valid JSON: " + err.Error()
		} else if probs := ValidateSchema(schema, v); len(probs) > 0 {
			violations = strings.Join(probs, "; ")
		} else {
			return &PhaseResult{Phase: phase, JSON: json.RawMessage(text), Usage: usage,
				ModelID: cfg.ModelID, Provider: ProviderMetaResponses, Latency: latency}, nil
		}
		if attempt >= maxSchemaRetries {
			return nil, fmt.Errorf("phase %s: schema validation failed after retries: %s", phase, violations)
		}
		prevID = resp.ID
		input = []any{userText("Your previous output failed schema validation:\n" + violations + "\nReturn corrected JSON only.")}
	}
}
