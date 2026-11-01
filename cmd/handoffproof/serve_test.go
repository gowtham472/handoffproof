package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) *server {
	t.Helper()
	return &server{
		root:      t.TempDir(),
		tokenHash: hashToken("test-console-token-1"),
		jobs:      map[string]*job{},
		jobDone:   map[string]time.Time{},
		rlHits:    map[string][]time.Time{},
	}
}

func TestConsoleRequiresAuth(t *testing.T) {
	s := testServer(t)
	h := s.routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated / = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/summary", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/summary = %d, want 401", rec.Code)
	}
}

func TestConsoleTokenSetsSessionCookie(t *testing.T) {
	s := testServer(t)
	h := s.routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?token=test-console-token-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("token / = %d, want 200", rec.Code)
	}
	cookie := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hp_session" {
			cookie = c.Value
			if !c.HttpOnly {
				t.Fatal("session cookie must be HttpOnly")
			}
		}
	}
	if cookie == "" {
		t.Fatal("no hp_session cookie set")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	req.AddCookie(&http.Cookie{Name: "hp_session", Value: cookie})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie /api/summary = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing CSP frame-ancestors")
	}
}

func TestConsoleCSRFAndDrillValidation(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}

	// POST without the custom header is rejected before the handler.
	req := httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.AddCookie(auth)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without X-HP-Request = %d, want 403", rec.Code)
	}

	// With the header, the handler runs and rejects the unknown drill.
	req = httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST unknown drill = %d, want 400", rec.Code)
	}

	// Without auth, even a well-formed POST is unauthorized.
	req = httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.Header.Set("X-HP-Request", "1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST = %d, want 401", rec.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	for i := 0; i < 3; i++ {
		if !s.allow(req, 3, time.Minute) {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if s.allow(req, 3, time.Minute) {
		t.Fatal("4th request in window should be limited")
	}
}
