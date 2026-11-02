package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLandingUsesConsoleAuthentication(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	for _, tc := range []struct {
		path, contentType, content string
	}{
		{"/landing", "text/html", "HandoffProof"},
		{"/landing/landing.js", "javascript", "createElement"},
		{"/landing/landing.css", "text/css", ".hero"},
		{"/landing/phosphor.svg", "image/svg+xml", "i-external"},
		{"/landing/jakarta-latin.woff2", "font/woff2", "wOF2"},
		{"/landing/screens/evidence.jpg", "image/jpeg", "\xff\xd8"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated request = %d, want 401", rec.Code)
			}
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.AddCookie(&http.Cookie{Name: "hp_session", Value: s.tokenHash})
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), tc.contentType) || !strings.Contains(rec.Body.String(), tc.content) {
				t.Fatalf("asset response: status=%d content-type=%s bytes=%d", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
			}
			if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
				t.Fatal("same-origin landing scripts must be allowed by CSP")
			}
		})
	}
}
