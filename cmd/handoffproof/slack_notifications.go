package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

type slackNotificationSettings struct {
	Enabled      bool `json:"enabled"`
	FailuresOnly bool `json:"failures_only"`
}

type slackDelivery struct {
	Status string    `json:"status"`
	At     time.Time `json:"at,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// Settings are independent of the encrypted webhook and fail closed on read errors.
func (s *server) notificationSettings() (slackNotificationSettings, error) {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	path, err := slackStorePath()
	if err != nil {
		return slackNotificationSettings{}, err
	}
	raw, err := os.ReadFile(path + ".notifications.json")
	if os.IsNotExist(err) {
		return slackNotificationSettings{}, nil
	}
	var settings slackNotificationSettings
	if err != nil {
		return settings, err
	}
	err = json.Unmarshal(raw, &settings)
	return settings, err
}

func (s *server) slackNotifications(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can manage Slack notifications"})
		return
	}
	if r.Method == http.MethodGet {
		settings, err := s.notificationSettings()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not read notification settings; automatic posting is disabled"})
			return
		}
		s.slackMu.Lock()
		delivery := s.slackDelivery
		s.slackMu.Unlock()
		writeJSON(w, 200, map[string]any{"settings": settings, "delivery": delivery})
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var settings slackNotificationSettings
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&settings) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid notification settings"})
		return
	}
	if settings.Enabled && s.currentSlackConnection().Webhook == "" {
		writeJSON(w, 400, map[string]string{"error": "connect Slack before enabling notifications"})
		return
	}
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	path, err := slackStorePath()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		raw, _ := json.Marshal(settings)
		err = os.WriteFile(path+".notifications.json.tmp", raw, 0o600)
		if err == nil {
			err = os.Rename(path+".notifications.json.tmp", path+".notifications.json")
		}
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not save notification settings"})
		return
	}
	writeJSON(w, 200, settings)
}

var slackEvidenceID = regexp.MustCompile(`^EV-[a-f0-9]{8}$`)

// Summaries contain structured controller outcomes, never model text or logs.
func slackEvidenceSummary(records []evidence.Record) string {
	passed, failed := 0, 0
	ids := make([]string, 0, 6)
	for _, rec := range records {
		if rec.Passed {
			passed++
		} else {
			failed++
		}
		if len(ids) < 6 && slackEvidenceID.MatchString(rec.ID) {
			ids = append(ids, rec.ID)
		}
	}
	text := fmt.Sprintf("%d evidence records (%d passed, %d failed).", len(records), passed, failed)
	if len(ids) > 0 {
		text += " Evidence: " + strings.Join(ids, ", ") + "."
	}
	return text
}

func (s *server) postSlackMessage(ctx context.Context, hook, text string) error {
	body, _ := json.Marshal(map[string]any{"text": text, "unfurl_links": false, "unfurl_media": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("could not build Slack request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Slack delivery could not be completed; check the connection")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Slack rejected the post (status %d)", res.StatusCode)
	}
	return nil
}

// Called once for each terminal console rehearsal, after controller execution.
// Delivery is bounded and cannot modify or determine a rehearsal's verdict.
func (s *server) notifySlack(records []evidence.Record, runErr error) {
	settings, err := s.notificationSettings()
	if err != nil || !settings.Enabled {
		return
	}
	hasFailure := runErr != nil
	for _, rec := range records {
		if !rec.Passed {
			hasFailure = true
		}
	}
	if settings.FailuresOnly && !hasFailure {
		return
	}
	hook := s.currentSlackConnection().Webhook
	if hook == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	text := "HandoffProof rehearsal completed: " + slackEvidenceSummary(records)
	if runErr != nil {
		text += " Execution ended with an error; review the console."
	}
	if err := controller.New(s.root).ManifestVerify(ctx); err != nil {
		text += " Manifest integrity could not be confirmed."
	} else {
		text += " Manifest verified unchanged."
	}
	err = s.postSlackMessage(ctx, hook, text)
	delivery := slackDelivery{Status: "sent", At: time.Now().UTC()}
	if err != nil {
		delivery.Status, delivery.Error = "error", err.Error()
	}
	s.slackMu.Lock()
	s.slackDelivery = delivery
	s.slackMu.Unlock()
}
