package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zmb3/spotify/v2"
)

func TestServeConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	serveConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var cfg struct {
		StartYear int `json:"startYear"`
		EndYear   int `json:"endYear"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if cfg.StartYear != 2015 {
		t.Errorf("startYear = %d, want 2015", cfg.StartYear)
	}
	if cfg.EndYear != time.Now().Year() {
		t.Errorf("endYear = %d, want %d", cfg.EndYear, time.Now().Year())
	}
}

func TestServeConfig_StartYearEnv(t *testing.T) {
	t.Setenv("START_YEAR", "2018")

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	serveConfig(w, req)

	var cfg struct {
		StartYear int `json:"startYear"`
		EndYear   int `json:"endYear"`
	}
	if err := json.NewDecoder(w.Result().Body).Decode(&cfg); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if cfg.StartYear != 2018 {
		t.Errorf("startYear = %d, want 2018", cfg.StartYear)
	}
}

func TestServeConfig_StartYearEnv_Invalid(t *testing.T) {
	// Non-numeric and out-of-range values should fall back to the default.
	for _, bad := range []string{"not-a-year", "1800", "99999"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("START_YEAR", bad)

			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			w := httptest.NewRecorder()
			serveConfig(w, req)

			var cfg struct {
				StartYear int `json:"startYear"`
			}
			if err := json.NewDecoder(w.Result().Body).Decode(&cfg); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}
			if cfg.StartYear != 2015 {
				t.Errorf("bad START_YEAR %q: startYear = %d, want 2015", bad, cfg.StartYear)
			}
		})
	}
}

func TestServeIndex_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	serveIndex(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want %q", loc, "/login")
	}
}

func TestMakeSongsHandler_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/songs?year=2024", nil)
	w := httptest.NewRecorder()

	handler := makeSongsHandler(true, newResultsCache())
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for missing session, got %d", w.Code)
	}

	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var resp apiErrorResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Code != "unauthorized" {
		t.Errorf("code = %q, want %q", resp.Code, "unauthorized")
	}
	if resp.Status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.Status, http.StatusUnauthorized)
	}
	if resp.Message == "" {
		t.Error("expected non-empty error message")
	}

	foundExpiredSessionCookie := false
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session" && cookie.MaxAge == -1 {
			foundExpiredSessionCookie = true
			break
		}
	}
	if !foundExpiredSessionCookie {
		t.Error("expected session cookie to be cleared")
	}
}

func TestMakeSongsHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/songs?year=2024", nil)
	w := httptest.NewRecorder()

	// Add a valid session so we get past the auth check
	sessionID := generateRandom()
	mu.Lock()
	sessions[sessionID] = &sessionEntry{
		client:    spotify.New(http.DefaultClient),
		expiresAt: time.Now().Add(time.Hour),
	}
	mu.Unlock()
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})

	handler := makeSongsHandler(true, newResultsCache())
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for POST, got %d", w.Code)
	}

	var resp apiErrorResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Code != "method_not_allowed" {
		t.Errorf("code = %q, want %q", resp.Code, "method_not_allowed")
	}

	mu.Lock()
	delete(sessions, sessionID)
	mu.Unlock()
}

func TestMakeSongsHandler_MissingYear(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/songs", nil)
	w := httptest.NewRecorder()

	sessionID := generateRandom()
	mu.Lock()
	sessions[sessionID] = &sessionEntry{
		client:    spotify.New(http.DefaultClient),
		expiresAt: time.Now().Add(time.Hour),
	}
	mu.Unlock()
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})

	handler := makeSongsHandler(true, newResultsCache())
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for missing year, got %d", w.Code)
	}

	var resp apiErrorResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Code != "missing_year" {
		t.Errorf("code = %q, want %q", resp.Code, "missing_year")
	}

	mu.Lock()
	delete(sessions, sessionID)
	mu.Unlock()
}

func TestResultsCache(t *testing.T) {
	c := &resultsCache{entries: make(map[string]*cacheEntry)}

	tracks := []TrackInfo{{ID: "a", Name: "Song A"}}

	// Not yet cached
	if _, ok := c.get("user:2024"); ok {
		t.Fatal("expected cache miss before set")
	}

	c.set("user:2024", tracks, time.Minute)

	got, ok := c.get("user:2024")
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("unexpected cached value: %+v", got)
	}

	// Expired entry should be a miss
	c.set("user:2023", tracks, -time.Second)
	if _, ok := c.get("user:2023"); ok {
		t.Error("expected expired entry to be a cache miss")
	}

	// Empty slice should not be stored (simulates no discoveries found)
	c.set("user:2026", []TrackInfo{}, time.Minute)
	if _, ok := c.get("user:2026"); ok {
		t.Error("expected empty tracks to not be cached")
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	mu.Lock()
	sessions["alive"] = &sessionEntry{client: spotify.New(http.DefaultClient), expiresAt: time.Now().Add(time.Hour)}
	sessions["dead"] = &sessionEntry{client: spotify.New(http.DefaultClient), expiresAt: time.Now().Add(-time.Second)}
	mu.Unlock()

	cleanExpiredSessions()

	mu.Lock()
	_, aliveOK := sessions["alive"]
	_, deadOK := sessions["dead"]
	delete(sessions, "alive")
	mu.Unlock()

	if !aliveOK {
		t.Error("expected live session to remain")
	}
	if deadOK {
		t.Error("expected expired session to be removed")
	}
}
