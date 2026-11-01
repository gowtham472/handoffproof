package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gowtham472/handoffproof/internal/score"
)

// cmdScore computes the Handover Score and appends it to the trend history.
func cmdScore(root string, args []string) error {
	res := score.Compute(root)
	entry := map[string]any{
		"ts":         time.Now().UTC().Format(time.RFC3339),
		"commit":     gitShort(root),
		"total":      res.Total,
		"components": res.Components,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	hist := filepath.Join(root, "docs", "score-history.jsonl")
	f, err := os.OpenFile(hist, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("Handover Score: %d/100 %v\nappended to %s\n", res.Total, res.Components, hist)
	return nil
}
